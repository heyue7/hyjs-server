package service

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestSweep84CopyProgressKeepsCrossChapterGateAndClaims(t *testing.T) {
	s := &GameService{Tables: map[string][]map[string]string{
		"CopyChapter": {{"ChapterId": "a"}, {"ChapterId": "b"}},
		"CopyMission": {{"ChapterId": "a", "MissionId": "1"}, {"ChapterId": "a", "MissionId": "2"}, {"ChapterId": "b", "MissionId": "3", "OpenId": "q"}, {"ChapterId": "b", "MissionId": "4"}},
	}}
	claimed := &pb.CopyChapterInfo{ChapterId: proto.String("a")}
	g := &model.GameState{Copy: &pb.SCCopyPassedInfo{PassedCopyInfo: []*pb.CopySimpleInfo{
		{ChapterId: proto.String("a"), MissionId: proto.String("1"), Star: proto.Int32(3), ChallengeTimes: proto.Int32(7)},
		{ChapterId: proto.String("a"), MissionId: proto.String("2"), Star: proto.Int32(2)},
	}, PassedChapterInfo: []*pb.CopyChapterInfo{claimed}}}
	s.refreshCopyProgress(g)
	if g.Copy.PassedChapterInfo[0] != claimed || claimed.GetStar() != 5 || g.Copy.PassedCopyInfo[0].GetChallengeTimes() != 7 || g.Copy.GetCurrentMissionId() != "2" || g.Copy.PassedCopyInfo[2].GetIsOpen() != 0 {
		t.Fatal("progress, counts, chapter identity or quest gate changed", g.Copy)
	}
	g.Quests = []string{"q"}
	s.refreshCopyProgress(g)
	if g.Copy.GetCurrentMissionId() != "3" || g.Copy.PassedCopyInfo[2].GetIsOpen() != 1 || g.Copy.PassedCopyInfo[3].GetIsOpen() != 0 {
		t.Fatal("cross chapter predecessor or quest gate changed", g.Copy)
	}
	g.Copy.PassedCopyInfo[2].Star = proto.Int32(1)
	s.refreshCopyProgress(g)
	if g.Copy.GetCurrentMissionId() != "4" || g.Copy.PassedChapterInfo[1].GetStar() != 1 {
		t.Fatal("cross chapter completion failed", g.Copy)
	}
	before, _ := proto.MarshalOptions{Deterministic: true}.Marshal(g.Copy)
	s.refreshCopyProgress(g)
	after, _ := proto.MarshalOptions{Deterministic: true}.Marshal(g.Copy)
	if !bytes.Equal(before, after) {
		t.Fatal("unchanged progress was rewritten")
	}
	// Historical duplicate rows keep the first row's displayed score, while any
	// passed duplicate still satisfies the original predecessor condition.
	first := &pb.CopySimpleInfo{ChapterId: proto.String("a"), MissionId: proto.String("1"), Star: proto.Int32(0)}
	duplicate := &pb.CopySimpleInfo{ChapterId: proto.String("a"), MissionId: proto.String("1"), Star: proto.Int32(3)}
	secondChapter := &pb.CopyChapterInfo{ChapterId: proto.String("a"), Star: proto.Int32(77)}
	g.Copy = &pb.SCCopyPassedInfo{PassedCopyInfo: []*pb.CopySimpleInfo{first, duplicate}, PassedChapterInfo: []*pb.CopyChapterInfo{claimed, secondChapter}}
	s.refreshCopyProgress(g)
	if g.Copy.PassedCopyInfo[0] != first || first.GetStar() != 0 || duplicate.GetStar() != 3 || g.Copy.PassedCopyInfo[2].GetIsOpen() != 1 || claimed.GetStar() != 0 || secondChapter.GetStar() != 77 {
		t.Fatal("historical duplicate semantics changed", g.Copy)
	}
}

func TestSweep84ArenaRefreshTouchesOnlyParticipantAndChangedSnapshot(t *testing.T) {
	f, _, unrelated := p11ArenaSetup(t)
	now := time.Now()
	p11ArenaQuery(t, f, f.id, &pb.CSPvpFindMatch{}, 84001, now)
	var member, other db.P11ArenaMember
	if err := db.GetIns().First(&member, "player_id=?", f.id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&other, "player_id=?", unrelated).Error; err != nil {
		t.Fatal(err)
	}
	before := member
	// Another participant can change or become unreadable while a frozen match
	// remains valid. The challenge must not traverse or mutate that profile.
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", unrelated).Update("game", "invalid unrelated profile").Error; err != nil {
		t.Fatal(err)
	}
	refresh := func(at time.Time) error {
		return db.GetIns().Transaction(func(tx *gorm.DB) error {
			var profile db.Profile
			if err := tx.First(&profile, f.id).Error; err != nil {
				return err
			}
			return f.s.p11RefreshArenaParticipant(tx, &profile, &member, at)
		})
	}
	if err := refresh(now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if member.UpdatedAt != before.UpdatedAt || member.SnapshotHash != before.SnapshotHash {
		t.Fatal("unchanged member updated")
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Name = proto.String("已更名"); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := refresh(now.Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	info, err := p11ArenaMemberInfo(member)
	if err != nil || info.GetInfo().GetPlayerName() != "已更名" || member.Rank != before.Rank || member.RankRewardBest != before.RankRewardBest || member.SnapshotHash == before.SnapshotHash {
		t.Fatal("changed projection or rank history", err, member)
	}
	var afterOther db.P11ArenaMember
	if err := db.GetIns().First(&afterOther, "player_id=?", unrelated).Error; err != nil {
		t.Fatal(err)
	}
	if afterOther.SnapshotHash != other.SnapshotHash || afterOther.UpdatedAt != other.UpdatedAt || !afterOther.Active {
		t.Fatal("unrelated member changed")
	}
}

type sweep84SQLCounter struct {
	logger.Interface
	queries, writes, fullProfileScans int
}

func (c *sweep84SQLCounter) Trace(_ context.Context, _ time.Time, query func() (string, int64), _ error) {
	sql, _ := query()
	sql = strings.ToLower(sql)
	c.queries++
	if strings.HasPrefix(sql, "update ") || strings.HasPrefix(sql, "insert ") || strings.HasPrefix(sql, "delete ") {
		c.writes++
	}
	if strings.Contains(sql, "from `profiles`") && strings.Contains(sql, "player is not null") {
		c.fullProfileScans++
	}
}

func TestSweep84ArenaParticipantSQLBudget(t *testing.T) {
	f, peer, _ := p11ArenaSetup(t)
	now := time.Now()
	p11ArenaQuery(t, f, f.id, &pb.CSPvpFindMatch{}, 84002, now)
	measure := func(full bool) (*sweep84SQLCounter, time.Duration) {
		counter := &sweep84SQLCounter{Interface: logger.Default.LogMode(logger.Silent)}
		began := time.Now()
		err := db.GetIns().Session(&gorm.Session{Logger: counter}).Transaction(func(tx *gorm.DB) error {
			cfg, err := p11ArenaConfig(tx)
			if err != nil {
				return err
			}
			if full {
				return f.s.p11SyncArenaMembers(tx, cfg, now)
			}
			if _, err := f.s.PrepareP11ArenaDailyMutation(f.ctx, tx, now); err != nil {
				return err
			}
			for _, id := range []int64{f.id, peer} {
				var profile db.Profile
				var member db.P11ArenaMember
				if err := tx.First(&profile, id).Error; err != nil {
					return err
				}
				if err := tx.First(&member, "player_id=?", id).Error; err != nil {
					return err
				}
				if err := f.s.p11RefreshArenaParticipant(tx, &profile, &member, now); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return counter, time.Since(began)
	}
	before, beforeTime := measure(true)
	after, afterTime := measure(false)
	t.Logf("same three-player registry: before queries=%d writes=%d fullProfileScans=%d elapsed=%s; participant queries=%d writes=%d fullProfileScans=%d elapsed=%s", before.queries, before.writes, before.fullProfileScans, beforeTime, after.queries, after.writes, after.fullProfileScans, afterTime)
	if before.fullProfileScans != 1 || after.fullProfileScans != 0 || after.writes != 0 || after.queries >= before.queries {
		t.Fatal("participant work still grows with whole registry")
	}
}

func TestSweep84CopyActualSweepAndColdReceipt(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.InitCooking(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeDailyCopy(f.ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level, p.Stamina = proto.Int32(100), proto.Int32(100)
		g.Copy.PassedCopyInfo = []*pb.CopySimpleInfo{{ChapterId: proto.String("1"), MissionId: proto.String("1"), Star: proto.Int32(5)}}
		refreshCopySweepDay(g, now)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	request := &pb.CSCopySweep{ChapterId: proto.String("1"), MissionId: proto.String("1")}
	before, _ := f.profile(t)
	result, err := f.s.copySweepAt(f.ctx, f.id, request, 84003, now)
	if err != nil {
		t.Fatal(err)
	}
	paid, state := f.profile(t)
	if paid.GetStamina() >= before.GetStamina() || state.Copy.PassedCopyInfo[0].GetStar() != 5 {
		t.Fatal("sweep cost or best progress lost")
	}
	f.restart(t)
	if err := f.s.InitializeDailyCopy(f.ctx); err != nil {
		t.Fatal(err)
	}
	replayed, err := f.s.copySweepAt(f.ctx, f.id, request, 84003, now)
	if err != nil || !proto.Equal(result[0], replayed[0]) {
		t.Fatal("cold original sweep receipt", err)
	}
	after, _ := f.profile(t)
	if after.GetStamina() != paid.GetStamina() {
		t.Fatal("cold retry charged twice")
	}
}

func TestSweep84ArenaComputationCrossesDailyCutoff(t *testing.T) {
	f, challenger, clock, end := p11ArenaDailySetup(t)
	*clock = end.Add(-time.Second)
	if _, err := f.s.p11ArenaQuery(f.ctx, challenger, &pb.CSPvpFindMatch{}, 84004, *clock); err != nil {
		t.Fatal(err)
	}
	before, after := end.Add(-time.Millisecond), end.Add(time.Millisecond)
	calls := 0
	ctx := context.WithValue(f.ctx, constUtil.P11ArenaDailyClockContextKey{}, func() time.Time {
		calls++
		if calls == 1 {
			return before
		}
		return after
	})
	request := &pb.CSPvpFightResult{PlayerId: proto.Int64(f.id), ServerId: proto.Int32(1)}
	result, err := f.s.p11ArenaScopedFightAt(ctx, challenger, request, 84005, *clock)
	if err != nil {
		t.Fatal(err)
	}
	var fact db.P11ArenaDailyChallenge
	if err := db.GetIns().First(&fact, "fight_id=?", result[0].(*pb.SCPvpFightResult).GetFightId()).Error; err != nil || fact.AcceptedAt != after.UnixMilli() || calls < 2 {
		t.Fatal("accepted on pre-computation day", fact, calls, err)
	}
	var snapshot db.P11ArenaDailySnapshot
	if err := db.GetIns().Where("end_at=?", end.UnixMilli()).First(&snapshot).Error; err != nil || snapshot.CapturedAt != after.UnixMilli() || snapshot.Status != constUtil.P11ArenaDailyComplete || snapshot.RanksJSON != "[]" {
		t.Fatal("prior day did not close before this challenge", snapshot, err)
	}
	f.restart(t)
	replayed, err := f.s.p11ArenaScopedFightAt(ctx, challenger, request, 84005, *clock)
	if err != nil || !proto.Equal(result[0], replayed[0]) {
		t.Fatal("cross-day cold receipt", err)
	}
	var count int64
	if err := db.GetIns().Model(&db.P11ArenaDailyChallenge{}).Where("fight_id=?", fact.FightID).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("duplicate challenge fact", count, err)
	}
}

func BenchmarkSweep84CopyProgress(b *testing.B) {
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		b.Fatal(err)
	}
	for _, legacy := range []bool{true, false} {
		for _, count := range []int{0, len(s.Tables["CopyMission"])} {
			b.Run(fmt.Sprintf("before-%t/passed-%d", legacy, count), func(b *testing.B) {
				g := &model.GameState{}
				s.refreshCopyProgress(g)
				for i := 0; i < count; i++ {
					g.Copy.PassedCopyInfo[i].Star = proto.Int32(3)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if legacy {
						s.refreshCopyProgressBefore84(g)
					} else {
						s.refreshCopyProgress(g)
					}
				}
			})
		}
	}
}

// Frozen pre-change implementation is retained only to measure the same input.
func (s *GameService) refreshCopyProgressBefore84(g *model.GameState) {
	if g.Copy == nil {
		g.Copy = &pb.SCCopyPassedInfo{}
	}
	stars := map[string]int32{}
	for _, mission := range s.Tables["CopyMission"] {
		var info *pb.CopySimpleInfo
		for _, saved := range g.Copy.GetPassedCopyInfo() {
			if saved.GetChapterId() == mission["ChapterId"] && saved.GetMissionId() == mission["MissionId"] {
				info = saved
				break
			}
		}
		if info == nil {
			info = &pb.CopySimpleInfo{ChapterId: proto.String(mission["ChapterId"]), MissionId: proto.String(mission["MissionId"]), Star: proto.Int32(0), ChallengeTimes: proto.Int32(0)}
			g.Copy.PassedCopyInfo = append(g.Copy.PassedCopyInfo, info)
		}
		info.IsOpen = proto.Int32(0)
		if s.copyMissionOpen(g, mission) || info.GetStar() > 0 {
			info.IsOpen = proto.Int32(1)
			g.Copy.CurrentChapterId = proto.String(mission["ChapterId"])
			g.Copy.CurrentMissionId = proto.String(mission["MissionId"])
		}
		stars[mission["ChapterId"]] += info.GetStar()
	}
	for _, row := range s.Tables["CopyChapter"] {
		var chapter *pb.CopyChapterInfo
		for _, info := range g.Copy.GetPassedChapterInfo() {
			if info.GetChapterId() == row["ChapterId"] {
				chapter = info
				break
			}
		}
		if chapter == nil {
			chapter = &pb.CopyChapterInfo{ChapterId: proto.String(row["ChapterId"])}
			g.Copy.PassedChapterInfo = append(g.Copy.PassedChapterInfo, chapter)
		}
		chapter.Star = proto.Int32(stars[row["ChapterId"]])
	}
}
