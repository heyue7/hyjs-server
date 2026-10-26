package service

import (
	"strconv"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP4ArenaWinPureMonthTemplateDefaultOff(t *testing.T) {
	for _, at := range []time.Time{time.Date(2026, 10, 1, 0, 0, 0, 0, time.FixedZone("UTC+8", 8*3600)), time.Date(2026, 12, 31, 23, 59, 59, 0, time.FixedZone("UTC+8", 8*3600))} {
		cfg := p4ArenaWinPeriod(at)
		if cfg.Enabled || !p4ArenaWinFrozenIdentity(cfg) || cfg.Goal != 5 || cfg.Rewards != "item_027,1" || cfg.StartAt > at.UnixMilli() || cfg.EndAt <= at.UnixMilli() {
			t.Fatal("finite default-off publication", cfg)
		}
		cfg.RewardEndAt++
		if p4ArenaWinFrozenIdentity(cfg) {
			t.Fatal("overlapping next-month Type9 grace accepted")
		}
	}
	// Closed source windows do not read, write or synthesize a historical fact.
	cfg := p4ArenaWinPeriod(time.Now())
	if err := (&GameService{}).p4ReconcileArenaWinProgress(nil, nil, cfg, cfg.EndAt); err != nil {
		t.Fatal(err)
	}
}

func TestP4ArenaWinActualAcceptedLatePublicationOriginalClaimTitlesAndCold(t *testing.T) {
	f, challenger := p11ArenaScopedSetup(t)
	for _, load := range []func(string) error{f.s.LoadP5Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.InitializeP4(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP5(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, challenger, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(100); return nil }); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P11ArenaPolicy{}, &db.P11ArenaFightPolicy{}} {
		if err := db.GetIns().Model(table).Where("id=1").Update("enabled", true).Error; err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	arenaTitleLadderFixture75(t, f, challenger, now)
	cfg := p4ArenaWinPeriod(now)
	var initial db.PeriodInstance
	if err := db.GetIns().First(&initial, "id=?", cfg.ID).Error; err != nil || initial.Enabled {
		t.Fatal("activity must start disabled", err)
	}
	count := func(id int64) int64 {
		t.Helper()
		var n int64
		if err := db.GetIns().Model(&db.EventFact{}).Where("player_id=? AND kind=?", id, constUtil.P4ArenaWinKind).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	if _, err := f.s.p11ArenaQuery(f.ctx, challenger, &pb.CSPvpFindMatch{}, 1001, now); err != nil {
		t.Fatal(err)
	}
	if count(challenger) != 0 || count(f.id) != 0 {
		t.Fatal("query/initial rank generated wins")
	}
	args := &pb.CSPvpFightResult{PlayerId: proto.Int64(f.id), ServerId: proto.Int32(1)}
	replies, err := f.s.p11ArenaScopedFightAt(f.ctx, challenger, args, 1002, now)
	if err != nil {
		t.Fatal(err)
	}
	winner := replies[0].(*pb.SCPvpFightResult)
	if winner.GetWinPlayerId() != challenger || count(challenger) != 1 || count(f.id) != 0 {
		t.Fatal("actual source missing or loser counted", winner)
	}
	var fact db.EventFact
	if err = db.GetIns().First(&fact, "player_id=? AND kind=?", challenger, constUtil.P4ArenaWinKind).Error; err != nil || fact.SourceKey != constUtil.P4ArenaWinFactPrefix+winner.GetFightId() {
		t.Fatal("source must name accepted fight", err)
	}
	// Publish one real-win goal using the existing configurable period row. The
	// actual earlier same-window fact is reconciled without inventing old battles.
	if err = db.GetIns().Model(&db.PeriodInstance{}).Where("id=?", cfg.ID).Updates(map[string]any{"enabled": true, "goal": 1}).Error; err != nil {
		t.Fatal(err)
	}
	out, err := f.s.P1Activity(f.ctx, challenger, &pb.CSActivityInfo{ActivityId: proto.String(cfg.ID)})
	if err != nil {
		t.Fatal(err)
	}
	info := out[0].(*pb.SCActivityInfo)
	if len(info.ItemList) != 1 || info.ItemList[0].GetValue() != "1" || info.ItemList[0].GetLeft() != 1 || info.ItemList[0].GetStatus() != 1 {
		t.Fatal("original Type9 item goal/progress/reward", info)
	}
	if len(info.ItemList[0].RewardList) != 1 || info.ItemList[0].RewardList[0].GetId() != "item_027" {
		t.Fatal("reward resource", info)
	}
	inventory := func() int32 {
		t.Helper()
		_, g := p10LuckyProfile(t, challenger)
		var n int32
		for _, v := range g.Items {
			if v.GetId() == "item_027" {
				n += v.GetNum()
			}
		}
		return n
	}
	before := inventory()
	request := &pb.CSActivityReward{ActivityId: proto.String(cfg.ID), Index: proto.Int32(1), GetResources: info.ItemList[0].RewardList}
	for i := 0; i < 3; i++ {
		out, err = f.s.P1Activity(f.ctx, challenger, request)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 && len(out[0].(*pb.SCActivityReward).GetGet().GetSimps()) != 1 {
			t.Fatal("actual original claim")
		}
	}
	if inventory() != before+1 {
		t.Fatal("P1 claim not once")
	}
	old, err := f.s.p11ArenaScopedFightAt(f.ctx, challenger, args, 1002, time.Now())
	if err != nil || !proto.Equal(old[0], winner) || count(challenger) != 1 || inventory() != before+1 {
		t.Fatal("paid retry changed actual win/reward", err)
	}
	titles, err := f.s.p5Titles(f.ctx, challenger)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{22, 23, 24, 25, 26} {
		found := false
		for _, v := range titles[0].(*pb.SCTitleList).TitleList {
			if v.GetTitleId() == "title_0"+strconv.Itoa(id) {
				found = v.GetState() == 1
			}
		}
		if !found {
			t.Fatal("same real winner title missing", id)
		}
	}
	// The weak player's real challenge loses; no defensive/Friend win event is inferred.
	if _, err = f.s.p11ArenaQuery(f.ctx, f.id, &pb.CSPvpFindMatch{}, 1003, time.Now()); err != nil {
		t.Fatal(err)
	}
	lost, err := f.s.p11ArenaScopedFightAt(f.ctx, f.id, &pb.CSPvpFightResult{PlayerId: proto.Int64(challenger), ServerId: proto.Int32(1)}, 1004, time.Now())
	if err != nil || lost[0].(*pb.SCPvpFightResult).GetWinPlayerId() != challenger || count(challenger) != 1 || count(f.id) != 0 {
		t.Fatal("loser/defensive winner created activity fact", err)
	}
	sqlDB, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	if err = sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	f.open(t)
	out, err = f.s.P1Activity(f.ctx, challenger, &pb.CSActivityInfo{ActivityId: proto.String(cfg.ID)})
	if err != nil || out[0].(*pb.SCActivityInfo).ItemList[0].GetStatus() != 2 || count(challenger) != 1 || inventory() != before+1 {
		t.Fatal("cold activity source/claim changed", err)
	}
}
