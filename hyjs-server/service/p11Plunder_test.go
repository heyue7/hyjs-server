package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"sync"
	"testing"
	"time"
)

func p11PlunderCall(t *testing.T, f *p1Fixture, id int64, request proto.Message, stamp int64, now time.Time) proto.Message {
	t.Helper()
	out, err := f.s.p11PlunderAt(f.ctx, id, request, stamp, now)
	if err != nil {
		t.Fatal(err)
	}
	return out[0]
}
func p11PlunderGrant(t *testing.T, f *p1Fixture, content string) {
	t.Helper()
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		_, err := f.s.GrantFixedReward(p, g, content)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
func TestP11PlunderOriginalProtectionAtomicReplayCountdownAndExpiry(t *testing.T) {
	f, _, _ := p11ArenaSetup(t)
	now := time.Now().Add(-15 * time.Minute)
	p11PlunderGrant(t, f, "item_015,2;item_014,1;frag_trea_001_1,1;frag_trea_001_2,1;frag_trea_001_3,1")
	query := p11PlunderCall(t, f, f.id, &pb.CSRefreshTreasFrag{}, 1, now).(*pb.SCRefreshTreasFrag)
	if len(query.Frags) != 3 || query.GetAvoidWarTime() != 0 || query.GetEnergy() != 100 || query.GetMaxEnergy() != 30 {
		t.Fatal("actual fragment/protection/resource projection", query)
	}
	if _, err := f.s.p11PlunderAt(f.ctx, f.id, &pb.CSUseAvoidPlunder{UsedItem: proto.String("item_015")}, 2, now); err == nil {
		t.Fatal("default new use active")
	}
	if err := db.GetIns().Model(&db.P11PlunderPolicy{}).Where("id=1").Update("avoid_enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	replies := make(chan proto.Message, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := f.s.p11PlunderAt(f.ctx, f.id, &pb.CSUseAvoidPlunder{UsedItem: proto.String("item_015")}, 100, now)
			errs <- err
			if err == nil {
				replies <- out[0]
			}
		}()
	}
	wg.Wait()
	close(errs)
	close(replies)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var original proto.Message
	for r := range replies {
		if original == nil {
			original = r
		}
		if !proto.Equal(original, r) {
			t.Fatal("same paid use at same time changed")
		}
	}
	if original.(*pb.SCUseAvoidPlunder).GetAvoidSurplusTime() != int64(time.Hour/time.Millisecond) {
		t.Fatal("relative milliseconds original contract", original)
	}
	var state db.P11PlunderState
	db.GetIns().First(&state, f.id)
	if state.AvoidUntil != now.Add(time.Hour).UnixMilli() {
		t.Fatal("same stamp extended protection repeatedly")
	}
	_, g := p10LuckyProfile(t, f.id)
	n := int32(0)
	for _, item := range g.Items {
		if item.GetId() == "item_015" {
			n = item.GetNum()
		}
	}
	if n != 1 {
		t.Fatal("same stamp deducted repeatedly", n)
	}
	if _, err := f.s.p11PlunderAt(f.ctx, f.id, &pb.CSUseAvoidPlunder{UsedItem: proto.String("item_014")}, 100, now); err == nil {
		t.Fatal("stamp changed source item")
	}
	extended := p11PlunderCall(t, f, f.id, &pb.CSUseAvoidPlunder{UsedItem: proto.String("item_014")}, 101, now.Add(10*time.Minute)).(*pb.SCUseAvoidPlunder)
	if extended.GetAvoidSurplusTime() != int64((8*time.Hour+50*time.Minute)/time.Millisecond) {
		t.Fatal("original text add time semantics", extended)
	}
	restarted := &GameService{}
	if err := restarted.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	f.s = restarted
	query = p11PlunderCall(t, f, f.id, &pb.CSRefreshTreasFrag{}, 102, now.Add(2*time.Hour)).(*pb.SCRefreshTreasFrag)
	if query.GetAvoidWarTime() != int64(7*time.Hour/time.Millisecond) {
		t.Fatal("persisted protection restart/day countdown", query)
	}
	old := p11PlunderCall(t, f, f.id, &pb.CSUseAvoidPlunder{UsedItem: proto.String("item_015")}, 100, now.Add(2*time.Hour)).(*pb.SCUseAvoidPlunder)
	if old.GetAvoidSurplusTime() != query.GetAvoidWarTime() {
		t.Fatal("paid old response revived old UI duration")
	}
	protected, err := P11PlunderProtected(db.GetIns(), f.id, now.Add(9*time.Hour-time.Millisecond))
	if err != nil || !protected {
		t.Fatal("early protection expired", err)
	}
	protected, err = P11PlunderProtected(db.GetIns(), f.id, now.Add(9*time.Hour))
	if err != nil || protected {
		t.Fatal("protection persisted after actual expiry", err)
	}
	if err := db.GetIns().Model(&db.P11PlunderPolicy{}).Where("id=1").Updates(map[string]any{"avoid_enabled": false, "item_hours_json": "[]", "item_hours_hash": "closed"}).Error; err != nil {
		t.Fatal(err)
	}
	expired := p11PlunderCall(t, f, f.id, &pb.CSUseAvoidPlunder{UsedItem: proto.String("item_015")}, 100, now.Add(10*time.Hour)).(*pb.SCUseAvoidPlunder)
	if expired.GetAvoidSurplusTime() != 0 {
		t.Fatal("expired old use revived protection")
	}
	beforeP, beforeG := p10LuckyProfile(t, f.id)
	before, _ := json.Marshal(beforeG)
	for _, request := range []proto.Message{&pb.CSFightTreasureStartResult{}, &pb.CSFightTreasureEnd{IsWin: proto.Int32(1)}, &pb.CSSweepTreasureFight{}, &pb.CSTreasureBattleList{}} {
		if _, err := f.s.p11PlunderAt(f.ctx, f.id, request, 200, now); err == nil {
			t.Fatal("non-native result or probability invented")
		}
	}
	afterP, afterG := p10LuckyProfile(t, f.id)
	after, _ := json.Marshal(afterG)
	if !proto.Equal(beforeP, afterP) || string(before) != string(after) {
		t.Fatal("closed plunder changed resources")
	}
}
func TestP11PlunderFailuresFragmentCombineAndNoSilentCorruption(t *testing.T) {
	f, _, _ := p11ArenaSetup(t)
	now := time.Now()
	if err := db.GetIns().Model(&db.P11PlunderPolicy{}).Where("id=1").Update("avoid_enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	for _, stamp := range []int64{0, 1} {
		if _, err := f.s.p11PlunderAt(f.ctx, f.id, &pb.CSUseAvoidPlunder{UsedItem: proto.String("item_015")}, stamp, now); err == nil {
			t.Fatal("missing stamp or missing stone accepted")
		}
	}
	var states int64
	db.GetIns().Model(&db.P11PlunderState{}).Count(&states)
	if states != 0 {
		t.Fatal("failed use persisted protection")
	}
	p11PlunderGrant(t, f, "item_015,1;frag_trea_001_1,1;frag_trea_001_2,1;frag_trea_001_3,1")
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(14); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p11PlunderAt(f.ctx, f.id, &pb.CSUseAvoidPlunder{UsedItem: proto.String("item_015")}, 2, now); err == nil {
		t.Fatal("original minimum15 bypassed")
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(20); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.treasureCombine(f.ctx, f.id, &pb.CSCombine{TargetId: proto.String("trea_001")}, 100); err != nil {
		t.Fatal(err)
	}
	query := p11PlunderCall(t, f, f.id, &pb.CSRefreshTreasFrag{}, 101, now).(*pb.SCRefreshTreasFrag)
	if len(query.Frags) != 0 {
		t.Fatal("spent fragment still offered", query)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		g.Items = append(g.Items, &pb.SimpleItem{Id: proto.String("frag_trea_unknown"), Num: proto.Int32(1)})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p11PlunderAt(f.ctx, f.id, &pb.CSRefreshTreasFrag{}, 102, now); err == nil {
		t.Fatal("unknown real fragment silently invented")
	}
	if err := db.GetIns().Create(&db.P11PlunderState{PlayerID: f.id, AvoidUntil: 1, LastUseAt: 2}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := P11PlunderProtected(db.GetIns(), f.id, now); err == nil {
		t.Fatal("corrupt deadline accepted")
	}
}
