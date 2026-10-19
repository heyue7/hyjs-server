package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	pb "hyjs-server/model/protocol"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// TestAdminActivityCalendarAdoption preserves the native consumer identity and
// its prior rules while moving actual access under the published realm calendar.
func TestAdminActivityCalendarAdoption(t *testing.T) {
	f, actor := adminModesFixture(t)
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	if err := f.s.InitializeAdminGameServers(f.ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	form := adminModeForm("discount-shop")
	form.Name = "已有投放"
	form.StartAt = now - 1000
	form.EndAt = now + 86400000
	form.ClaimEndAt = form.EndAt
	form.Products = []model.AdminActivityProduct{{ID: "prior", Name: "已有商品", Index: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "item_005", Amount: 2}}, Cost: model.AdminResourceAmount{ResourceID: "diamond", Amount: 10}, DailyLimit: 1, TotalLimit: 2, Stock: -1, MinLevel: 1, DiscountBps: 10000, Weight: 1}}
	live, err := f.s.AdminSaveTypedActivity(f.ctx, actor, "", model.AdminTypedActivityRequest{AdminActivityForm: form, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	form.Name = "已停用投放"
	off, err := f.s.AdminSaveTypedActivity(f.ctx, actor, "", model.AdminTypedActivityRequest{AdminActivityForm: form, Enabled: false})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
		t.Fatal(err)
	}
	before, _, err := adminActivityRaw(db.GetIns().WithContext(f.ctx), live.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.AdoptAdminActivityCalendar(f.ctx); err != nil {
		t.Fatal(err)
	}
	calendar, err := f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil || !calendar.Managed {
		t.Fatal("calendar not adopted", calendar, err)
	}
	foundLive, foundOff := false, false
	for _, item := range calendar.Items {
		if item.ConsumerID == live.ID {
			foundLive = item.Enabled && item.StartAt == live.Form.StartAt && item.EndAt == live.Form.EndAt && item.Form.Products[0].Cost.Amount == 10
			_, key, _ := strings.Cut(live.ID, ":")
			allowed, e := adminActivityScheduleAccess(db.GetIns().WithContext(f.ctx), key)
			if e != nil || !allowed {
				t.Fatal("adopted native entry inaccessible", allowed, e)
			}
			foreign := &GameService{ServerID: 2}
			allowed, e = adminActivityScheduleAccess(db.GetIns().WithContext(context.WithValue(f.ctx, constUtil.GameServiceContextKey, foreign)), key)
			if e != nil || allowed {
				t.Fatal("foreign realm accessed adopted entry", allowed, e)
			}
		}
		if item.ConsumerID == off.ID {
			foundOff = !item.Enabled
		}
	}
	if !foundLive || !foundOff {
		t.Fatal("live/off state changed", foundLive, foundOff)
	}
	after, _, err := adminActivityRaw(db.GetIns().WithContext(f.ctx), live.ID)
	if err != nil || adminVersion(before) != adminVersion(after) {
		t.Fatal("native source rewritten", err)
	}
	if err = f.s.AdoptAdminActivityCalendar(f.ctx); err != nil {
		t.Fatal(err)
	}
	repeated, err := f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil || repeated.Version != calendar.Version || len(repeated.Items) != len(calendar.Items) {
		t.Fatal("adoption not idempotent", err)
	}
	for index := range repeated.Items {
		if repeated.Items[index].ConsumerID == live.ID {
			repeated.Items[index].Enabled = false
		}
	}
	saved, err := f.s.AdminSaveActivitySchedule(f.ctx, actor, 1, model.AdminActivityScheduleSaveRequest{Version: repeated.Version, Items: repeated.Items})
	if err != nil {
		t.Fatal("calendar could not disable adopted source", err)
	}
	if err = f.s.AdoptAdminActivityCalendar(f.ctx); err != nil {
		t.Fatal(err)
	}
	stable, err := f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil || stable.Version != saved.Version {
		t.Fatal("startup overwrote operator calendar", err)
	}
}

// TestAdminActivityOpeningCharacterDayAfterAdoption keeps a normal new role's
// original Day 1 available even when the realm's old opening window has ended.
// The actual original protocol must then reject access after backend disabling.
func TestAdminActivityOpeningCharacterDayAfterAdoption(t *testing.T) {
	f, actor := adminModesFixture(t)
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	loadOpenServerTaskFixture(t, f.s)
	if err := f.s.InitializeOpenServerActivity(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializePlayerOpeningCalendar(f.ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	zone := time.FixedZone("UTC+8", 8*3600)
	local := now.In(zone)
	oldStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone).AddDate(0, 0, -40)
	if err := db.GetIns().Model(&db.OpenServerActivityRule{}).Where("id=?", constUtil.OpenServerActivityID).Updates(map[string]any{"start_at": oldStart.UnixMilli(), "end_at": oldStart.AddDate(0, 0, 14).UnixMilli(), "reward_end_at": oldStart.AddDate(0, 0, 15).UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	account, err := LoginAccount(f.ctx, "opening-calendar-new-role", "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, account.Id, "新角色日程", "hero_043"); err != nil {
		t.Fatal(err)
	}
	if err = f.s.InitializeAdminGameServers(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err = f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err = f.s.AdoptAdminActivityCalendar(f.ctx); err != nil {
		t.Fatal(err)
	}
	messages, err := f.s.openServerActivityAt(f.ctx, account.Id, &pb.CSOpenServerActivity{DayType: proto.Int32(7)}, 0, now)
	if err != nil || len(messages) != 1 {
		t.Fatal("new role lost its original opening entry after adoption", err)
	}
	if messages[0].(*pb.SCOpenServerActivity).GetCurrentDay() != 1 {
		t.Fatal("new role did not start on Day 1")
	}
	calendar, err := f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for index := range calendar.Items {
		if calendar.Items[index].Mode == "opening-14" {
			found = true
			if calendar.Items[index].Timing != "character-day" {
				t.Fatal("relative opening has absolute calendar label")
			}
			calendar.Items[index].Enabled = false
		}
	}
	if !found {
		t.Fatal("opening programme absent from managed calendar")
	}
	if _, err = f.s.AdminSaveActivitySchedule(f.ctx, actor, 1, model.AdminActivityScheduleSaveRequest{Version: calendar.Version, Items: calendar.Items}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.openServerActivityAt(f.ctx, account.Id, &pb.CSOpenServerActivity{DayType: proto.Int32(7)}, 0, now); err == nil {
		t.Fatal("backend disabled opening remained accessible")
	}
	var anchor db.PlayerOpeningCalendar
	if err = db.GetIns().First(&anchor, "player_id=?", account.Id).Error; err != nil || anchor.StartAt != time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone).UnixMilli() {
		t.Fatal("backend scheduling rewrote role anchor", err)
	}
}

// TestAdminOpeningRealmNaturalUnlockAndFullRoleValidity follows the two clocks:
// a late joiner sees the realm's eleventh unlock day and keeps all configured
// thirty participation days. The fourteen original task tiers are unchanged.
func TestAdminOpeningRealmNaturalUnlockAndFullRoleValidity(t *testing.T) {
	f, actor := adminModesFixture(t)
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	loadOpenServerTaskFixture(t, f.s)
	for _, init := range []func(context.Context) error{f.s.InitializeOpenServerActivity, f.s.InitializePlayerOpeningCalendar, f.s.InitializeAdminGameServers, f.s.InitializeAdminActivitySchedules} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	zone := time.FixedZone("UTC+8", 8*3600)
	local := now.In(zone)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone)
	opened := today.AddDate(0, 0, -10).Add(20 * time.Hour)
	if err := db.GetIns().Model(&db.AdminGameServer{}).Where("id=?", 1).Update("open_at", opened.UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	account, err := LoginAccount(f.ctx, "realm-unlock-late-player", "fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, account.Id, "晚加入完整时长", "hero_043"); err != nil {
		t.Fatal(err)
	}
	definitionID := "original_opening_14"
	definitions, err := f.s.AdminActivityDefinitions(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	version := ""
	for _, definition := range definitions {
		if definition.ID == definitionID {
			version = definition.Version
		}
	}
	calendar, err := f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.AdminSaveActivitySchedule(f.ctx, actor, 1, model.AdminActivityScheduleSaveRequest{Version: calendar.Version, Items: []model.AdminActivityOccurrenceDTO{{TemplateID: definitionID, TemplateVersion: version, Mode: "opening-14", Timing: "character-day", StartDay: 1, DurationDays: 30, ClaimDays: 0, StartAt: today.UnixMilli(), EndAt: today.AddDate(0, 0, 30).UnixMilli(), ClaimEndAt: today.AddDate(0, 0, 30).UnixMilli(), Enabled: true}}})
	if err != nil {
		t.Fatal("configurable role opening duration", err)
	}
	result, err := f.s.openServerActivityAt(f.ctx, account.Id, &pb.CSOpenServerActivity{DayType: proto.Int32(14)}, 0, now)
	if err != nil || len(result) != 1 {
		t.Fatal("late joiner's original activity", err)
	}
	reply := result[0].(*pb.SCOpenServerActivity)
	if reply.GetCurrentDay() != 11 || reply.GetActivityEndTime() != today.AddDate(0, 0, 30).UnixMilli() || reply.GetGetRewardEndTime() != reply.GetActivityEndTime() {
		t.Fatal("realm unlock and role lifetime mixed", reply)
	}
	states, e := f.s.openServerOpenStatesAt(f.ctx, account.Id, now)
	if e != nil || len(states) < 2 || states[0].GetIsOpen() != 1 || states[1].GetIsOpen() != 1 {
		t.Fatal("late joiner lost first seven-day entrance", states, e)
	}
	firstSeven, e := f.s.openServerActivityAt(f.ctx, account.Id, &pb.CSOpenServerActivity{DayType: proto.Int32(7)}, 0, now)
	if e != nil || firstSeven[0].(*pb.SCOpenServerActivity).GetCurrentDay() != 7 {
		t.Fatal("seven-tab client received out-of-range day", e)
	}
	cfg, err := loadPlayerOpenServerRule(db.GetIns().WithContext(f.ctx), account.Id)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UnlockStartAt != today.AddDate(0, 0, -10).UnixMilli() || cfg.StartAt != today.UnixMilli() {
		t.Fatal("natural-day anchor or role anchor wrong", cfg)
	}
	if openServerDay(cfg, today.Add(time.Minute)) != 11 {
		t.Fatal("unlock used elapsed 24 hours instead of Beijing date")
	}
	early := *cfg
	early.UnlockStartAt = today.UnixMilli()
	if openServerDay(&early, today.Add(23*time.Hour)) != 1 || openServerDay(&early, today.AddDate(0, 0, 1)) != 2 {
		t.Fatal("early player's next natural day did not unlock")
	}
	if got := p1Count(t, &db.OpenServerActivityTask{}, "open_day BETWEEN ? AND ?", 1, 14); got != 249 {
		t.Fatal("original task tiers changed", got)
	}
}

// Published relative timing cannot be rewritten through a future viewport, and
// replacing a stopped programme never rebinds an already-participating role.
func TestAdminOpeningPublishedFreezeAndAcceptedWindow(t *testing.T) {
	f, actor := adminModesFixture(t)
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	loadOpenServerTaskFixture(t, f.s)
	for _, init := range []func(context.Context) error{f.s.InitializeOpenServerActivity, f.s.InitializePlayerOpeningCalendar, f.s.InitializeAdminGameServers, f.s.InitializeAdminActivitySchedules} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	role, err := LoginAccount(f.ctx, "opening-frozen-role", "fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, role.Id, "接受旧排期", "hero_043"); err != nil {
		t.Fatal(err)
	}
	definitions, err := f.s.AdminActivityDefinitions(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	version := ""
	for _, d := range definitions {
		if d.ID == "original_opening_14" {
			version = d.Version
		}
	}
	calendar, err := f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil {
		t.Fatal(err)
	}
	future := now.AddDate(0, 0, 90)
	item := model.AdminActivityOccurrenceDTO{TemplateID: "original_opening_14", TemplateVersion: version, Mode: "opening-14", Timing: "character-day", StartDay: 1, DurationDays: 30, ClaimDays: 1, StartAt: future.UnixMilli(), EndAt: future.AddDate(0, 0, 30).UnixMilli(), ClaimEndAt: future.AddDate(0, 0, 31).UnixMilli(), Enabled: true}
	calendar, err = f.s.AdminSaveActivitySchedule(f.ctx, actor, 1, model.AdminActivityScheduleSaveRequest{Version: calendar.Version, Items: []model.AdminActivityOccurrenceDTO{item}})
	if err != nil {
		t.Fatal(err)
	}
	if len(calendar.Items) != 1 || !calendar.Items[0].Frozen {
		t.Fatal("future viewport left published relative opening mutable")
	}
	result, err := f.s.openServerActivityAt(f.ctx, role.Id, &pb.CSOpenServerActivity{DayType: proto.Int32(7)}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	original := result[0].(*pb.SCOpenServerActivity)
	if err = recordOpenServerCounterAt(db.GetIns(), role.Id, constUtil.OpenServerActivityCounterLogin+p1Day(now), 1, now); err != nil {
		t.Fatal(err)
	}
	claimRequest := &pb.CSGetOpenServerReward{ItemId: proto.Int32(1)}
	claimed, err := f.s.openServerActivityAt(f.ctx, role.Id, claimRequest, 9751, now)
	if err != nil {
		t.Fatal("original opening claim", err)
	}
	changed := append([]model.AdminActivityOccurrenceDTO{}, calendar.Items...)
	changed[0].DurationDays = 45
	if _, err = f.s.AdminSaveActivitySchedule(f.ctx, actor, 1, model.AdminActivityScheduleSaveRequest{Version: calendar.Version, Items: changed}); err == nil {
		t.Fatal("accepted opening duration rewritten")
	}
	if _, err = f.s.AdminSaveActivitySchedule(f.ctx, actor, 1, model.AdminActivityScheduleSaveRequest{Version: calendar.Version, Items: []model.AdminActivityOccurrenceDTO{}}); err == nil {
		t.Fatal("published opening removed")
	}
	calendar.Items[0].Enabled = false
	item.DurationDays = 45
	item.EndAt = future.AddDate(0, 0, 45).UnixMilli()
	item.ClaimEndAt = future.AddDate(0, 0, 46).UnixMilli()
	calendar, err = f.s.AdminSaveActivitySchedule(f.ctx, actor, 1, model.AdminActivityScheduleSaveRequest{Version: calendar.Version, Items: append(calendar.Items, item)})
	if err != nil {
		t.Fatal("disable and replace", err)
	}
	cfg, err := loadPlayerOpenServerRule(db.GetIns().WithContext(f.ctx), role.Id)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Enabled || cfg.EndAt != original.GetActivityEndTime() || cfg.RewardEndAt != original.GetGetRewardEndTime() {
		t.Fatal("old role rebound to replacement", cfg)
	}
	if _, err = f.s.openServerActivityAt(f.ctx, role.Id, &pb.CSOpenServerActivity{DayType: proto.Int32(7)}, 0, now); err == nil {
		t.Fatal("disabled accepted programme accessible")
	}
	replayed, err := f.s.openServerActivityAt(f.ctx, role.Id, claimRequest, 9751, now)
	if err != nil || !proto.Equal(replayed[0], claimed[0]) {
		t.Fatal("disabled source lost original claim receipt", err)
	}
	if _, err = f.s.openServerActivityAt(f.ctx, role.Id, claimRequest, 9752, now); err == nil {
		t.Fatal("replacement reissued original reward")
	}
	peer, err := LoginAccount(f.ctx, "opening-new-after-replacement", "fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, peer.Id, "接受新排期", "hero_043"); err != nil {
		t.Fatal(err)
	}
	peerResult, err := f.s.openServerActivityAt(f.ctx, peer.Id, &pb.CSOpenServerActivity{DayType: proto.Int32(7)}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if peerResult[0].(*pb.SCOpenServerActivity).GetActivityEndTime()-original.GetActivityEndTime() != 15*86400000 {
		t.Fatal("new role did not use replacement duration")
	}
	if got := p1Count(t, &db.AdminCharacterActivityInstance{}, "player_id=? AND mode=?", role.Id, "opening-14"); got != 1 {
		t.Fatal("role accepted multiple opening programmes", got)
	}
}
