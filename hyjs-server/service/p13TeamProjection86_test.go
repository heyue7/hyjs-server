package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p13TeamProjection86Fixture(t *testing.T) (*p1Fixture, []int64, db.P13Team) {
	t.Helper()
	f, ids, _ := p13TeamSetup(t)
	team := db.P13Team{ID: "projection86", LeaderID: ids[2], MaxSize: constUtil.P13TeamMaxSize, AuthorityVersion: 1, Revision: 1}
	if err := db.GetIns().Create(&team).Error; err != nil {
		t.Fatal(err)
	}
	scenes := []string{"town_main", "town", "field_001", "town_main", "town_main"}
	for i, id := range ids {
		if err := updateProfile(f.ctx, id, func(player *pb.Player, game *model.GameState) error {
			game.Position = &pb.Position{Scene: proto.String(scenes[i])}
			game.PositionLoading = false
			game.Field = &model.FieldState{Side: &pb.PlayerSide{PlayerId: proto.Int64(id), State: proto.Int32(constUtil.P8FieldIdle), CurHp: proto.Float64(float64(5+i) / 10), MaxHp: proto.Float64(1)}}
			game.Run = nil
			if i == 4 {
				game.Run = &model.CopyRun{Complete: false}
			}
			if i == 0 {
				for n := 0; n < 400; n++ {
					warrior := proto.Clone(game.Warriors[0]).(*pb.Warrior)
					warrior.Guid = proto.String(fmt.Sprintf("projection86-cold-%04d", n))
					game.Warriors = append(game.Warriors, warrior)
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		member := db.P13TeamMember{PlayerID: id, TeamID: team.ID, Generation: 1, JoinedAt: int64(i + 1)}
		if err := db.GetIns().Create(&member).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, guild := range []db.P13Guild{{ID: "guild86a", Name: "实际公会甲"}, {ID: "guild86b", Name: "实际公会乙"}} {
		if err := db.GetIns().Create(&guild).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i, guild := range []string{"guild86a", "guild86a", "guild86b"} {
		if err := db.GetIns().Create(&db.P13GuildMember{PlayerID: ids[i], GuildID: guild, JoinedAt: 1}).Error; err != nil {
			t.Fatal(err)
		}
	}
	// Keep one compressed and one legacy save, including unknown fields; a read
	// must neither drop those fields nor rewrite either source.
	for _, id := range ids[:2] {
		var row db.Profile
		if err := db.GetIns().First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		raw := []byte(*row.Game)
		if id == ids[0] && !bytes.Contains(raw, []byte(constUtil.ProfileWarriorsCodec86)) {
			t.Fatal("fixture did not produce a compressed roster")
		}
		if id == ids[1] {
			var err error
			raw, err = model.ExpandProfileGame86(raw)
			if err != nil {
				t.Fatal(err)
			}
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		fields["future86"] = json.RawMessage(`{"exact":9007199254740993}`)
		raw, _ = json.Marshal(fields)
		if err := db.GetIns().Model(&db.Profile{}).Where("id=?", id).Updates(map[string]any{"game": string(raw), "ggo_credit": int64(123)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	return f, ids, team
}

func p13TeamProjection86ByID(team *pb.Team) map[int64]*pb.TeamMember {
	result := make(map[int64]*pb.TeamMember, len(team.GetMemberList()))
	for _, member := range team.GetMemberList() {
		result[member.GetPlayerId()] = member
	}
	return result
}

func TestP13TeamProjection86FieldsSessionAndBatch(t *testing.T) {
	f, ids, team := p13TeamProjection86Fixture(t)
	var sources []db.Profile
	if err := db.GetIns().Where("id IN ?", ids[:2]).Order("id").Find(&sources).Error; err != nil {
		t.Fatal(err)
	}
	key := p10ClientKey{f.s.p10ClientOwner(), ids[3]}
	original, ok := p10ClientSinks.Load(key)
	if !ok {
		t.Fatal("fixture socket missing")
	}
	p10ClientSinks.Store(key, &p10ClientSink{session: []byte("stale-session")})
	defer p10ClientSinks.Store(key, original)
	trace := &p10BatchSQLTrace{Interface: logger.Default.LogMode(logger.Silent)}
	err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		projection := &p13TeamProjector{s: f.s, tx: tx.Session(&gorm.Session{Logger: trace})}
		first, err := projection.team(ids[0], team)
		if err != nil {
			return err
		}
		if len(first.GetMemberList()) != 5 || first.GetMemberList()[0].GetPlayerId() != team.LeaderID {
			t.Fatalf("leader ordering or capacity changed: %v", first)
		}
		byID := p13TeamProjection86ByID(first)
		for i, id := range ids {
			canonical, err := f.s.p10Player(tx, id)
			if err != nil {
				return err
			}
			got := byID[id]
			if got == nil || got.GetPlayerId() != canonical.GetPlayerId() || got.GetServerId() != int32(f.s.OriginalServerID()) || got.GetHeroId() != canonical.GetHeroWarId() || got.GetLevel() != canonical.GetLevel() || got.GetNickName() != canonical.GetPlayerName() || got.GetFightPower() != canonical.GetFightPower() || got.GetCurHp() != float64(5+i)/10 {
				t.Fatalf("actual identity/power/HP changed for %d: %v, %v", id, got, canonical)
			}
		}
		if byID[ids[0]].GetUnionName() != "实际公会甲" || byID[ids[1]].GetUnionName() != "实际公会甲" || byID[ids[2]].GetUnionName() != "实际公会乙" || byID[ids[3]].GetUnionName() != "" {
			t.Fatal("current guild names changed", first)
		}
		for i, want := range []int32{1, 1, 0, 0, 0} {
			if byID[ids[i]].GetState() != want {
				t.Fatalf("scene/session state for %d: %v", ids[i], byID[ids[i]])
			}
		}
		second, err := projection.team(ids[2], team)
		if err != nil {
			return err
		}
		if by := p13TeamProjection86ByID(second); by[ids[2]].GetState() != 1 || by[ids[0]].GetState() != 0 {
			t.Fatal("viewer's current scene was not applied", second)
		}
		memberKey := p10ClientKey{f.s.p10ClientOwner(), ids[1]}
		current, exists := p10ClientSinks.Load(memberKey)
		if !exists {
			t.Fatal("second fixture socket missing")
		}
		p10ClientSinks.Store(memberKey, &p10ClientSink{session: []byte("replaced")})
		changed, err := projection.team(ids[0], team)
		p10ClientSinks.Store(memberKey, current)
		if err != nil {
			return err
		}
		if p13TeamProjection86ByID(changed)[ids[1]].GetState() != 0 {
			t.Fatal("replacement session reused old online state")
		}
		var profileReads int
		for _, query := range trace.sql {
			q := strings.ToLower(query)
			if strings.Contains(q, "from `profiles`") || strings.Contains(q, `from "profiles"`) {
				profileReads++
				if !strings.Contains(q, " in ") {
					t.Fatalf("per-player profile read: %s", query)
				}
			}
			if strings.Count(q, " join ") > 3 {
				t.Fatalf("query exceeded JOIN limit: %s", query)
			}
		}
		if profileReads != 1 {
			t.Fatalf("five members and three viewers require one profile batch, got %d: %v", profileReads, trace.sql)
		}
		t.Logf("five-member projection, three views: %d profile batch; %d SQL statements", profileReads, len(trace.sql))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var after []db.Profile
	if err := db.GetIns().Where("id IN ?", ids[:2]).Order("id").Find(&after).Error; err != nil {
		t.Fatal(err)
	}
	for i := range sources {
		if *sources[i].Game != *after[i].Game {
			t.Fatal("projection rewrote legacy, compressed or unknown save data")
		}
	}
	// A new request must compare the current stored session with the live sink.
	var rotated db.Profile
	if err := db.GetIns().First(&rotated, ids[1]).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", ids[1]).Update("session", []byte("rotated-session")).Error; err != nil {
		t.Fatal(err)
	}
	defer db.GetIns().Model(&db.Profile{}).Where("id=?", ids[1]).Update("session", rotated.Session)
	err = db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		fresh, err := f.s.p13TeamProjection(tx, ids[0], team)
		if err == nil && p13TeamProjection86ByID(fresh)[ids[1]].GetState() != 0 {
			t.Fatal("rotated stored session remained online")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestP13TeamProjection86RejectsMissingOrCorruptFacts(t *testing.T) {
	f, ids, team := p13TeamProjection86Fixture(t)
	if err := db.GetIns().Model(&db.P13TeamMember{}).Where("player_id=?", ids[4]).Update("player_id", int64(9999999)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p13TeamProjection(db.GetIns().WithContext(f.ctx), ids[0], team); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("missing member profile accepted: %v", err)
	}
	if err := db.GetIns().Model(&db.P13TeamMember{}).Where("player_id=?", 9999999).Update("player_id", ids[4]).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P13GuildMember{}).Where("player_id=?", ids[1]).Update("guild_id", "missing-guild").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p13TeamProjection(db.GetIns().WithContext(f.ctx), ids[0], team); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("missing guild accepted: %v", err)
	}
	if err := db.GetIns().Model(&db.P13GuildMember{}).Where("player_id=?", ids[1]).Update("guild_id", "guild86a").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", ids[0]).Update("game", `{"warriors":{"codec":"hyjs-warriors-zlib-v1","bytes":1,"sha256":"bad","data":"AA=="}}`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p13TeamProjection(db.GetIns().WithContext(f.ctx), ids[1], team); err == nil {
		t.Fatal("corrupt compressed roster accepted")
	}
	// A viewer outside the team still receives full stored-profile validation.
	if err := db.GetIns().Model(&db.P13TeamMember{}).Where("player_id=?", ids[0]).Update("team_id", "").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p13TeamProjection(db.GetIns().WithContext(f.ctx), ids[0], team); err == nil {
		t.Fatal("corrupt external viewer roster accepted")
	}
	if err := updateProfile(f.ctx, ids[1], func(_ *pb.Player, game *model.GameState) error {
		if err := f.s.PrepareFormation(game); err != nil {
			return err
		}
		game.Field.Side.CurHp = proto.Float64(-1)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p13TeamProjection(db.GetIns().WithContext(f.ctx), ids[2], team); err == nil || !strings.Contains(err.Error(), "血量") {
		t.Fatalf("negative actual HP accepted: %v", err)
	}
}
