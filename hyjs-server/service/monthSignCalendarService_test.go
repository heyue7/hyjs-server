package service

import (
	"encoding/json"
	"errors"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"sync"
	"testing"
	"time"
)

func TestMonthSignScheduledCalendar(t *testing.T) {
	f, actor := adminModesFixture(t)
	if err := f.s.InitializeMonthSign(f.ctx); err != nil {
		t.Fatal(err)
	}
	now := p4At(1, 12)
	next := now.AddDate(0, 1, 0)
	month, future := now.Format("2006-01"), next.Format("2006-01")
	current, err := f.s.AdminMonthSignRules(f.ctx, actor, month)
	if err != nil {
		t.Fatal(err)
	}
	upcoming, err := f.s.AdminMonthSignRules(f.ctx, actor, future)
	if err != nil {
		t.Fatal(err)
	}
	if current.Configured || upcoming.Configured || current.Version == upcoming.Version {
		t.Fatal("inheritance/version month binding")
	}
	wrong := model.AdminMonthSignRulesSaveRequest{Month: future, Version: current.Version, Enabled: true, Rows: current.Rows}
	if _, err := f.s.AdminSaveMonthSignRules(f.ctx, actor, wrong); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("cross month version", err)
	}
	req := model.AdminMonthSignRulesSaveRequest{Month: future, Version: upcoming.Version, Enabled: true, Rows: upcoming.Rows}
	req.Rows[0].Reward = []model.AdminResourceAmount{{ResourceID: "coin", Amount: 123}}
	req.Rows[0].DoubleVIP = 2
	saved, err := f.s.AdminSaveMonthSignRules(f.ctx, actor, req)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.Configured || saved.Month != future || len(saved.Rows) != 31 {
		t.Fatal("saved response", saved)
	}
	unchanged, err := f.s.AdminMonthSignRules(f.ctx, actor, month)
	if err != nil || unchanged.Version != current.Version || unchanged.Configured {
		t.Fatal("future edit changed current", err)
	}
	var global db.MonthSignRule
	if err := db.GetIns().First(&global, 1).Error; err != nil || global.Reward != "coin,100000" {
		t.Fatal("global mutated", err)
	}
	info := func(at time.Time) *pb.SCRewardMonthSignList {
		t.Helper()
		out, err := f.s.monthSignAt(f.ctx, f.id, &pb.CSRewardMonthSignList{}, at)
		if err != nil {
			t.Fatal(err)
		}
		return out[0].(*pb.SCRewardMonthSignList)
	}
	if got := info(now); got.List[0].GetRewardId() != "coin,100000" {
		t.Fatal("current calendar changed", got)
	}
	days := time.Date(next.Year(), next.Month()+1, 0, 0, 0, 0, 0, next.Location()).Day()
	if got := info(next); len(got.List) != days || got.List[0].GetRewardId() != "coin,123" || got.GetCurrentDay() != 1 {
		t.Fatal("month switch", got)
	}
	before, _ := f.profile(t)
	original, err := f.s.monthSignAt(f.ctx, f.id, &pb.CSReceiveMonthSign{}, next, 901)
	if err != nil {
		t.Fatal(err)
	}
	edit := model.AdminMonthSignRulesSaveRequest{Month: future, Version: saved.Version, Enabled: true, Rows: saved.Rows}
	edit.Rows[0].Reward = []model.AdminResourceAmount{{ResourceID: "coin", Amount: 999}}
	edit.Rows[0].DoubleVIP = 12
	changed, err := f.s.AdminSaveMonthSignRules(f.ctx, actor, edit)
	if err != nil {
		t.Fatal(err)
	}
	if got := info(next).List[0]; got.GetRewardId() != "coin,123" || got.GetDoubleNeedVipLevel() != 2 {
		t.Fatal("claim snapshot changed", got)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Vip = proto.Int32(2); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.monthSignAt(f.ctx, f.id, &pb.CSReceiveMonthSign{}, next, 902); err != nil {
		t.Fatal("VIP snapshot", err)
	}
	f.restart(t)
	replay, err := f.s.monthSignAt(f.ctx, f.id, &pb.CSReceiveMonthSign{}, next.AddDate(0, 1, 0), 901)
	if err != nil || !proto.Equal(original[0], replay[0]) {
		t.Fatal("cold old stamp replay", err)
	}
	after, _ := f.profile(t)
	if after.GetCoin()-before.GetCoin() != 246 {
		t.Fatal("snapshot/retry gain", after.GetCoin()-before.GetCoin())
	}
	stale := model.AdminMonthSignRulesSaveRequest{Month: future, Version: changed.Version, Enabled: true, Rows: changed.Rows}
	stale.Rows[1].Reward = []model.AdminResourceAmount{{ResourceID: "coin", Amount: 456}}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := f.s.AdminSaveMonthSignRules(f.ctx, actor, stale); results <- e }()
	}
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, constUtil.ErrAdminConfigConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatal("concurrent lost update", successes, conflicts)
	}
	past := now.AddDate(0, -1, 0).Format("2006-01")
	history, err := f.s.AdminMonthSignRules(f.ctx, actor, past)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.AdminSaveMonthSignRules(f.ctx, actor, model.AdminMonthSignRulesSaveRequest{Month: past, Version: history.Version, Enabled: true, Rows: history.Rows}); !errors.Is(err, constUtil.ErrAdminInput) {
		t.Fatal("past saved", err)
	}
	viewer := db.AdminUser{Username: "month-viewer", Role: constUtil.AdminRoleViewer, Enabled: true, PasswordHash: "unused"}
	if err := db.GetIns().Create(&viewer).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.AdminSaveMonthSignRules(f.ctx, &model.AdminPrincipal{UserID: viewer.ID}, stale); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("viewer saved", err)
	}
}

func TestMonthSignCalendarMonthValidation(t *testing.T) {
	for _, month := range []string{"2026-1", "2026-00", "2026-13", "2026-01x", " 2026-01"} {
		if ValidMonthSignMonth(month) {
			t.Fatal(month)
		}
	}
	if !ValidMonthSignMonth("2028-02") {
		t.Fatal("leap month rejected")
	}
}

func TestMonthSignScheduledOverridesPersonalSnapshot(t *testing.T) {
	f, id := localPlayActivitiesFixture(t)
	if _, err := f.s.LocalPlayActivitiesApply(f.ctx, id, "open", "EveryDaySign", "00000000-0000-4000-8000-000000000051"); err != nil {
		t.Fatal(err)
	}
	var rules []db.MonthSignRule
	if err := db.GetIns().Order("day").Find(&rules).Error; err != nil {
		t.Fatal(err)
	}
	for _, at := range []time.Time{time.Date(2028, 2, 1, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600)), time.Date(2028, 4, 1, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))} {
		rules[0].Reward = "coin,777"
		raw, err := json.Marshal(rules)
		if err != nil {
			t.Fatal(err)
		}
		if err = db.GetIns().Create(&db.MonthSignCalendar{MonthKey: at.Format("2006-01"), Rules: raw}).Error; err != nil {
			t.Fatal(err)
		}
		out, err := f.s.monthSignAt(f.ctx, id, &pb.CSRewardMonthSignList{}, at)
		if err != nil {
			t.Fatal(err)
		}
		got := out[0].(*pb.SCRewardMonthSignList)
		expected := time.Date(at.Year(), at.Month()+1, 0, 0, 0, 0, 0, at.Location()).Day()
		if len(got.List) != expected || got.List[0].GetRewardId() != "coin,777" {
			t.Fatal("personal snapshot masked scheduled month", got)
		}
	}
}
