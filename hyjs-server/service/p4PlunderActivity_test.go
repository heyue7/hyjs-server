package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

// TestP4PlunderActivityActualDefeatPrepublicationClaimAndCold verifies original
// Type10 attempts: a real accepted loss counts after publication; an earlier
// paid attempt and a frozen query remain outside the published activity.
func TestP4PlunderActivityActualDefeatPrepublicationClaimAndCold(t *testing.T) {
	f, opponent, _, now := p11PlunderFightSetup(t)
	if err := f.s.InitializeP4(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{f.id, opponent} {
		if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(20)
			if id == opponent {
				p.Level = proto.Int32(100)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	p11PlunderFiniteItems(t, f, opponent, "frag_trea_001_1,1")
	p11PlunderPublish(t, now)
	if err := db.GetIns().Model(&db.P11PlunderFightPolicy{}).Where("id=1").Update("level_difference", 100).Error; err != nil {
		t.Fatal(err)
	}
	cfg := p4PlunderActivityPeriod(now)
	var row db.PeriodInstance
	if err := db.GetIns().First(&row, "id=?", cfg.ID).Error; err != nil || row.Enabled || !p4PlunderActivityIdentity(row) {
		t.Fatal("defaultoff finite template", err)
	}
	attempts := func() int64 {
		t.Helper()
		var n int64
		if err := db.GetIns().Model(&db.EventFact{}).Where("player_id=? AND kind=?", f.id, cfg.Kind).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	query := func() {
		t.Helper()
		out, err := f.s.p11PlunderListAt(f.ctx, f.id, "frag_trea_001_1", time.Now())
		if err != nil || len(out[0].(*pb.SCTreasureBattleList).InfoList) != 1 {
			t.Fatal("actual held target", err)
		}
	}
	request := &pb.CSFightTreasureStartResult{FightPlayerId: proto.Int64(opponent), TargetFlagId: proto.String("frag_trea_001_1")}
	query()
	old, err := f.s.p11PlunderFightAt(f.ctx, f.id, request, 4401, time.Now())
	if err != nil || old[0].(*pb.SCFightTreasureStartResult).GetIsWin() != 0 || attempts() != 0 {
		t.Fatal("actual prepublication loss", err)
	}
	if err = db.GetIns().Model(&db.PeriodInstance{}).Where("id=?", cfg.ID).Updates(map[string]any{"enabled": true, "goal": 1}).Error; err != nil {
		t.Fatal(err)
	}
	info := func() *pb.SCActivityInfo {
		t.Helper()
		out, e := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(cfg.ID)})
		if e != nil {
			t.Fatal(e)
		}
		return out[0].(*pb.SCActivityInfo)
	}
	if v := info(); len(v.ItemList) != 1 || v.ItemList[0].GetLeft() != 0 || v.ItemList[0].GetStatus() != 0 || attempts() != 0 {
		t.Fatal("earlier generic attempt backfilled the publication", v)
	}
	query()
	if attempts() != 0 {
		t.Fatal("match query counted as accepted attempt")
	}
	accepted, err := f.s.p11PlunderFightAt(f.ctx, f.id, request, 4402, time.Now())
	if err != nil || accepted[0].(*pb.SCFightTreasureStartResult).GetIsWin() != 0 || attempts() != 1 {
		t.Fatal("actual accepted defeat must count once", err)
	}
	v := info()
	if len(v.ItemList) != 1 || v.ItemList[0].GetValue() != "1" || v.ItemList[0].GetLeft() != 1 || v.ItemList[0].GetStatus() != 1 {
		t.Fatal("original Type10 goal/progress", v)
	}
	var fact db.EventFact
	if err = db.GetIns().Where("player_id=? AND kind=?", f.id, cfg.Kind).First(&fact).Error; err != nil || fact.SourceKey != constUtil.P4PlunderActivityFactPrefix+cfg.ID+":"+accepted[0].(*pb.SCFightTreasureStartResult).GetFightId() {
		t.Fatal("activity must bind accepted fight and publication", err)
	}
	beforeP, beforeG := p10LuckyProfile(t, f.id)
	before, _ := p11PlunderFragmentCount(beforeG, "item_022")
	claim := &pb.CSActivityReward{ActivityId: proto.String(cfg.ID), Index: proto.Int32(1), GetResources: v.ItemList[0].RewardList}
	for i := 0; i < 3; i++ {
		out, e := f.s.P1Activity(f.ctx, f.id, claim)
		if e != nil {
			t.Fatal(e)
		}
		if i == 0 && (len(out[0].(*pb.SCActivityReward).GetGet().GetSimps()) != 1) {
			t.Fatal("real original item022 reward")
		}
	}
	afterP, afterG := p10LuckyProfile(t, f.id)
	after, _ := p11PlunderFragmentCount(afterG, "item_022")
	if after != before+1 || afterP.GetCoin() != beforeP.GetCoin() {
		t.Fatal("activity reward must enter inventory once")
	}
	pRaw, _ := json.Marshal(afterP)
	gRaw, _ := json.Marshal(afterG)
	conn, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	if err = conn.Close(); err != nil {
		t.Fatal(err)
	}
	f.open(t)
	retry, err := f.s.p11PlunderFightAt(f.ctx, f.id, request, 4402, time.Now())
	if err != nil || !proto.Equal(retry[0], accepted[0]) || attempts() != 1 {
		t.Fatal("cold accepted attempt repeated", err)
	}
	if v := info(); v.ItemList[0].GetStatus() != 2 {
		t.Fatal("cold original activity claimed state", v)
	}
	currentP, currentG := p10LuckyProfile(t, f.id)
	currentPRaw, _ := json.Marshal(currentP)
	currentGRaw, _ := json.Marshal(currentG)
	if string(currentPRaw) != string(pRaw) || string(currentGRaw) != string(gRaw) {
		t.Fatal("cold activity query or old fight changed resources")
	}
}
