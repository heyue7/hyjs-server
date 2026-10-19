package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestAdminNewPlayerScheduleTypedConsumersAndCold publishes real per-role
// login/trade/personal-wheel plans, then consumes original protocols. Finite
// ordinary rewards avoid introducing any production acquisition configuration.
func TestAdminNewPlayerScheduleTypedConsumersAndCold(t *testing.T) {
	f, actor := adminModesFixture(t)
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	for _, init := range []func(context.Context) error{f.s.InitializeTurntable, f.s.InitializePlayerOpeningCalendar, f.s.InitializeAdminGameServers, f.s.InitializeAdminActivitySchedules, f.s.InitializeServerGameClock} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	other, err := LoginAccount(f.ctx, fmt.Sprintf("relative-other-%d", now.UnixNano()), "fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, other.Id, "角色日程乙", "hero_043"); err != nil {
		t.Fatal(err)
	}
	contexts := map[int64]context.Context{}
	for _, id := range []int64{f.id, other.Id} {
		var profile db.Profile
		if err = db.GetIns().First(&profile, id).Error; err != nil {
			t.Fatal(err)
		}
		bound, e := f.s.BindServerGameClockContext(f.ctx, &profile)
		if e != nil {
			t.Fatal(e)
		}
		contexts[id] = bound
		if err = updateProfile(bound, id, func(p *pb.Player, _ *model.GameState) error { p.Diamond = proto.Int64(500); return nil }); err != nil {
			t.Fatal(err)
		}
	}
	// Finite persisted age fixture: both roles are on their second game day.
	// This verifies a configured startDay=2 through actual native consumers.
	zone := time.FixedZone("UTC+8", 8*3600)
	local := now.In(zone)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone)
	for _, id := range []int64{f.id, other.Id} {
		anchor := db.PlayerOpeningCalendar{PlayerID: id, StartAt: today.AddDate(0, 0, -1).UnixMilli(), CreatedAt: now.UnixMilli(), Source: constUtil.PlayerOpeningCalendarSource}
		if err = db.GetIns().Save(&anchor).Error; err != nil {
			t.Fatal(err)
		}
	}
	forms := []model.AdminActivityForm{adminModeForm("login-rewards"), adminModeForm("exchange"), adminModeForm("turntable")}
	forms[0].Milestones = []model.AdminActivityMilestone{{Goal: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 7}}}}
	forms[1].Products = []model.AdminActivityProduct{{ID: "role-trade", Name: "普通道具", Index: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "item_005", Amount: 2}}, Cost: model.AdminResourceAmount{ResourceID: "diamond", Amount: 10}, DailyLimit: 1, TotalLimit: 1, Stock: 1, MinLevel: 1, DiscountBps: 10000, Weight: 1}}
	forms[2].Options.ScorePerDraw = 1
	forms[2].Options.InitialDiamondPool = 10000
	forms[2].Options.PoolContributionBps = 200
	for i := int64(0); i < 9; i++ {
		forms[2].Prizes = append(forms[2].Prizes, model.AdminActivityPrize{ID: i, ResourceID: "coin", Amount: 1, Weight: 1, Stock: 1})
	}
	calendar, err := f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil {
		t.Fatal(err)
	}
	plans := []model.AdminActivityOccurrenceDTO{}
	for _, form := range forms {
		definition, e := f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: form.Mode, Form: form, DefaultDays: 7, Enabled: true})
		if e != nil {
			t.Fatal(form.Mode, e)
		}
		startDay := int64(1)
		if form.Mode == "login-rewards" {
			startDay = 2
		}
		plans = append(plans, model.AdminActivityOccurrenceDTO{TemplateID: definition.ID, TemplateVersion: definition.Version, Mode: form.Mode, Timing: "character-day", StartDay: startDay, DurationDays: 7, StartAt: now.UnixMilli(), EndAt: now.AddDate(0, 0, 7).UnixMilli(), ClaimEndAt: now.AddDate(0, 0, 7).UnixMilli(), Enabled: true})
	}
	saved, err := f.s.AdminSaveActivitySchedule(f.ctx, actor, 1, model.AdminActivityScheduleSaveRequest{Version: calendar.Version, Items: plans})
	if err != nil {
		t.Fatal("relative publish", err)
	}
	for _, id := range []int64{f.id, other.Id} {
		if err = f.s.PrepareAdminCharacterActivitySchedules(contexts[id], id); err != nil {
			t.Fatal("prepare", id, err)
		}
	}
	if got := p1Count(t, &db.AdminCharacterActivityInstance{}, "server_id=?", 1); got != 6 {
		t.Fatal("expected exactly one native consumer per role and plan", got)
	}
	own := map[string]db.AdminCharacterActivityInstance{}
	peer := map[string]db.AdminCharacterActivityInstance{}
	for _, id := range []int64{f.id, other.Id} {
		var rows []db.AdminCharacterActivityInstance
		if err = db.GetIns().Where("player_id=?", id).Find(&rows).Error; err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			if id == f.id {
				own[row.Mode] = row
			} else {
				peer[row.Mode] = row
			}
		}
	}
	if own["login-rewards"].StartAt != today.UnixMilli() || own["exchange"].StartAt != today.AddDate(0, 0, -1).UnixMilli() {
		t.Fatal("relative starting day ignored")
	}
	if own["turntable"].ConsumerKey == peer["turntable"].ConsumerKey {
		t.Fatal("two roles share a consumer")
	}
	loginFirst := map[int64]proto.Message{}
	for _, id := range []int64{f.id, other.Id} {
		fixture := *f
		fixture.id = id
		fixture.ctx = contexts[id]
		fixture.event(t, fmt.Sprintf("relative-login-%d", id), constUtil.P1EventLoginDay, 1, now)
		messages, e := f.s.p4Welfare(contexts[id], id, &pb.CSGetLoginReward{}, 88001, now)
		if e != nil || len(messages) != 1 {
			t.Fatal("original login claim", id, e)
		}
		loginFirst[id] = messages[0]
	}
	ownTrade := own["exchange"].ConsumerKey
	if _, err = f.s.p4Commerce(contexts[other.Id], other.Id, &pb.CSActivityShopInfo{ActivityId: proto.String(ownTrade)}, 0, now); err == nil {
		t.Fatal("foreign role guessed trade instance")
	}
	request := &pb.CSGetActivityShopReward{ActivityId: proto.String(ownTrade), Index: proto.Int32(1), CostResources: []*pb.BaseResource{{Id: proto.String("diamond"), Num: proto.Int32(10)}}, GetResources: []*pb.BaseResource{{Id: proto.String("item_005"), Num: proto.Int32(2)}}}
	trade, err := f.s.p4Commerce(contexts[f.id], f.id, request, 88002, now)
	if err != nil {
		t.Fatal("original exchange", err)
	}
	peerRequest := proto.Clone(request).(*pb.CSGetActivityShopReward)
	peerRequest.ActivityId = proto.String(peer["exchange"].ConsumerKey)
	if _, err = f.s.p4Commerce(contexts[other.Id], other.Id, peerRequest, 88002, now); err != nil {
		t.Fatal("peer stock consumed", err)
	}
	wheelReq := &pb.CSPlayBigTurn{Count: proto.Int32(1)}
	wheel, err := f.s.playBigTurn(contexts[f.id], f.id, wheelReq, 88003)
	if err != nil {
		t.Fatal("original relative personal wheel", err)
	}
	if _, err = f.s.playBigTurn(contexts[other.Id], other.Id, wheelReq, 88003); err != nil {
		t.Fatal("peer personal wheel", err)
	}
	var a, b db.TurntablePersonal
	if err = db.GetIns().First(&a, "pool_id=? AND player_id=?", own["turntable"].ConsumerKey, f.id).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&b, "pool_id=? AND player_id=?", peer["turntable"].ConsumerKey, other.Id).Error; err != nil {
		t.Fatal(err)
	}
	if a.DiamondPool != 10001 || b.DiamondPool != 10001 {
		t.Fatal("personal pool contribution not independent", a.DiamondPool, b.DiamondPool)
	}
	var group sync.WaitGroup
	failures := make(chan error, 4)
	for i := 0; i < 4; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if e := f.s.PrepareAdminCharacterActivitySchedules(contexts[f.id], f.id); e != nil {
				failures <- e
				return
			}
			messages, e := f.s.playBigTurn(contexts[f.id], f.id, wheelReq, 88003)
			if e == nil && !proto.Equal(messages[0], wheel[0]) {
				e = fmt.Errorf("concurrent receipt changed")
			}
			failures <- e
		}()
	}
	group.Wait()
	close(failures)
	for e := range failures {
		if e != nil {
			t.Fatal(e)
		}
	}
	if got := p1Count(t, &db.AdminCharacterActivityInstance{}, "server_id=?", 1); got != 6 {
		t.Fatal("repeat preparation recreated native consumers", got)
	}
	for _, id := range []int64{f.id, other.Id} {
		var profile db.Profile
		if err = db.GetIns().First(&profile, id).Error; err != nil {
			t.Fatal(err)
		}
		player, game, e := decodeProfile(&profile)
		if e != nil || player.GetDiamond() != 440 || player.GetCoin() != 10008 || simpleOwned(player, game, "item_005") < 2 {
			t.Fatal("relative trade/draw conservation", id, player.GetDiamond(), player.GetCoin(), e)
		}
	}
	// Existing configuration changes must not rewrite a frozen role window.
	current, err := f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil {
		t.Fatal(err)
	}
	for i := range current.Items {
		current.Items[i].Enabled = false
	}
	if _, err = f.s.AdminSaveActivitySchedule(f.ctx, actor, 1, model.AdminActivityScheduleSaveRequest{Version: current.Version, Items: current.Items}); err != nil {
		t.Fatal("disable relative plan", err)
	}
	f.restart(t)
	if err = f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
		t.Fatal(err)
	}
	again, e := f.s.p4Commerce(contexts[f.id], f.id, request, 88002, now.AddDate(0, 0, 10))
	if e != nil || !proto.Equal(again[0], trade[0]) {
		t.Fatal("cold trade receipt lost", e)
	}
	again, e = f.s.playBigTurn(contexts[f.id], f.id, wheelReq, 88003)
	if e != nil || !proto.Equal(again[0], wheel[0]) {
		t.Fatal("cold wheel receipt lost", e)
	}
	again, e = f.s.p4Welfare(contexts[f.id], f.id, &pb.CSGetLoginReward{}, 88001, now.AddDate(0, 0, 10))
	if e != nil || !proto.Equal(again[0], loginFirst[f.id]) {
		t.Fatal("cold login receipt lost", e)
	}
	if got := p1Count(t, &db.RewardDelivery{}, "source_key LIKE ?", "turntable:%"); got != 0 {
		t.Fatal("personal-only wheel created a legacy ranking reward", got)
	}
	_ = saved
}

// TestAdminFixedCalendarRemainsSharedAcrossRoleAges verifies fixed dates and
// common inventory remain shared even when two roles have very different Day 1.
func TestAdminFixedCalendarRemainsSharedAcrossRoleAges(t *testing.T) {
	f, actor := adminModesFixture(t)
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	for _, init := range []func(context.Context) error{f.s.InitializePlayerOpeningCalendar, f.s.InitializeAdminGameServers, f.s.InitializeAdminActivitySchedules, f.s.InitializeServerGameClock} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	other, err := LoginAccount(f.ctx, fmt.Sprintf("fixed-calendar-other-%d", now.UnixNano()), "fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, other.Id, "定期日历乙", "hero_043"); err != nil {
		t.Fatal(err)
	}
	old := db.PlayerOpeningCalendar{PlayerID: f.id, StartAt: now.AddDate(0, 0, -60).UnixMilli(), CreatedAt: now.UnixMilli(), Source: constUtil.PlayerOpeningCalendarSource}
	if err = db.GetIns().Save(&old).Error; err != nil {
		t.Fatal(err)
	}
	form := adminModeForm("exchange")
	form.Products = []model.AdminActivityProduct{{ID: "shared", Name: "共用库存", Index: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "item_005", Amount: 1}}, Cost: model.AdminResourceAmount{ResourceID: "diamond", Amount: 10}, DailyLimit: 1, TotalLimit: 1, Stock: 1, MinLevel: 1, DiscountBps: 10000, Weight: 1}}
	definition, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: "共同日期", Form: form, DefaultDays: 1, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	calendar, err := f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil {
		t.Fatal(err)
	}
	start := now.Add(-time.Hour).UnixMilli()
	end := now.Add(time.Hour).UnixMilli()
	published, err := f.s.AdminSaveActivitySchedule(f.ctx, actor, 1, model.AdminActivityScheduleSaveRequest{Version: calendar.Version, Items: []model.AdminActivityOccurrenceDTO{{TemplateID: definition.ID, TemplateVersion: definition.Version, Timing: "calendar", StartAt: start, EndAt: end, ClaimEndAt: end, Enabled: true}}})
	if err != nil {
		t.Fatal(err)
	}
	if published.Items[0].StartAt != start || published.Items[0].EndAt != end || published.Items[0].Timing != "calendar" {
		t.Fatal("fixed window shifted")
	}
	_, key, _ := strings.Cut(published.Items[0].ConsumerID, ":")
	contexts := map[int64]context.Context{}
	for _, id := range []int64{f.id, other.Id} {
		var profile db.Profile
		if err = db.GetIns().First(&profile, id).Error; err != nil {
			t.Fatal(err)
		}
		bound, e := f.s.BindServerGameClockContext(f.ctx, &profile)
		if e != nil {
			t.Fatal(e)
		}
		contexts[id] = bound
		if err = updateProfile(bound, id, func(p *pb.Player, _ *model.GameState) error { p.Diamond = proto.Int64(100); return nil }); err != nil {
			t.Fatal(err)
		}
		if err = f.s.PrepareAdminCharacterActivitySchedules(bound, id); err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.p4Commerce(bound, id, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, now); err != nil {
			t.Fatal("shared calendar info follows role age", id, err)
		}
	}
	if got := p1Count(t, &db.AdminCharacterActivityInstance{}, "server_id=?", 1); got != 0 {
		t.Fatal("fixed calendar created private instances", got)
	}
	request := &pb.CSGetActivityShopReward{ActivityId: proto.String(key), Index: proto.Int32(1), CostResources: []*pb.BaseResource{{Id: proto.String("diamond"), Num: proto.Int32(10)}}, GetResources: []*pb.BaseResource{{Id: proto.String("item_005"), Num: proto.Int32(1)}}}
	if _, err = f.s.p4Commerce(contexts[f.id], f.id, request, 88004, now); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p4Commerce(contexts[other.Id], other.Id, request, 88004, now); err == nil {
		t.Fatal("fixed realm inventory silently became per-role")
	}
}
