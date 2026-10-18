package service

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func p11GrabRandomTestAt() time.Time {
	local := time.Now().In(time.FixedZone("UTC+8", 28800))
	return time.Date(local.Year(), local.Month()-1, 10, 12, 0, 0, 0, local.Location())
}

func p11GrabRandomPublish(t *testing.T, at time.Time) {
	t.Helper()
	end := time.Date(at.Year(), at.Month()+1, 1, 0, 0, 0, 0, at.Location())
	err := db.GetIns().Model(&db.P11GrabRandomPolicy{}).Where("id=1").Updates(map[string]any{
		"enabled": true, "published_at": at.Add(-2 * time.Hour).UnixMilli(), "start_at": at.Add(-time.Hour).UnixMilli(), "end_at": end.UnixMilli(),
		"pool_json": `[{"id":"item_001","num":1,"weight":1}]`, "interval_seconds": 1800, "daily_limit": 1,
	}).Error
	if err != nil {
		t.Fatal(err)
	}
}

func p11GrabRandomFlag(t *testing.T, f *p1Fixture, id int64, at time.Time) int32 {
	t.Helper()
	out, err := f.s.p11GrabRandomAt(f.ctx, id, 0, false, at)
	if err != nil {
		t.Fatal(err)
	}
	for _, point := range out[0].(*pb.SCRedPoint).RedPointList {
		if point.GetType() == "grabRandomReward" {
			return point.GetValue()
		}
	}
	return 0
}

func TestP11GrabRandomActualSegmentsOriginalRewardUseDayCapCold(t *testing.T) {
	f, b, _ := p11ArenaSetup(t)
	if err := f.s.InitializeP11GrabRandom(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP5(f.ctx); err != nil {
		t.Fatal(err)
	}
	p11GrabPrepare(t, f, f.id, b)
	at := p11GrabRandomTestAt()
	// An actual earlier occupation is not a historical entitlement. The offer
	// freezes only when explicitly opened and starts counting its future overlap.
	p11GrabCall(t, f, f.id, &pb.CSGrab{Index: proto.Int32(1)}, 10, at.Add(-30*time.Minute))
	if p11GrabRandomFlag(t, f, f.id, at) != 0 {
		t.Fatal("defaultoff random source")
	}
	var offers int64
	db.GetIns().Model(&db.P11GrabRandomOffer{}).Count(&offers)
	if offers != 0 {
		t.Fatal("disabled source created a historical offer")
	}
	p11GrabRandomPublish(t, at)
	if p11GrabRandomFlag(t, f, f.id, at) != 0 {
		t.Fatal("prepublication time was credited")
	}
	var offer db.P11GrabRandomOffer
	if err := db.GetIns().First(&offer).Error; err != nil {
		t.Fatal(err)
	}
	frozen := offer
	p11GrabCall(t, f, f.id, &pb.CSGiveUpGrab{Index: proto.Int32(1)}, 11, at.Add(10*time.Minute))
	p11GrabCall(t, f, f.id, &pb.CSGrab{Index: proto.Int32(7)}, 12, at.Add(10*time.Minute))
	if p11GrabRandomFlag(t, f, f.id, at.Add(30*time.Minute-time.Millisecond)) != 0 {
		t.Fatal("incomplete actual interval")
	}
	if p11GrabRandomFlag(t, f, f.id, at.Add(30*time.Minute)) != 1 || p11GrabRandomFlag(t, f, b, at.Add(time.Hour)) != 0 {
		t.Fatal("owner/actual duration eligibility")
	}
	beforeP, beforeG := p10LuckyProfile(t, f.id)
	var group sync.WaitGroup
	results := make(chan proto.Message, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			out, err := f.s.p11GrabRandomAt(f.ctx, f.id, 50, true, at.Add(30*time.Minute))
			errs <- err
			if err == nil {
				results <- out[0]
			}
		}()
	}
	group.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var first proto.Message
	for result := range results {
		if first == nil {
			first = result
		}
		if !proto.Equal(first, result) {
			t.Fatal("same claim diverged")
		}
	}
	if first == nil {
		t.Fatal("no original random response")
	}
	gain := first.(*pb.SCGetRandReward).GetAdd()
	if len(gain.GetSimps()) != 1 || gain.Simps[0].GetId() != "item_001" || gain.Simps[0].GetNum() != 1 {
		t.Fatal("original82053.add wrong", gain)
	}
	currentP, currentG := p10LuckyProfile(t, f.id)
	if simpleOwned(currentP, currentG, "item_001")-simpleOwned(beforeP, beforeG, "item_001") != 1 || currentP.GetCoin() != beforeP.GetCoin() {
		t.Fatal("concurrent claim granted repeatedly or charged")
	}
	if p11GrabRandomFlag(t, f, f.id, at.Add(time.Hour)) != 0 {
		t.Fatal("frozen daily1 cap ignored")
	}
	if _, err := f.s.p11GrabRandomAt(f.ctx, f.id, 51, true, at.Add(time.Hour)); err == nil {
		t.Fatal("daily cap bypassed")
	}
	// Actual claimed resource is consumed through the original item API.
	use := &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_001"), Num: proto.Int32(1)}}
	if _, err := f.s.p5BasicItemUse(f.ctx, f.id, use, 52); err != nil {
		t.Fatal(err)
	}
	usedP, usedG := p10LuckyProfile(t, f.id)
	if simpleOwned(usedP, usedG, "item_001") != simpleOwned(beforeP, beforeG, "item_001") || usedP.GetEnergy() != currentP.GetEnergy()+10 {
		t.Fatal("real random item use did not reach energy")
	}
	// Live off/higher limits/new pool cannot overwrite the committed finite offer.
	if err := db.GetIns().Model(&db.P11GrabRandomPolicy{}).Where("id=1").Updates(map[string]any{"enabled": false, "daily_limit": 1000, "pool_json": `[{"id":"coin","num":999,"weight":1}]`}).Error; err != nil {
		t.Fatal(err)
	}
	if p11GrabRandomFlag(t, f, f.id, at.Add(24*time.Hour)) != 1 {
		t.Fatal("legit earned window time lost at day boundary")
	}
	later, err := f.s.p11GrabRandomAt(f.ctx, f.id, 53, true, at.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(first, later[0]) {
		t.Fatal("pool changed after first publication")
	}
	var afterOffer db.P11GrabRandomOffer
	if err = db.GetIns().First(&afterOffer).Error; err != nil {
		t.Fatal(err)
	}
	if afterOffer != frozen {
		t.Fatal("frozen source changed")
	}
	var awards []db.P11GrabRandomAward
	if err = db.GetIns().Where("player_id=?", f.id).Order("ordinal").Find(&awards).Error; err != nil {
		t.Fatal(err)
	}
	if len(awards) != 2 || awards[0].Ordinal != 1 || awards[1].Ordinal != 2 || awards[0].EligibleMillis != 1800000 || awards[1].EligibleMillis != int64(190*time.Minute/time.Millisecond) {
		t.Fatal("actual segments /3h expiry double-counted", awards)
	}
	var facts int64
	db.GetIns().Model(&db.EventFact{}).Where("player_id=? AND kind=?", f.id, "grab_occupation_random_reward").Count(&facts)
	if facts != 2 {
		t.Fatal("actual P1 facts not exactly once", facts)
	}
	finalP, finalG := p10LuckyProfile(t, f.id)
	before, _ := json.Marshal(finalG)
	sql, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	sql.Close()
	f.open(t)
	replay, err := f.s.p11GrabRandomAt(f.ctx, f.id, 53, true, at.Add(25*time.Hour))
	if err != nil || !proto.Equal(replay[0], later[0]) {
		t.Fatal("cold original response not stable", err)
	}
	afterP, afterG := p10LuckyProfile(t, f.id)
	after, _ := json.Marshal(afterG)
	if !proto.Equal(finalP, afterP) || string(before) != string(after) {
		t.Fatal("cold claim changed current inventory")
	}
	if p11GrabRandomFlag(t, f, f.id, time.UnixMilli(frozen.CapturedAt).AddDate(0, 1, 0)) != 0 {
		t.Fatal("expired/missing future offer granted")
	}
}

func TestP11GrabRandomPrepublicationAndRealNoOccupation(t *testing.T) {
	f, _, _ := p11ArenaSetup(t)
	if err := f.s.InitializeP11GrabRandom(f.ctx); err != nil {
		t.Fatal(err)
	}
	at := p11GrabRandomTestAt()
	p11GrabRandomPublish(t, at)
	// Publication after its declared start cannot invent a prior operating window.
	if err := db.GetIns().Model(&db.P11GrabRandomPolicy{}).Where("id=1").Update("published_at", at.UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p11GrabRandomAt(f.ctx, f.id, 1, true, at); err == nil {
		t.Fatal("late publication accepted")
	}
	var offers, awards int64
	db.GetIns().Model(&db.P11GrabRandomOffer{}).Count(&offers)
	db.GetIns().Model(&db.P11GrabRandomAward{}).Count(&awards)
	if offers+awards != 0 {
		t.Fatal("invalid publication left partial state")
	}
	p11GrabRandomPublish(t, at)
	if p11GrabRandomFlag(t, f, f.id, at.Add(time.Hour)) != 0 {
		t.Fatal("rank/initial actor created occupation entitlement")
	}
	if _, err := f.s.p11GrabRandomAt(f.ctx, f.id, 2, true, at.Add(time.Hour)); err == nil {
		t.Fatal("unoccupied player received award")
	}
	db.GetIns().Model(&db.P11GrabRandomAward{}).Count(&awards)
	if awards != 0 {
		t.Fatal("missing actual source award")
	}
}
