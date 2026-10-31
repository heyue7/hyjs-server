package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func openingEarlyLoginFixture(t *testing.T, realmDaysBefore int) (*p1Fixture, *model.AdminPrincipal, context.Context) {
	t.Helper()
	f, actor := reuseFixture(t, true)
	if err := f.s.InitializeActivities(f.ctx); err != nil {
		t.Fatal(err)
	}
	zone := time.FixedZone("UTC+8", 8*3600)
	now := time.Now().In(zone)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, zone)
	if err := db.GetIns().Model(&db.AdminGameServer{}).Where("id=?", 1).Update("open_at", today.AddDate(0, 0, -realmDaysBefore).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	role, err := LoginAccount(f.ctx, "early-opening-"+p6Digest([]byte(t.Name()))[:16], "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, role.Id, "开服登录回归", "hero_043"); err != nil {
		t.Fatal(err)
	}
	f.id = role.Id
	definitions, err := f.s.AdminActivityDefinitions(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	for _, def := range definitions {
		if def.ID == "original_opening_14" {
			reusePublish(t, f, actor, def, 1, 14, "character-day")
			return f, actor, serverClockBindTest(t, f, f.id)
		}
	}
	t.Fatal("original opening definition missing")
	return nil, nil, nil
}

func openingEarlyLoginItem(t *testing.T, f *p1Fixture, ctx context.Context, day, taskID int32) *pb.OpenServer7DayItem {
	t.Helper()
	messages, err := f.s.openServerActivityAt(ctx, f.id, &pb.CSNewOpenSevenDayByTitle{Day: proto.Int32(day), Tap: proto.Int32(1)}, 0, LocalPlayNow(ctx))
	if err != nil {
		t.Fatal("query original opening tab", day, err)
	}
	for _, item := range messages[0].(*pb.SCNewOpenSevenDayByTitle).GetItemList() {
		if item.GetItemId() == taskID {
			return item
		}
	}
	t.Fatal("original opening task missing", taskID)
	return nil
}

// The role is created on realm day four. Only the three earlier gifts can use
// its actual period login; the registration day and later tiers retain dates.
func TestOpeningEarlyLoginLateRoleNativeClaimsAndCrossDay(t *testing.T) {
	f, actor, bound := openingEarlyLoginFixture(t, 3)
	for day, task := range map[int32]int32{1: 1, 2: 21, 3: 38, 4: 55} {
		if got := openingEarlyLoginItem(t, f, bound, day, task); got.GetState() != 0 || got.GetCurrentNum() != 0 {
			t.Fatal("query created login progress", day, got)
		}
	}
	if _, err := f.s.openServerActivityAt(bound, f.id, &pb.CSNewOpenSevenDayByTitle{Day: proto.Int32(5), Tap: proto.Int32(1)}, 0, LocalPlayNow(bound)); err == nil {
		t.Fatal("future tab unlocked")
	}
	if _, err := f.s.openServerActivityAt(bound, f.id, &pb.CSGetOpenServerReward{ItemId: proto.Int32(82)}, 80100, LocalPlayNow(bound)); err == nil {
		t.Fatal("future gift claimed")
	}
	before, _ := f.profile(t)
	for i := 0; i < 2; i++ {
		if err := f.s.RecordActivityLogin(bound, f.id); err != nil {
			t.Fatal("successful login", err)
		}
	}
	after, _ := f.profile(t)
	if !proto.Equal(before, after) {
		t.Fatal("login awarded resources without a claim")
	}
	var counters []db.OpenServerActivityCounter
	if err := db.GetIns().Where("player_id=? AND counter_key LIKE ?", f.id, constUtil.OpenServerActivityCounterLogin+"%").Find(&counters).Error; err != nil || len(counters) != 1 || counters[0].CounterKey != constUtil.OpenServerActivityCounterLogin+p1Day(LocalPlayNow(bound)) || counters[0].Value != 1 {
		t.Fatal("reconnect changed or fabricated dated logins", counters, err)
	}
	if got := openingEarlyLoginItem(t, f, bound, 4, 55); got.GetState() != 0 || got.GetCurrentNum() != 0 {
		t.Fatal("registration-day task condition changed", got)
	}
	for day, task := range map[int32]int32{1: 1, 2: 21, 3: 38} {
		if got := openingEarlyLoginItem(t, f, bound, day, task); got.GetState() != 1 || got.GetCurrentNum() != 1 {
			t.Fatal("real period login did not qualify earlier gift", day, got)
		}
		stamp := int64(81000 + task)
		req := &pb.CSGetOpenServerReward{ItemId: proto.Int32(task)}
		first, err := f.s.openServerActivityAt(bound, f.id, req, stamp, LocalPlayNow(bound))
		if err != nil || first[0].(*pb.SCGetOpenServerReward).GetNewItemState() != 2 {
			t.Fatal("original claim", task, err)
		}
		replay, err := f.s.openServerActivityAt(bound, f.id, req, stamp, LocalPlayNow(bound))
		if err != nil || !proto.Equal(first[0], replay[0]) {
			t.Fatal("receipt retry", task, err)
		}
		if _, err = f.s.openServerActivityAt(bound, f.id, req, stamp+1000, LocalPlayNow(bound)); err == nil {
			t.Fatal("gift claimed twice", task)
		}
	}
	f.restart(t)
	bound = serverClockBindTest(t, f, f.id)
	if got := openingEarlyLoginItem(t, f, bound, 2, 21); got.GetState() != 2 {
		t.Fatal("cold claim state", got)
	}
	reuseClockDay(t, f, actor)
	bound = serverClockBindTest(t, f, f.id)
	for day, task := range map[int32]int32{4: 55, 5: 82} {
		if got := openingEarlyLoginItem(t, f, bound, day, task); got.GetState() != 0 {
			t.Fatal("registration-day or later gift expanded", day, got)
		}
	}
	if err := f.s.RecordActivityLogin(bound, f.id); err != nil {
		t.Fatal("next-day login", err)
	}
	if got := openingEarlyLoginItem(t, f, bound, 5, 82); got.GetState() != 0 {
		t.Fatal("future-at-registration gift used earlier login", got)
	}
	if got := p1Count(t, &db.OpenServerActivityCounter{}, "player_id=? AND counter_key LIKE ?", f.id, constUtil.OpenServerActivityCounterLogin+"%"); got != 2 {
		t.Fatal("next-day actual login did not add exactly one date", got)
	}
	// A fresh request after the personal cutoff must not award anything.
	if _, err := f.s.openServerActivityAt(bound, f.id, &pb.CSGetOpenServerReward{ItemId: proto.Int32(55)}, 82000, LocalPlayNow(bound).AddDate(0, 0, 20)); err == nil {
		t.Fatal("expired activity accepted a new claim")
	}
}

// Roles created on the realm's first day continue to require the exact dated
// login for each new tier, and a reconnect cannot stand in for tomorrow.
func TestOpeningEarlyLoginFirstDayKeepsOriginalDates(t *testing.T) {
	f, actor, bound := openingEarlyLoginFixture(t, 0)
	if err := f.s.RecordActivityLogin(bound, f.id); err != nil {
		t.Fatal(err)
	}
	if got := openingEarlyLoginItem(t, f, bound, 1, 1); got.GetState() != 1 {
		t.Fatal("first-day login", got)
	}
	reuseClockDay(t, f, actor)
	bound = serverClockBindTest(t, f, f.id)
	if got := openingEarlyLoginItem(t, f, bound, 2, 21); got.GetState() != 0 {
		t.Fatal("yesterday's login met next day's original task", got)
	}
	if err := f.s.RecordActivityLogin(bound, f.id); err != nil {
		t.Fatal(err)
	}
	if got := openingEarlyLoginItem(t, f, bound, 2, 21); got.GetState() != 1 {
		t.Fatal("dated next-day login ignored", got)
	}
}

func TestOpeningEarlyLoginRejectsOutsidePeriodFacts(t *testing.T) {
	zone := time.FixedZone("UTC+8", 8*3600)
	start := time.Date(2026, 10, 10, 0, 0, 0, 0, zone)
	cfg := &db.OpenServerActivityRule{StartAt: start.UnixMilli(), EndAt: start.AddDate(0, 0, 2).UnixMilli()}
	values := map[string]int32{"login:2026-10-09": 1, "login:2026-10-12": 1}
	if got := openServerPeriodCounter(cfg, values, constUtil.OpenServerActivityCounterLogin, false); got != 0 {
		t.Fatal("outside-period login accepted", got)
	}
	values["login:2026-10-11"] = 1
	if got := openServerPeriodCounter(cfg, values, constUtil.OpenServerActivityCounterLogin, false); got != 1 {
		t.Fatal("in-period login ignored", got)
	}
}

// SQLite serializes these requests with its single test connection. MySQL's
// row-lock behavior remains a separate integration check.
func TestOpeningEarlyLoginConcurrentClaims(t *testing.T) {
	f, _, bound := openingEarlyLoginFixture(t, 3)
	if err := f.s.RecordActivityLogin(bound, f.id); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, stamp := range []int64{83001, 83002} {
		wg.Add(1)
		go func(stamp int64) {
			defer wg.Done()
			_, err := f.s.openServerActivityAt(bound, f.id, &pb.CSGetOpenServerReward{ItemId: proto.Int32(21)}, stamp, LocalPlayNow(bound))
			errs <- err
		}(stamp)
	}
	wg.Wait()
	close(errs)
	winners := 0
	for err := range errs {
		if err == nil {
			winners++
		}
	}
	if winners != 1 {
		t.Fatal("concurrent gift claims", winners)
	}
	if got := p1Count(t, &db.OpenServerActivityReceipt{}, "player_id=?", f.id); got != 1 {
		t.Fatal("concurrent receipt count", got)
	}
	if got := openingEarlyLoginItem(t, f, bound, 2, 21); got.GetState() != 2 {
		t.Fatal("concurrent claim state", got)
	}
}
