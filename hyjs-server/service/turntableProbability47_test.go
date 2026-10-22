package service

import (
	"context"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

func pity47(id, hard int32) model.TurntablePrizePityRule {
	return model.TurntablePrizePityRule{PrizeID: id, Enabled: true, HardPity: hard, SoftStart: hard - 10, BasePPM: 500, SoftMaxPPM: 50000}
}
func TestTurntablePity47CurveAndValidation(t *testing.T) {
	r := pity47(0, 100)
	for _, test := range []struct {
		since int32
		want  int64
	}{{0, 500}, {89, 500}, {98, 50000}, {99, 1000000}} {
		if got := turntablePityProbability(r, test.since); got != test.want {
			t.Fatal(test, got)
		}
	}
	if !model.TurntablePrizePityValid([]model.TurntablePrizePityRule{r, pity47(3, 120)}, 200) {
		t.Fatal("valid rules")
	}
	for _, bad := range []model.TurntablePrizePityRule{{PrizeID: -1}, {PrizeID: 10}, {Enabled: true}, {PrizeID: 0, Enabled: true, HardPity: 100, SoftStart: 99, BasePPM: 1, SoftMaxPPM: 2}, {PrizeID: 0, Enabled: true, HardPity: 10001, SoftStart: 90, BasePPM: 1, SoftMaxPPM: 2}, {PrizeID: 0, Enabled: true, HardPity: 100, SoftStart: 90, BasePPM: 0, SoftMaxPPM: 2}} {
		if model.TurntablePrizePityValid([]model.TurntablePrizePityRule{bad}, 0) {
			t.Fatal("accepted", bad)
		}
	}
	if model.TurntablePrizePityValid([]model.TurntablePrizePityRule{r, r}, 0) {
		t.Fatal("duplicate")
	}
	all := []model.TurntablePrizePityRule{}
	for i := int32(0); i < 9; i++ {
		all = append(all, pity47(i, 100))
	}
	if model.TurntablePrizePityValid(all, 0) {
		t.Fatal("no fallback")
	}
	r.SoftMaxPPM = 999999
	if model.TurntablePrizePityValid([]model.TurntablePrizePityRule{r}, 200) {
		t.Fatal("sum overflow")
	}
	for _, raw := range []string{"", "[]", "null"} {
		if got, err := turntablePityRules(raw, 200); err != nil || len(got) != 0 {
			t.Fatal("legacy disabled", raw, err)
		}
	}
	if _, err := turntablePitySchedule([]turntablePityDue{{0, 1}, {3, 1}}); err == nil {
		t.Fatal("infeasible schedule")
	}
	if id, err := turntablePitySchedule([]turntablePityDue{{3, 2}, {0, 2}}); err != nil || id != 0 {
		t.Fatal("early schedule", id, err)
	}
}
func TestTurntablePity47LongSequenceIndependentResetStockAndDiamond(t *testing.T) {
	rewards := []db.GameplayPoolReward{{ID: 0, ResourceID: "coin", Weight: 1}, {ID: 3, ResourceID: "diamond", Weight: 1}, {ID: 8, ResourceID: "coin", Weight: 1}}
	rules := []model.TurntablePrizePityRule{pity47(0, 100), pity47(3, 120)}
	state := map[int32]turntablePityProgress{}
	last := map[int32]int{0: 0, 3: 0}
	for n := 1; n <= 20000; n++ {
		id, err := turntablePityDraw(rules, state, rewards, map[int32]int32{}, 10000, 200, 999999)
		if err != nil {
			t.Fatal(n, err)
		}
		if id >= 0 {
			if n-last[id] > int(rules[map[int32]int{0: 0, 3: 1}[id]].HardPity) {
				t.Fatal("missed hard", n, id, last)
			}
			last[id] = n
		}
		for _, r := range rules {
			if n-last[r.PrizeID] >= int(r.HardPity) {
				t.Fatal("overdue", n, r, last)
			}
		}
	}
	peer := map[int32]turntablePityProgress{}
	if _, err := turntablePityDraw(rules, peer, rewards, map[int32]int32{}, 10000, 200, 0); err != nil || peer[0].Since != 0 || peer[3].Since != 1 {
		t.Fatal("early reset", peer, err)
	}
	if state[0].Since == peer[0].Since && state[3].Since == peer[3].Since {
		t.Fatal("independence fixture")
	}
	prior := peer[0]
	rewards[0].Stock = 1
	if _, err := turntablePityDraw(rules, peer, rewards, map[int32]int32{0: 1}, 10000, 200, 999999); err != nil || peer[0].Since != prior.Since || !peer[0].Paused {
		t.Fatal("sold out progresses", err, peer)
	}
	diamondRules := []model.TurntablePrizePityRule{pity47(9, 100)}
	ds := map[int32]turntablePityProgress{}
	if _, err := turntablePityDraw(diamondRules, ds, rewards, map[int32]int32{}, 0, 200, 0); err != nil || ds[9].Since != 0 {
		t.Fatal("empty pool", ds, err)
	}
	if id, err := turntablePityDraw(diamondRules, ds, rewards, map[int32]int32{}, 10, 200, 0); err != nil || id != 9 || ds[9].Since != 0 {
		t.Fatal("enabled diamond", id, err)
	}
	rewards[2].Stock = 1
	if _, err := turntablePityDraw(rules, peer, rewards, map[int32]int32{8: 1}, 10, 200, 0); err == nil {
		t.Fatal("depleted fallback accepted")
	}
}

func TestTurntablePity47SQLiteFrozenRollbackAndReplay(t *testing.T) {
	f, id := localPlayActivitiesFixture(t)
	localPlayNonRedWheelSource(t)
	source := constUtil.TurntableScreenshotPool
	rule := model.TurntablePrizePityRule{PrizeID: 0, Enabled: true, HardPity: 3, SoftStart: 1, BasePPM: 1, SoftMaxPPM: 1}
	raw, _ := json.Marshal([]model.TurntablePrizePityRule{rule})
	if err := db.GetIns().Model(&db.TurntableRules{}).Where("pool_id=?", source).Update("prize_pity_rules", string(raw)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.LocalPlayActivitiesApply(f.ctx, id, "open", "BigTurnTable", "00000000-0000-4000-8000-000000004747"); err != nil {
		t.Fatal(err)
	}
	cycle := localPlayCycleID(t, f, id, "BigTurnTable")
	var frozen db.TurntableRules
	if err := db.GetIns().First(&frozen, "pool_id=?", cycle).Error; err != nil || frozen.PrizePityRules != string(raw) {
		t.Fatal("frozen rules missing", err)
	}
	if err := db.GetIns().Model(&db.TurntableRules{}).Where("pool_id=?", source).Update("prize_pity_rules", "[]").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.GameplayPoolReward{}).Where("pool_id=?", cycle).Updates(map[string]any{"resource_id": "coin", "amount": 1, "stock": 0}).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(f.ctx, constUtil.PersonalTurntableOwnerKey, id)
	progress, _ := json.Marshal(map[int32]turntablePityProgress{0: {Key: "0:coin", Since: 2}})
	state := db.TurntableState{PoolID: cycle, PlayerID: id, PrizePityState: string(progress)}
	if err := db.GetIns().Create(&state).Error; err != nil {
		t.Fatal(err)
	}
	first, err := f.s.playBigTurn(ctx, id, &pb.CSPlayBigTurn{Count: proto.Int32(1)}, 474701)
	if err != nil || first[0].(*pb.SCPlayBigTurn).GetRewardItemId() != 0 {
		t.Fatal("hard pity", err)
	}
	if err := db.GetIns().First(&state, "pool_id=? AND player_id=?", cycle, id).Error; err != nil {
		t.Fatal(err)
	}
	committed := state.PrizePityState
	if err := json.Unmarshal([]byte(committed), &map[int32]turntablePityProgress{}); err != nil {
		t.Fatal(err)
	}
	// 无剩余普通承接奖位时，费用、回执和进度必须一起回滚。
	if err := db.GetIns().Model(&db.GameplayPoolReward{}).Where("pool_id=? AND id<>0", cycle).Update("stock", 1).Error; err != nil {
		t.Fatal(err)
	}
	var personal db.TurntablePersonal
	if err := db.GetIns().First(&personal, "pool_id=? AND player_id=?", cycle, id).Error; err != nil {
		t.Fatal(err)
	}
	used := map[int32]int32{}
	for i := int32(1); i < 9; i++ {
		used[i] = 1
	}
	stock, _ := json.Marshal(used)
	if err := db.GetIns().Model(&personal).Update("used_stock", stock).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfileWithTx(f.ctx, id, func(_ *gorm.DB, p *pb.Player, _ *model.GameState) error { p.Diamond = proto.Int64(1000); return nil }); err != nil {
		t.Fatal(err)
	}
	before, _ := f.profile(t)
	if _, err := f.s.playBigTurn(ctx, id, &pb.CSPlayBigTurn{Count: proto.Int32(1)}, 474702); err == nil || err.Error() != "转盘未开启递增的普通奖位库存不足" {
		t.Fatal("missing fallback guard", err)
	}
	after, _ := f.profile(t)
	if before.GetDiamond() != after.GetDiamond() {
		t.Fatal("failed draw charged")
	}
	state = db.TurntableState{}
	if err := db.GetIns().First(&state, "pool_id=? AND player_id=?", cycle, id).Error; err != nil || state.PrizePityState != committed || state.Timestamp != 474701 {
		t.Fatal("failed draw moved state", err)
	}
	f.restart(t)
	again, err := f.s.playBigTurn(ctx, id, &pb.CSPlayBigTurn{Count: proto.Int32(1)}, 474701)
	if err != nil || !proto.Equal(first[0], again[0]) {
		t.Fatal("cold replay changed", err)
	}
}

func TestTurntablePity47SQLiteTenResetAndBagRollback(t *testing.T) {
	f, id := localPlayActivitiesFixture(t)
	localPlayNonRedWheelSource(t)
	if _, err := f.s.LocalPlayActivitiesApply(f.ctx, id, "open", "BigTurnTable", "00000000-0000-4000-8000-000000004748"); err != nil {
		t.Fatal(err)
	}
	cycle := localPlayCycleID(t, f, id, "BigTurnTable")
	rule := model.TurntablePrizePityRule{PrizeID: 0, Enabled: true, HardPity: 3, SoftStart: 1, BasePPM: 1, SoftMaxPPM: 1}
	raw, _ := json.Marshal([]model.TurntablePrizePityRule{rule})
	if err := db.GetIns().Model(&db.TurntableRules{}).Where("pool_id=?", cycle).Update("prize_pity_rules", string(raw)).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.GameplayPoolReward{}).Where("pool_id=?", cycle).Updates(map[string]any{"resource_id": "coin", "amount": 1, "stock": 0}).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfileWithTx(f.ctx, id, func(_ *gorm.DB, p *pb.Player, _ *model.GameState) error { p.Diamond = proto.Int64(1000); return nil }); err != nil {
		t.Fatal(err)
	}
	response, err := f.s.playBigTurn(f.ctx, id, &pb.CSPlayBigTurn{Count: proto.Int32(10)}, 474801)
	if err != nil || len(response[0].(*pb.SCPlayBigTurn).GetIdNumStr()) != 10 {
		t.Fatal("ten draw", err)
	}
	var state db.TurntableState
	if err := db.GetIns().First(&state, "pool_id=? AND player_id=?", cycle, id).Error; err != nil {
		t.Fatal(err)
	}
	progress := map[int32]turntablePityProgress{}
	if err := json.Unmarshal([]byte(state.PrizePityState), &progress); err != nil || progress[0].Since >= 3 {
		t.Fatal("ten reset missing", progress, err)
	}
	var records []db.TurntableRecord
	if err := db.GetIns().Where("pool_id=?", cycle).Order("id").Find(&records).Error; err != nil || len(records) != 10 {
		t.Fatal("ten records", err)
	}
	// 强制目标改为装备并填满背包，检查发奖失败后的全事务回滚。
	if err := db.GetIns().Model(&db.GameplayPoolReward{}).Where("pool_id=? AND id=0", cycle).Update("resource_id", "equip_001").Error; err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(map[int32]turntablePityProgress{0: {Key: "0:equip_001", Since: 2}})
	if err := db.GetIns().Model(&state).Update("prize_pity_state", string(raw)).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfileWithTx(f.ctx, id, func(_ *gorm.DB, p *pb.Player, _ *model.GameState) error { p.EquipBagSize = proto.Int32(0); return nil }); err != nil {
		t.Fatal(err)
	}
	before, _ := f.profile(t)
	if _, err := f.s.playBigTurn(f.ctx, id, &pb.CSPlayBigTurn{Count: proto.Int32(1)}, 474802); err == nil || err.Error() != "装备背包已满" {
		t.Fatal("bag guard", err)
	}
	after, _ := f.profile(t)
	if before.GetDiamond() != after.GetDiamond() {
		t.Fatal("bag failure charged")
	}
	state = db.TurntableState{}
	if err := db.GetIns().First(&state, "pool_id=? AND player_id=?", cycle, id).Error; err != nil || state.PrizePityState != string(raw) || state.Timestamp != 474801 {
		t.Fatal("bag failure moved progress", err)
	}
}

func TestTurntablePity47AvailabilityRecoveryAndIllegalProgress(t *testing.T) {
	rule := func(id int32) model.TurntablePrizePityRule {
		return model.TurntablePrizePityRule{PrizeID: id, Enabled: true, HardPity: 3, SoftStart: 1, BasePPM: 1, SoftMaxPPM: 1}
	}
	rewards := []db.GameplayPoolReward{{ID: 0, ResourceID: "coin", Weight: 1, Stock: 1}, {ID: 3, ResourceID: "coin", Weight: 1}, {ID: 8, ResourceID: "coin", Weight: 1}}
	rules := []model.TurntablePrizePityRule{rule(0), rule(3)}
	state := map[int32]turntablePityProgress{0: {Key: "0:coin", Since: 2}, 3: {Key: "3:coin", Since: 1}}
	if _, err := turntablePityDraw(rules, state, rewards, map[int32]int32{0: 1}, 10, 0, 999999); err != nil || !state[0].Paused || state[0].Since != 2 || state[3].Since != 2 {
		t.Fatal("pause", state, err)
	}
	for _, want := range []int32{0, 3} {
		if got, err := turntablePityDraw(rules, state, rewards, map[int32]int32{}, 10, 0, 999999); err != nil || got != want {
			t.Fatal("restock queue", got, want, state, err)
		}
	}
	for _, st := range state {
		if st.Paused || st.Recovering {
			t.Fatal("recovery marker remains", state)
		}
	}
	illegal := map[int32]turntablePityProgress{0: {Key: "0:coin", Since: 2}, 3: {Key: "3:coin", Since: 2}}
	if _, err := turntablePityDraw(rules, illegal, rewards, map[int32]int32{}, 10, 0, 999999); err == nil {
		t.Fatal("unmarked corruption repaired")
	}
	disabled := map[int32]turntablePityProgress{0: {Key: "0:coin", Since: 2}, 3: {Key: "3:coin", Since: 2}}
	turntablePityPauseDisabled(nil, disabled)
	for _, want := range []int32{0, 3} {
		if got, err := turntablePityDraw(rules, disabled, rewards, map[int32]int32{}, 10, 0, 999999); err != nil || got != want {
			t.Fatal("re-enable queue", got, want, err)
		}
	}
	diamondRules := []model.TurntablePrizePityRule{rule(0), rule(9)}
	diamond := map[int32]turntablePityProgress{0: {Key: "0:coin", Since: 1}, 9: {Key: "9:diamond", Since: 2}}
	if _, err := turntablePityDraw(diamondRules, diamond, rewards, map[int32]int32{}, 0, 0, 999999); err != nil || !diamond[9].Paused {
		t.Fatal("empty diamond pause", err)
	}
	for _, want := range []int32{0, 9} {
		if got, err := turntablePityDraw(diamondRules, diamond, rewards, map[int32]int32{}, 10, 0, 999999); err != nil || got != want {
			t.Fatal("diamond return queue", got, want, err)
		}
	}
	// 三个合法恢复奖位逐个兑现，稳定库存后继续正常严格限次。
	three := []model.TurntablePrizePityRule{rule(0), rule(3), rule(9)}
	restored := map[int32]turntablePityProgress{0: {Key: "0:coin", Since: 2, Paused: true}, 3: {Key: "3:coin", Since: 2, Paused: true}, 9: {Key: "9:diamond", Since: 2, Paused: true}}
	for _, want := range []int32{0, 3, 9} {
		if got, err := turntablePityDraw(three, restored, rewards, map[int32]int32{}, 10, 0, 999999); err != nil || got != want {
			t.Fatal("three restore", got, want, err)
		}
	}
	for n := 0; n < 1000; n++ {
		if _, err := turntablePityDraw(three, restored, rewards, map[int32]int32{}, 10, 0, 999999); err != nil {
			t.Fatal(n, err)
		}
		for _, st := range restored {
			if st.Since >= 3 || st.Recovering || st.Paused {
				t.Fatal("stable after recovery", n, restored)
			}
		}
	}
}
