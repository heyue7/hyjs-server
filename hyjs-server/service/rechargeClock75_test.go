package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func recharge75Fixture(t *testing.T) *p1Fixture {
	t.Helper()
	f := p1Setup(t)
	if err := db.GetIns().AutoMigrate(&db.PaymentOrder{}, &db.LocalPlayPaymentTime{}); err != nil {
		t.Fatal(err)
	}
	return f
}

func recharge75Context(owner, days int64) context.Context {
	ctx := context.WithValue(context.Background(), constUtil.LocalPlayBoundKey, true)
	ctx = context.WithValue(ctx, constUtil.LocalPlayOwnerKey, owner)
	return context.WithValue(ctx, constUtil.LocalPlayOffsetDaysKey, days)
}

func recharge75Order(t *testing.T, f *p1Fixture, key, status string, at time.Time, cents int64) db.PaymentOrder {
	t.Helper()
	order := db.PaymentOrder{ID: key, PlayerID: f.id, RequestKey: at.UnixNano(), RequestHash: key, ProductID: "fixture", Snapshot: "{}", Status: status, CreatedAt: at.Add(-time.Minute).UnixMilli(), ExpiresAt: at.Add(time.Hour).UnixMilli(), ConfirmedAt: at.UnixMilli(), Cents: cents, Source: constUtil.P3PaymentSource}
	if err := db.GetIns().Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	return order
}

func TestRechargeClock75FrozenOrderIndependentOfReadContext(t *testing.T) {
	f := recharge75Fixture(t)
	real := time.Now().Add(-time.Minute)
	order := recharge75Order(t, f, "recharge75-frozen", constUtil.P3PaymentConfirmed, real, 600)
	game := real.AddDate(0, 0, 7)
	projection := db.LocalPlayPaymentTime{OrderID: order.ID, PlayerID: f.id, RealAt: real.UnixMilli(), GameAt: game.UnixMilli(), OffsetDays: 7}
	if err := db.GetIns().Create(&projection).Error; err != nil {
		t.Fatal(err)
	}
	for _, ctx := range []context.Context{f.ctx, recharge75Context(f.id, 0), recharge75Context(f.id, 7), recharge75Context(f.id, -7)} {
		orders, err := localPlayPaymentOrders(db.GetIns().WithContext(ctx), f.id, game.Add(-time.Second).UnixMilli(), game.Add(time.Second).UnixMilli())
		if err != nil || len(orders) != 1 || orders[0].ConfirmedAt != game.UnixMilli() {
			t.Fatalf("frozen paid calendar changed with reader context: orders=%v err=%v", orders, err)
		}
		orders, err = localPlayPaymentOrders(db.GetIns().WithContext(ctx), f.id, real.Add(-time.Second).UnixMilli(), real.Add(time.Second).UnixMilli())
		if err != nil || len(orders) != 0 {
			t.Fatalf("frozen future payment leaked into real-date activity: orders=%v err=%v", orders, err)
		}
	}
}

func TestRechargeClock75UnshiftedConfirmationAlsoFreezesCalendar(t *testing.T) {
	f := recharge75Fixture(t)
	real := time.Now().Add(-time.Minute)
	order := recharge75Order(t, f, "recharge75-no-offset", constUtil.P3PaymentConfirmed, real, 600)
	if err := localPlayRecordPaymentTime(db.GetIns().WithContext(f.ctx), &order, real); err != nil {
		t.Fatal(err)
	}
	var projection db.LocalPlayPaymentTime
	if err := db.GetIns().First(&projection, "order_id=?", order.ID).Error; err != nil || projection.GameAt != real.UnixMilli() || projection.RealAt != real.UnixMilli() || projection.OffsetDays != 0 {
		t.Fatal("successful unshifted payment lacks immutable calendar attribution", projection, err)
	}
}

func TestRechargeClock75WindowBoundariesLegacyAndCold(t *testing.T) {
	f := recharge75Fixture(t)
	start := time.Now().Add(-time.Hour)
	end := start.Add(time.Hour)
	for _, test := range []struct {
		key    string
		at     time.Time
		status string
		cents  int64
	}{
		{"before", start.Add(-time.Millisecond), constUtil.P3PaymentConfirmed, 100},
		{"start", start, constUtil.P3PaymentConfirmed, 200},
		{"last", end.Add(-time.Millisecond), constUtil.P3PaymentConfirmed, 300},
		{"end", end, constUtil.P3PaymentConfirmed, 400},
		{"pending", start.Add(time.Minute), constUtil.P3PaymentPending, 500},
		{"failed", start.Add(2 * time.Minute), constUtil.P3PaymentFailed, 600},
		{"refunded", start.Add(3 * time.Minute), "refunded", 700},
	} {
		recharge75Order(t, f, "recharge75-boundary-"+test.key, test.status, test.at, test.cents)
	}
	// Legacy rows have no frozen projection. They keep their original real date,
	// even when the reader has since moved seven days into another activity.
	for pass := 0; pass < 2; pass++ {
		for _, days := range []int64{-7, 0, 7} {
			orders, err := localPlayPaymentOrders(db.GetIns().WithContext(recharge75Context(f.id, days)), f.id, start.UnixMilli(), end.UnixMilli())
			if err != nil || len(orders) != 2 || orders[0].ID != "recharge75-boundary-start" || orders[1].ID != "recharge75-boundary-last" {
				t.Fatal("half-open period/confirmed-only legacy attribution changed", orders, err)
			}
			orders, err = localPlayPaymentOrders(db.GetIns().WithContext(recharge75Context(f.id, days)), f.id, start.AddDate(0, 0, 7).UnixMilli(), end.AddDate(0, 0, 7).UnixMilli())
			if err != nil || len(orders) != 0 {
				t.Fatal("current offset retroactively shifted a legacy order", orders, err)
			}
		}
		f.restart(t)
	}
}

func TestRechargeClock75FirstViewCountsOnlyConfirmedWindowOrders(t *testing.T) {
	f := recharge75Fixture(t)
	now := time.Now()
	start := now.Add(-time.Hour)
	end := now.Add(time.Hour)
	recharge75Order(t, f, "recharge75-prior", constUtil.P3PaymentConfirmed, start.Add(-time.Millisecond), 9000)
	recharge75Order(t, f, "recharge75-in-window", constUtil.P3PaymentConfirmed, now.Add(-time.Minute), 600)
	recharge75Order(t, f, "recharge75-pending", constUtil.P3PaymentPending, now.Add(-2*time.Minute), 9000)
	recharge75Order(t, f, "recharge75-failed", constUtil.P3PaymentFailed, now.Add(-3*time.Minute), 9000)
	recharge75Order(t, f, "recharge75-refunded", "refunded", now.Add(-4*time.Minute), 9000)
	cfg := db.PeriodInstance{ID: "rechargeTotal_p4_recharge75", Title: "本期累充", Enabled: true, Kind: "payment_amount", Mode: "cumulative", Goal: 600, Rewards: "coin,17", StartAt: start.UnixMilli(), EndAt: end.UnixMilli(), RewardEndAt: end.Add(time.Hour).UnixMilli(), Source: "isolated confirmed-order regression"}
	if err := db.GetIns().Create(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		messages, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(cfg.ID)})
		if err != nil {
			t.Fatal(err)
		}
		item := messages[0].(*pb.SCActivityInfo).GetItemList()[0]
		if item.GetLeft() != 600 || item.GetStatus() != 1 {
			t.Fatal("first view lost confirmed in-window payment or imported unpaid/prior orders", item)
		}
	}
}

func TestRechargeClock75PublicationGateAndHistoricalClaimPreservation(t *testing.T) {
	f := recharge75Fixture(t)
	now := time.Now()
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	if err := db.GetIns().AutoMigrate(&db.AdminActivityOccurrence{}); err != nil {
		t.Fatal(err)
	}
	cfg := db.PeriodInstance{ID: "rechargeTotal_p4_admin_schedule_recharge75", Title: "本期累充", Enabled: true, Kind: "payment_amount", Mode: "cumulative", Goal: 1000, Rewards: "coin,17", StartAt: now.Add(-2 * time.Hour).UnixMilli(), EndAt: now.Add(time.Hour).UnixMilli(), RewardEndAt: now.Add(2 * time.Hour).UnixMilli(), Source: "isolated publication regression"}
	if err := db.GetIns().Create(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	publication := now.Add(-30 * time.Minute).UnixMilli()
	occ := db.AdminActivityOccurrence{ID: "recharge75-publication", ServerID: f.s.OriginalServerID(), TemplateID: "recharge75", Mode: "recharge-total", ConsumerKey: cfg.ID, ConsumerID: "period:" + cfg.ID, Enabled: true, FirstPublishedAt: publication, StartAt: cfg.StartAt, EndAt: cfg.EndAt, ClaimEndAt: cfg.RewardEndAt, FormJSON: []byte(`{"repeatPolicy":"repeat"}`)}
	if err := db.GetIns().Create(&occ).Error; err != nil {
		t.Fatal(err)
	}
	recharge75Order(t, f, "recharge75-pre-publication", constUtil.P3PaymentConfirmed, now.Add(-45*time.Minute), 9000)
	paid := recharge75Order(t, f, "recharge75-before-view", constUtil.P3PaymentConfirmed, now.Add(-10*time.Minute), 600)
	snapshot, _ := json.Marshal(cfg)
	progress := db.PeriodProgress{InstanceID: cfg.ID, PlayerID: f.id, Snapshot: snapshot, Value: 9600}
	if err := db.GetIns().Create(&progress).Error; err != nil {
		t.Fatal(err)
	}
	var delivery *db.RewardDelivery
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		var err error
		delivery, err = f.s.EnqueueRewardNotice(tx, f.id, "activity:"+cfg.ID, cfg.Title, cfg.Source, cfg.Rewards, cfg.RewardEndAt)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	read := func() *pb.ActivityItem {
		t.Helper()
		messages, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(cfg.ID)})
		if err != nil {
			t.Fatal(err)
		}
		return messages[0].(*pb.SCActivityInfo).GetItemList()[0]
	}
	if item := read(); item.GetLeft() != 600 || item.GetStatus() != 0 {
		t.Fatal("backdated window imported pre-publication payment", item)
	}
	if points := inventoryPoints(t, f); points[constUtil.RedPointRewardCenter] != 0 {
		t.Fatal("unqualified payment attachment kept empty reward center red", points)
	}
	claim := &pb.CSActivityReward{ActivityId: proto.String(cfg.ID), Index: proto.Int32(1), GetResources: []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(17)}}}
	if _, err := f.s.P1Activity(f.ctx, f.id, claim); err == nil {
		t.Fatal("old unearned attachment bypassed direct activity qualification")
	}
	center, err := f.s.RewardCenter(f.ctx, f.id, &pb.CSQueryUserReward{})
	if err != nil || len(center[0].(*pb.SCQueryUserReward).GetRewardItemInfoList()) != 0 {
		t.Fatal("unearned payment attachment remained visible at reward center", center, err)
	}
	if _, err := f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{delivery.ID}}); err == nil {
		t.Fatal("reward center bypassed current-period qualification")
	}
	// More than one protocol page of rejected historical notices must not hide
	// a later valid ordinary attachment or leave its red point cleared.
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		for i := 0; i <= constUtil.P1RewardMaximumBatch; i++ {
			if _, err := f.s.EnqueueReward(tx, f.id, fmt.Sprintf("activity:%s:tier:stale75-%d", cfg.ID, i), "stale test notice", "", "coin,1", cfg.RewardEndAt); err != nil {
				return err
			}
		}
		_, err := f.s.EnqueueReward(tx, f.id, "recharge75:ordinary-after-stale", "ordinary test reward", "", "coin,3", cfg.RewardEndAt)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	center, err = f.s.RewardCenter(f.ctx, f.id, &pb.CSQueryUserReward{})
	if err != nil {
		t.Fatal(err)
	}
	visible := center[0].(*pb.SCQueryUserReward).GetRewardItemInfoList()
	if len(visible) != 1 || visible[0].GetRewardTitle() != "ordinary test reward" {
		t.Fatal("stale payment page hid later ordinary reward", visible)
	}
	if points := inventoryPoints(t, f); points[constUtil.RedPointRewardCenter] != 1 {
		t.Fatal("ordinary reward after rejected payment page lost red point", points)
	}
	if _, err := f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{visible[0].GetRewardId()}}); err != nil {
		t.Fatal(err)
	}
	if points := inventoryPoints(t, f); points[constUtil.RedPointRewardCenter] != 0 {
		t.Fatal("empty eligible center retained red point", points)
	}
	additional := recharge75Order(t, f, "recharge75-new-qualifying", constUtil.P3PaymentConfirmed, now.Add(-5*time.Minute), 500)
	if item := read(); item.GetLeft() != 1000 || item.GetStatus() != 1 {
		t.Fatal("valid payment before first view did not qualify", item)
	}
	if points := inventoryPoints(t, f); points[constUtil.RedPointRewardCenter] != 1 {
		t.Fatal("qualified actual payment reward lost shared red point", points)
	}
	// A historical recipient's frozen threshold remains the claim contract even
	// if a later operating change edits the live template for future recipients.
	if err := db.GetIns().Model(&db.PeriodInstance{}).Where("id=?", cfg.ID).Update("goal", 2000).Error; err != nil {
		t.Fatal(err)
	}
	before, _ := f.profile(t)
	for i := 0; i < 2; i++ {
		if _, err := f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{delivery.ID}}); err != nil {
			t.Fatal(err)
		}
	}
	after, _ := f.profile(t)
	if after.GetCoin() != before.GetCoin()+17 {
		t.Fatal("repeated reward center request changed payout", before.GetCoin(), after.GetCoin())
	}
	// A later authoritative status change may lower unclaimed qualification;
	// it does not erase or recover a historical already-paid reward.
	if err := db.GetIns().Model(&db.PaymentOrder{}).Where("id IN ?", []string{paid.ID, additional.ID}).Update("status", "refunded").Error; err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	if item := read(); item.GetLeft() != 0 || item.GetStatus() != 2 {
		t.Fatal("claimed history was erased after cold authoritative reconciliation", item)
	}
	if _, err := f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{delivery.ID}}); err != nil {
		t.Fatal("already claimed receipt lost idempotent retry", err)
	}
	final, _ := f.profile(t)
	if final.GetCoin() != after.GetCoin() {
		t.Fatal("historical claim replayed or reclaimed property")
	}
}

func TestRechargeClock75CharacterUsesPublicationAndNaturalBirth(t *testing.T) {
	f := recharge75Fixture(t)
	if err := db.GetIns().AutoMigrate(&db.AdminActivityOccurrence{}, &db.AdminCharacterActivityInstance{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	published := now.Add(-2 * time.Hour).UnixMilli()
	birth := now.Add(-time.Hour).UnixMilli()
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", f.id).Update("created_at", birth).Error; err != nil {
		t.Fatal(err)
	}
	source := db.AdminActivityOccurrence{ID: "recharge75-character-source", ServerID: 1, ConsumerKey: "source-recharge75", ConsumerID: "period:source-recharge75", FirstPublishedAt: published, FormJSON: []byte(`{}`)}
	issued := db.AdminCharacterActivityInstance{PlayerID: f.id, OccurrenceID: source.ID, ServerID: 1, ConsumerKey: "rechargeTotal_p4_admin_character_recharge75", ConsumerID: "period:rechargeTotal_p4_admin_character_recharge75", FormJSON: []byte(`{}`)}
	if err := db.GetIns().Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&issued).Error; err != nil {
		t.Fatal(err)
	}
	if floor, err := rechargeActivityRealStart75(db.GetIns(), f.id, issued.ConsumerKey); err != nil || floor != birth {
		t.Fatal("character period failed to preserve natural birth", floor, err)
	}
	later := now.Add(-30 * time.Minute).UnixMilli()
	if err := db.GetIns().Model(&source).Update("first_published_at", later).Error; err != nil {
		t.Fatal(err)
	}
	if floor, err := rechargeActivityRealStart75(db.GetIns(), f.id, issued.ConsumerKey); err != nil || floor != later {
		t.Fatal("backdated character period bypassed source publication", floor, err)
	}
	if _, err := rechargeActivityRealStart75(db.GetIns(), f.id+1, issued.ConsumerKey); err == nil {
		t.Fatal("foreign character period accepted another player's orders")
	}
}

func TestRechargeClock75AuditedPublicationBackfillIsExplicitAndIdempotent(t *testing.T) {
	f, actor := adminModesFixture(t)
	if err := db.GetIns().AutoMigrate(&db.AdminActivityOccurrence{}); err != nil {
		t.Fatal(err)
	}
	form := model.AdminActivityForm{Source: "成长投放V12 / 2026-10-26"}
	raw, _ := json.Marshal(form)
	row := db.AdminActivityOccurrence{ID: "recharge75-audited", ServerID: 1, ConsumerKey: "audited-recharge75", ConsumerID: "period:audited-recharge75", FormJSON: raw}
	other := db.AdminActivityOccurrence{ID: "recharge75-other", ServerID: 1, ConsumerKey: "other-recharge75", ConsumerID: "period:other-recharge75", FormJSON: []byte(`{"source":"older-history"}`)}
	if err := db.GetIns().Create(&[]db.AdminActivityOccurrence{row, other}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.BackfillGrowth75PublishedAt(f.ctx, actor); err == nil {
		t.Fatal("backfill accepted missing real publication evidence")
	}
	proof := db.AdminAudit{ID: 321, Time: 1793026861377, Action: "activity.schedule.save", Target: "1", Detail: "realm=1 revision=10 slots=820"}
	if err := db.GetIns().Create(&proof).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := f.s.BackfillGrowth75PublishedAt(f.ctx, actor); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().First(&row, "id=?", row.ID).Error; err != nil || row.FirstPublishedAt != proof.Time {
		t.Fatal("real publication boundary was not recovered", row.FirstPublishedAt, err)
	}
	if err := db.GetIns().First(&other, "id=?", other.ID).Error; err != nil || other.FirstPublishedAt != 0 {
		t.Fatal("unrelated historical activity was rewritten", other.FirstPublishedAt, err)
	}
	if count := p1Count(t, &db.AdminAudit{}, "action=?", "growth75.activity.publication"); count != 1 {
		t.Fatal("backfill replay wrote another audit", count)
	}
}

func TestRechargeClock75EventFailureRollsBackOriginalPaymentAndWallet(t *testing.T) {
	f, id := p3DemoProviderFixture(t)
	f.id = id
	p3DemoProviderFund(t, f, id, "recharge75-real-wallet-funding", 1000)
	beforeWallet, err := f.s.LocalWalletSnapshot(f.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	beforePlayer, beforeGame := f.profile(t)
	real := time.Now()
	messages, err := f.s.generatePayment(f.ctx, id, p3DemoProviderRequest(t, "djsy.wuyou.andp0006"), 751001, real)
	if err != nil {
		t.Fatal(err)
	}
	orderID := messages[0].(*pb.SCGenerateOrder).GetOrderId()
	// A finite invalid activity fixture forces the real RecordP1Event validation
	// stage to fail after the original wallet debit and confirmed status save.
	cfg := db.PeriodInstance{ID: "rechargeTotal_p4_rollback75", Title: "回滚检查", Kind: "payment_amount", Mode: "cumulative", Enabled: true, Goal: 0, Rewards: "coin,17", StartAt: real.Add(-time.Hour).UnixMilli(), EndAt: real.Add(time.Hour).UnixMilli(), RewardEndAt: real.Add(2 * time.Hour).UnixMilli(), Source: "isolated event-stage rollback fixture"}
	if err := db.GetIns().Create(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	trusted := context.WithValue(f.ctx, constUtil.P3TestPaymentContextKey, true)
	trusted = context.WithValue(trusted, constUtil.LocalWalletPaymentContextKey, true)
	if _, err := f.s.confirmTestPaymentAt(trusted, orderID, "recharge75-rollback-event", 600, constUtil.P3PaymentConfirmed, real); err == nil {
		t.Fatal("event-stage failure unexpectedly committed payment")
	}
	var order db.PaymentOrder
	if err := db.GetIns().First(&order, "id=?", orderID).Error; err != nil || order.Status != constUtil.P3PaymentPending || order.ConfirmedAt != 0 || order.Credited != 0 {
		t.Fatal("event-stage failure retained confirmed order fields", order, err)
	}
	for _, check := range []struct {
		table any
		where string
		args  []any
	}{
		{&db.LocalPlayPaymentTime{}, "order_id=?", []any{orderID}},
		{&db.PaymentConfirmation{}, "order_id=?", []any{orderID}},
		{&db.LocalWalletEntry{}, "order_id=?", []any{orderID}},
		{&db.EventFact{}, "player_id=? AND source_key=?", []any{id, "payment:" + orderID}},
		{&db.PeriodProgress{}, "player_id=? AND instance_id=?", []any{id, cfg.ID}},
		{&db.RewardDelivery{}, "player_id=? AND source_key=?", []any{id, "activity:" + cfg.ID}},
	} {
		if count := p1Count(t, check.table, check.where, check.args...); count != 0 {
			t.Fatal("event failure retained financial/activity side effect", check.table, count)
		}
	}
	afterWallet, err := f.s.LocalWalletSnapshot(f.ctx, id)
	if err != nil || afterWallet.BalanceCents != beforeWallet.BalanceCents || afterWallet.Revision != beforeWallet.Revision {
		t.Fatal("event failure did not roll back actual wallet debit", beforeWallet, afterWallet, err)
	}
	afterPlayer, afterGame := f.profile(t)
	if !proto.Equal(beforePlayer, afterPlayer) || string(adminJSON(beforeGame)) != string(adminJSON(afterGame)) {
		t.Fatal("event failure retained grants/VIP/player changes")
	}
	if err := db.GetIns().Model(&cfg).Update("goal", 600).Error; err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	for i := 0; i < 2; i++ {
		if _, err := f.s.confirmTestPaymentAt(trusted, orderID, "recharge75-rollback-event", 600, constUtil.P3PaymentConfirmed, real); err != nil {
			t.Fatal("corrected original order failed to recover idempotently", err)
		}
	}
	afterWallet, err = f.s.LocalWalletSnapshot(f.ctx, id)
	if err != nil || afterWallet.BalanceCents != beforeWallet.BalanceCents-600 || p1Count(t, &db.LocalPlayPaymentTime{}, "order_id=?", orderID) != 1 || p1Count(t, &db.LocalWalletEntry{}, "order_id=?", orderID) != 1 {
		t.Fatal("original retry duplicated debit/projection", afterWallet, err)
	}
}

func TestRechargeClock75OverlappingPeriodsRemainIndependent(t *testing.T) {
	f, id := p3DemoProviderFixture(t)
	f.id = id
	p3DemoProviderFund(t, f, id, "recharge75-overlap-fund", 1000)
	real := time.Now()
	for _, key := range []string{"rechargeTotal_p4_personal75", "rechargeTotal_p4_monthly75"} {
		cfg := db.PeriodInstance{ID: key, Title: "独立累充", Kind: "payment_amount", Mode: "cumulative", Enabled: true, Goal: 600, Rewards: "coin,17", StartAt: real.Add(-time.Hour).UnixMilli(), EndAt: real.Add(time.Hour).UnixMilli(), RewardEndAt: real.Add(2 * time.Hour).UnixMilli(), Source: "isolated overlapping consumers"}
		if err := db.GetIns().Create(&cfg).Error; err != nil {
			t.Fatal(err)
		}
	}
	f.s.P3AutoConfirmDemoPayments = true
	for i := 0; i < 2; i++ {
		if _, err := f.s.generatePayment(f.ctx, id, p3DemoProviderRequest(t, "djsy.wuyou.andp0006"), 751002, real); err != nil {
			t.Fatal(err)
		}
	}
	for _, key := range []string{"rechargeTotal_p4_personal75", "rechargeTotal_p4_monthly75"} {
		progress := p1ProgressValue(t, f, key)
		if progress.Value != 600 || p1Count(t, &db.RewardDelivery{}, "player_id=? AND source_key=?", id, "activity:"+key) != 1 {
			t.Fatal("overlapping independent period lost or duplicated one confirmation", key, progress)
		}
	}
	wallet, err := f.s.LocalWalletSnapshot(f.ctx, id)
	if err != nil || wallet.BalanceCents != 400 || p1Count(t, &db.PaymentOrder{}, "player_id=? AND status=?", id, constUtil.P3PaymentConfirmed) != 1 {
		t.Fatal("two activity consumers charged the same order twice", wallet, err)
	}
}

func TestRechargeClock75PublishedOnceThroughOriginalScheduleAPI(t *testing.T) {
	f, actor := adminModesFixture(t)
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	if err := f.s.InitializeAdminGameServers(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
		t.Fatal(err)
	}
	form := adminModeForm("recharge-total")
	form.Milestones = []model.AdminActivityMilestone{{Goal: 600, Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 17}}}}
	definition, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: "发布时间检查", Form: form, DefaultDays: 2, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	calendar, err := f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	saved, err := f.s.AdminSaveActivitySchedule(f.ctx, actor, 1, model.AdminActivityScheduleSaveRequest{Version: calendar.Version, Items: []model.AdminActivityOccurrenceDTO{{TemplateID: definition.ID, TemplateVersion: definition.Version, StartAt: now - 3600000, EndAt: now + 86400000, ClaimEndAt: now + 86400000, Enabled: false}}})
	if err != nil {
		t.Fatal(err)
	}
	var row db.AdminActivityOccurrence
	if err := db.GetIns().First(&row, "id=?", saved.Items[0].ID).Error; err != nil || row.FirstPublishedAt != 0 {
		t.Fatal("disabled schedule invented a publication", row.FirstPublishedAt, err)
	}
	var first int64
	for _, enabled := range []bool{true, false, true} {
		saved.Items[0].Enabled = enabled
		saved, err = f.s.AdminSaveActivitySchedule(f.ctx, actor, 1, model.AdminActivityScheduleSaveRequest{Version: saved.Version, Items: saved.Items})
		if err != nil {
			t.Fatal(err)
		}
		if err := db.GetIns().First(&row, "id=?", saved.Items[0].ID).Error; err != nil {
			t.Fatal(err)
		}
		if first == 0 {
			first = row.FirstPublishedAt
		}
		if first < now || row.FirstPublishedAt != first {
			t.Fatal("disable/re-enable changed immutable first publication", first, row.FirstPublishedAt)
		}
	}
}

func TestRechargeClock75OriginalOpeningUsesPublishedSourceAndRechecksUnclaimed(t *testing.T) {
	f, id := p3DemoProviderFixture(t)
	f.id = id
	if err := f.s.InitializeOpenServerActivity(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().AutoMigrate(&db.AdminActivityOccurrence{}, &db.AdminCharacterActivityInstance{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	start := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location()).AddDate(0, 0, -6)
	if err := db.GetIns().Model(&db.OpenServerActivityRule{}).Where("id=?", constUtil.OpenServerActivityID).Updates(map[string]any{"start_at": start.UnixMilli(), "end_at": start.AddDate(0, 0, 14).UnixMilli(), "reward_end_at": start.AddDate(0, 0, 15).UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	birth := now.Add(-2 * time.Hour).UnixMilli()
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", id).Update("created_at", birth).Error; err != nil {
		t.Fatal(err)
	}
	source := db.AdminActivityOccurrence{ID: "recharge75-opening", ServerID: 1, ConsumerKey: "opening_schedule_recharge75", ConsumerID: "opening:opening_schedule_recharge75", Mode: "opening-14", FirstPublishedAt: 0, FormJSON: []byte(`{}`)}
	if err := db.GetIns().Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	old := recharge75Order(t, f, "recharge75-opening-old", constUtil.P3PaymentConfirmed, now.Add(-time.Hour), 9000000)
	oldGame := now.AddDate(0, 0, -1)
	if err := db.GetIns().Create(&db.LocalPlayPaymentTime{OrderID: old.ID, PlayerID: id, RealAt: old.ConfirmedAt, GameAt: oldGame.UnixMilli(), OffsetDays: -1}).Error; err != nil {
		t.Fatal(err)
	}
	recharge75Order(t, f, "recharge75-opening-current", constUtil.P3PaymentConfirmed, now.Add(-10*time.Minute), 600)
	tx := db.GetIns().WithContext(f.ctx)
	adminBindOpeningPeriod(tx, db.AdminCharacterActivityInstance{PlayerID: id, OccurrenceID: source.ID, ConsumerKey: "opening_character_1_2"})
	if adminOpeningPeriod(tx) != nil {
		t.Fatal("payment publication context changed original once-only task ledger")
	}
	p, g := f.profile(t)
	if err := f.s.refreshOpenServerProgressAt(tx, p, g, now); err != nil {
		t.Fatal(err)
	}
	var paymentStates []db.OpenServerActivityState
	if err := tx.Table("open_server_activity_states AS state").Select("state.*").Joins("JOIN open_server_activity_tasks AS task ON task.id=state.task_id").Where("state.player_id=? AND state.achieved=? AND state.progress>? AND task.task_type IN ?", id, true, 6, []int32{2, 101}).Order("state.task_id").Find(&paymentStates).Error; err != nil {
		t.Fatal(err)
	}
	if len(paymentStates) < 2 {
		t.Fatal("original249 task fixture did not produce two independent paid prerequisites", paymentStates)
	}
	// A finite historical claim marker proves that reconciliation preserves an
	// already consumed receipt; no property is granted by this fixture marker.
	claimed := paymentStates[0]
	if err := tx.Model(&db.OpenServerActivityState{}).Where("player_id=? AND task_id=?", id, claimed.TaskID).Update("claims", 1).Error; err != nil {
		t.Fatal(err)
	}
	published := now.Add(-30 * time.Minute).UnixMilli()
	if err := tx.Model(&source).Update("first_published_at", published).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.refreshOpenServerProgressAt(tx, p, g, now); err != nil {
		t.Fatal(err)
	}
	for _, original := range paymentStates {
		var current db.OpenServerActivityState
		if err := tx.First(&current, "player_id=? AND task_id=?", id, original.TaskID).Error; err != nil {
			t.Fatal(err)
		}
		if original.TaskID == claimed.TaskID {
			if !current.Achieved || current.Claims != 1 || current.Progress != claimed.Progress {
				t.Fatal("publication correction changed historical claimed task", current)
			}
		} else if current.Achieved || current.Progress > 6 {
			t.Fatal("unclaimed original task retained pre-publication payment", current)
		}
	}
	var obsolete db.OpenServerActivityCounter
	if err := tx.First(&obsolete, "player_id=? AND counter_key=?", id, constUtil.P3OpenServerSinglePaid+p3Day(oldGame)).Error; err != nil || obsolete.Value != 0 {
		t.Fatal("obsolete original single-payment date retained old qualification", obsolete, err)
	}
	if floor, err := rechargeActivityRealStart75(tx, id, constUtil.OpenServerActivityID); err != nil || floor != published {
		t.Fatal("original opening ID did not resolve accepted publication", floor, err)
	}
	if floor, err := rechargeActivityRealStart75(tx, id+1, constUtil.OpenServerActivityID); err == nil || floor != 0 {
		t.Fatal("another player borrowed original opening publication context", floor, err)
	}
}
