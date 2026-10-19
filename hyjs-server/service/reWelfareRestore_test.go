package service

import (
	"bytes"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

func TestAdminMonthSignNative(t *testing.T) {
	f, actor := adminModesFixture(t)
	if err := f.s.InitializeMonthSign(f.ctx); err != nil {
		t.Fatal(err)
	}
	initial, err := f.s.AdminMonthSignRules(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	if !initial.Enabled || len(initial.Rows) != 31 || initial.Rows[0].Reward[0].ResourceID != "coin" || initial.Rows[0].Reward[0].Amount != 100000 || initial.Rows[7].Reward[0].Amount != 100000 {
		t.Fatal("Re seven-day original configuration missing", initial)
	}
	at := p4At(1, 12)
	out, err := f.s.monthSignAt(f.ctx, f.id, &pb.CSRewardMonthSignList{}, at)
	if err != nil {
		t.Fatal(err)
	}
	if len(out[0].(*pb.SCRewardMonthSignList).GetList()) < 28 {
		t.Fatal("original month page incomplete")
	}
	if _, err = f.s.monthSignAt(f.ctx, f.id, &pb.CSReceiveMonthSign{}, at, 501); err != nil {
		t.Fatal(err)
	}
	before, _ := f.profile(t)
	edit := model.AdminMonthSignRulesSaveRequest{Version: initial.Version, Enabled: true, Rows: initial.Rows}
	edit.Rows[0].Reward = []model.AdminResourceAmount{{ResourceID: "diamond", Amount: 19}}
	edit.Rows[0].DoubleVIP = 2
	edit.Rows[1].Reward = []model.AdminResourceAmount{{ResourceID: "coin", Amount: 17}}
	edit.Rows[2].Reward = []model.AdminResourceAmount{{ResourceID: "frag_hero_001", Amount: 3}}
	saved, err := f.s.AdminSaveMonthSignRules(f.ctx, actor, edit)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Version == initial.Version || saved.Rows[0].Reward[0].ResourceID != "diamond" {
		t.Fatal("save response stale")
	}
	if _, err = f.s.AdminSaveMonthSignRules(f.ctx, actor, edit); err == nil {
		t.Fatal("stale editor overwrote latest config")
	}
	out, err = f.s.monthSignAt(f.ctx, f.id, &pb.CSRewardMonthSignList{}, at)
	if err != nil {
		t.Fatal(err)
	}
	current := out[0].(*pb.SCRewardMonthSignList).GetList()[0]
	if current.GetRewardId() != "coin,100000" || current.GetDoubleNeedVipLevel() != 0 || current.GetGetState() != 3 {
		t.Fatal("already claimed day changed", current)
	}
	if _, err = f.s.monthSignAt(f.ctx, f.id, &pb.CSReceiveMonthSign{}, at, 502); err == nil {
		t.Fatal("old day reward regranted")
	}
	if _, err = f.s.monthSignAt(f.ctx, f.id, &pb.CSReceiveMonthSign{}, at.AddDate(0, 0, 1), 503); err != nil {
		t.Fatal(err)
	}
	after, _ := f.profile(t)
	if after.GetCoin()-before.GetCoin() != 17 || after.GetDiamond() != before.GetDiamond() {
		t.Fatal("next day ignored new real config")
	}
	fragment, err := f.s.monthSignAt(f.ctx, f.id, &pb.CSReceiveMonthSign{}, at.AddDate(0, 0, 2), 504)
	if err != nil {
		t.Fatal("configured original fragment could not be claimed", err)
	}
	gain := fragment[0].(*pb.SCReceiveMonthSign).GetGet()
	if len(gain.GetSimps()) != 1 || gain.GetSimps()[0].GetId() != "frag_hero_001" || gain.GetSimps()[0].GetNum() != 3 {
		t.Fatal("configured fragment grant mismatch", gain)
	}
	off := model.AdminMonthSignRulesSaveRequest{Version: saved.Version, Enabled: false, Rows: saved.Rows}
	stopped, err := f.s.AdminSaveMonthSignRules(f.ctx, actor, off)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.monthSignAt(f.ctx, f.id, &pb.CSReceiveMonthSign{}, at.AddDate(0, 0, 3), 505); err == nil {
		t.Fatal("disabled monthly calendar granted")
	}
	f.restart(t)
	if err = f.s.InitializeMonthSign(f.ctx); err != nil {
		t.Fatal(err)
	}
	cold, err := f.s.AdminMonthSignRules(f.ctx, actor)
	if err != nil || cold.Enabled || cold.Version != stopped.Version {
		t.Fatal("restart overwrote operator stop", err)
	}
	var count int64
	if err = db.GetIns().Model(&db.AdminAudit{}).Where("action=?", "month-sign.save").Count(&count).Error; err != nil || count != 2 {
		t.Fatal("missing config audit", count, err)
	}
}

func TestReRestaurantNative(t *testing.T) {
	f, actor := reuseFixture(t, false)
	if err := f.s.InitializeLocalPlayActivities(f.ctx); err != nil {
		t.Fatal(err)
	}
	account, err := LoginAccount(f.ctx, "re-restaurant-owner", "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, account.Id, "餐厅原入口", "hero_043"); err != nil {
		t.Fatal(err)
	}
	f.id = account.Id
	var old db.P4WelfareRule
	if err = db.GetIns().Where("kind=?", constUtil.P4WelfareMeal).First(&old).Error; err != nil {
		t.Fatal(err)
	}
	oldRaw := adminJSON(old)
	if err = f.s.InitializeReRestaurant(f.ctx); err != nil {
		t.Fatal(err)
	}
	ctx := serverClockBindTest(t, f, f.id)
	if err = db.GetIns().Create(&db.LocalPlayActivityBinding{PlayerID: f.id, ActivityID: "AsunaKitchen", CycleID: "lp-00000000000000000000000001", Revision: 1}).Error; err != nil {
		t.Fatal(err)
	}
	at := p4At(3, 11).Add(10 * time.Minute)
	out, err := f.s.p4Welfare(ctx, f.id, &pb.CSStaminaRefill{}, 0, at)
	if err != nil {
		t.Fatal("Re restaurant hidden by retired private binding", err)
	}
	data := out[0].(*pb.SCStaminaRefill).GetDataList()
	if len(data) != 2 || data[0].GetStatus() != 1 || time.UnixMilli(data[0].GetEndTime()).In(at.Location()).Hour() != 13 {
		t.Fatal("Re native two-window page", data)
	}
	first, err := f.s.p4Welfare(ctx, f.id, &pb.CSStaminaGet{}, 601, at)
	if err != nil {
		t.Fatal(err)
	}
	gain := first[0].(*pb.SCStaminaGet).GetGet()
	if len(gain.GetSimps()) != 1 || gain.GetSimps()[0].GetId() != "stamina" || gain.GetSimps()[0].GetNum() != 180 {
		t.Fatal("Re restaurant amount", gain)
	}
	before, _ := f.profile(t)
	if _, err = f.s.p4Welfare(ctx, f.id, &pb.CSStaminaGet{}, 601, at); err != nil {
		t.Fatal("same original request retry", err)
	}
	after, _ := f.profile(t)
	if before.GetStamina() != after.GetStamina() {
		t.Fatal("receipt replay regranted stamina")
	}
	if _, err = f.s.p4Welfare(ctx, f.id, &pb.CSStaminaGet{}, 602, at); err == nil {
		t.Fatal("same lunch regranted")
	}
	if _, err = f.s.p4Welfare(ctx, f.id, &pb.CSStaminaGet{}, 603, p4At(3, 14)); err == nil {
		t.Fatal("outside real Re window")
	}
	if _, err = f.s.p4Welfare(ctx, f.id, &pb.CSStaminaGet{}, 604, p4At(3, 17)); err != nil {
		t.Fatal("independent Re dinner", err)
	}
	if _, err = f.s.p4Welfare(ctx, f.id, &pb.CSStaminaGet{}, 605, at.AddDate(0, 0, 1)); err != nil {
		t.Fatal("next real day lunch", err)
	}
	// A guessed retired personal ID still cannot cross owners in normal requests.
	privateID := "p4:meal:lp-00000000000000000000000002"
	cycle := db.LocalPlayActivityCycle{ID: "lp-00000000000000000000000002", PlayerID: f.id + 1, ActivityID: "AsunaKitchen", Source: constUtil.LocalPlayActivitiesVersion, StartAt: at.UnixMilli(), EndAt: at.AddDate(0, 0, 1).UnixMilli(), RewardEndAt: at.AddDate(0, 0, 1).UnixMilli(), Snapshot: []byte(`{}`)}
	if err = db.GetIns().Create(&cycle).Error; err != nil {
		t.Fatal(err)
	}
	allowed, err := localPlayActivityVisible(db.GetIns().WithContext(ctx), privateID)
	if err != nil || allowed {
		t.Fatal("foreign private ID opened", allowed, err)
	}
	calendar, err := f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil {
		t.Fatal(err)
	}
	for i := range calendar.Items {
		if calendar.Items[i].ID == "restore_re1051_meal_1" {
			calendar.Items[i].Enabled = false
		}
	}
	if _, err = f.s.AdminSaveActivitySchedule(f.ctx, actor, 1, model.AdminActivityScheduleSaveRequest{Version: calendar.Version, Items: calendar.Items}); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	if err = f.s.InitializeReRestaurant(f.ctx); err != nil {
		t.Fatal(err)
	}
	ctx = serverClockBindTest(t, f, f.id)
	if _, err = f.s.p4Welfare(ctx, f.id, &pb.CSStaminaRefill{}, 0, at); err == nil {
		t.Fatal("operator stop overwritten after restart")
	}
	var kept db.P4WelfareRule
	if err = db.GetIns().First(&kept, "id=?", old.ID).Error; err != nil || !bytes.Equal(oldRaw, adminJSON(kept)) {
		t.Fatal("old disabled consumer rewritten", err)
	}
	var bindings int64
	db.GetIns().Model(&db.LocalPlayActivityBinding{}).Where("player_id=? AND activity_id=?", f.id, "AsunaKitchen").Count(&bindings)
	if bindings != 1 {
		t.Fatal("old history binding removed")
	}
}
