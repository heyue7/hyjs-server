package service

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func localClockFixture(t *testing.T) (*p1Fixture, int64, *db.Profile) {
	t.Helper()
	f, id := p3DemoProviderFixture(t)
	f.id = id
	for _, initialize := range []func(context.Context) error{f.s.InitializeLocalPlay, f.s.InitializeLocalPlayActivities, f.s.InitializeOpenServerActivity, f.s.InitializeP4, f.s.InitializeMonthSign, f.s.InitializeDailyTasks} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.LocalPlayToolsEnabled = true
	if err := db.GetIns().Model(&db.LocalPlayPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	var owner db.Profile
	if err := db.GetIns().First(&owner, id).Error; err != nil {
		t.Fatal(err)
	}
	return f, id, &owner
}

func localClockBind(t *testing.T, f *p1Fixture, owner *db.Profile) context.Context {
	t.Helper()
	ctx, err := f.s.BindLocalPlayContext(f.ctx, owner)
	if err != nil || !LocalPlayContextBound(ctx) {
		t.Fatal("original authenticated owner clock binding", err)
	}
	return ctx
}

func localClockEnable(t *testing.T) {
	t.Helper()
	if err := db.GetIns().Model(&db.LocalPlayPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
}

func TestLocalPlayClockCalendarIsolationIdempotenceAndCold(t *testing.T) {
	f, id, owner := localClockFixture(t)
	closed, err := f.s.BindLocalPlayContext(f.ctx, owner)
	if err != nil || LocalPlayContextBound(closed) {
		t.Fatal("default-off changed the business clock", err)
	}
	if _, err = f.s.LocalPlayTimeChange(closed, id, "nextDay", "closed"); err == nil {
		t.Fatal("closed clock changed time")
	}
	localClockEnable(t)
	ctx := localClockBind(t, f, owner)
	before, game := f.profile(t)
	walletBefore, err := f.s.LocalWalletSnapshot(f.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	errs := make(chan error, 8)
	views := make(chan *model.LocalPlayTimeView, 8)
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			v, e := f.s.LocalPlayTimeChange(ctx, id, "nextDay", "same-day-next")
			errs <- e
			if e == nil {
				views <- v
			}
		}()
	}
	workers.Wait()
	close(errs)
	close(views)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var first *model.LocalPlayTimeView
	for view := range views {
		if first == nil {
			first = view
		} else if !reflect.DeepEqual(first, view) {
			t.Fatal("same request replay changed its exact result")
		}
	}
	if first == nil || first.OffsetDays != 1 || first.Revision != 1 || first.GameTimeMS-first.RealTimeMS != int64(24*time.Hour/time.Millisecond) {
		t.Fatal("eight retries advanced more than one actual calendar day", first)
	}
	if _, err = f.s.LocalPlayTimeChange(ctx, id, "previousDay", "same-day-next"); err == nil {
		t.Fatal("same key changed its action")
	}
	previous, err := f.s.LocalPlayTimeChange(ctx, id, "previousDay", "prev-1")
	if err != nil || previous.OffsetDays != 0 || previous.Revision != 2 {
		t.Fatal("actual previous day", previous, err)
	}
	previous, err = f.s.LocalPlayTimeChange(ctx, id, "previousDay", "prev-2")
	if err != nil || previous.OffsetDays != -1 || previous.GameTimeMS-previous.RealTimeMS != -int64(24*time.Hour/time.Millisecond) {
		t.Fatal("negative calendar offset", previous, err)
	}
	reset, err := f.s.LocalPlayTimeChange(ctx, id, "reset", "reset-real")
	if err != nil || reset.OffsetDays != 0 || reset.GameTimeMS != reset.RealTimeMS || reset.Revision != 4 {
		t.Fatal("reset real calendar", reset, err)
	}
	after, afterGame := f.profile(t)
	walletAfter, err := f.s.LocalWalletSnapshot(f.ctx, id)
	if err != nil || !proto.Equal(before, after) || !reflect.DeepEqual(game, afterGame) || !reflect.DeepEqual(walletBefore, walletAfter) {
		t.Fatal("calendar operations modified actual player/inventory/wallet", err)
	}
	f.restart(t)
	ctx = localClockBind(t, f, owner)
	cold, err := f.s.LocalPlayTimeChange(ctx, id, "nextDay", "same-day-next")
	if err != nil || !reflect.DeepEqual(first, cold) {
		t.Fatal("cold exact old time receipt", cold, err)
	}
	current, err := f.s.LocalPlayTimeState(ctx, id)
	if err != nil || current.OffsetDays != 0 || current.Revision != 4 {
		t.Fatal("old receipt rewound current clock state", current, err)
	}
}

func TestLocalPlayClockOwnerSessionRotationAndProtectedOne(t *testing.T) {
	f, id, owner := localClockFixture(t)
	localClockEnable(t)
	ctx := localClockBind(t, f, owner)
	other, err := LoginAccount(f.ctx, fmt.Sprintf("demo-clock-other-%d", time.Now().UnixNano()), "isolated-local-demo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, other.Id, "另一个时钟角色", "hero_043"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.LocalPlayTimeChange(ctx, other.Id, "nextDay", "wrong-owner"); err == nil {
		t.Fatal("bound clock changed another account")
	}
	protected := *owner
	protected.Id = 1
	bound, err := f.s.BindLocalPlayContext(f.ctx, &protected)
	if err != nil || LocalPlayContextBound(bound) {
		t.Fatal("protected account1 gained the demo clock", err)
	}
	rotated, err := LoginAccount(f.ctx, owner.Uid, "isolated-local-demo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.LocalPlayTimeChange(ctx, id, "nextDay", "old-session"); err == nil {
		t.Fatal("rotated session changed a calendar")
	}
	f.ctx = context.WithValue(f.ctx, constUtil.LocalWalletSessionContextKey, append([]byte(nil), rotated.Session...))
	newctx := localClockBind(t, f, rotated)
	if _, err = f.s.LocalPlayTimeChange(newctx, id, "nextDay", "new-session"); err != nil {
		t.Fatal("new original session could not change its calendar", err)
	}
	var count int64
	if err = db.GetIns().Model(&db.LocalPlayClockState{}).Where("player_id=?", other.Id).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("denied cross-account request created other clock state", count, err)
	}
}

func TestLocalPlayClockOriginalPaymentMembershipAndDailyClaimCalendar(t *testing.T) {
	f, id, owner := localClockFixture(t)
	localClockEnable(t)
	ctx := localClockBind(t, f, owner)
	if _, err := f.s.monthSign(ctx, id, &pb.CSReceiveMonthSign{}, 11001); err != nil {
		t.Fatal("original actual-day sign", err)
	}
	if _, err := f.s.dailyTasks(ctx, id, &pb.CSTaskList{}); err != nil {
		t.Fatal("original actual-day tasks", err)
	}
	if _, err := f.s.LocalPlayTimeChange(ctx, id, "nextDay", "paid-next-day"); err != nil {
		t.Fatal(err)
	}
	ctx = localClockBind(t, f, owner)
	if _, err := f.s.monthSign(ctx, id, &pb.CSReceiveMonthSign{}, 11002); err != nil {
		t.Fatal("original new-day sign", err)
	}
	if _, err := f.s.dailyTasks(ctx, id, &pb.CSTaskList{}); err != nil {
		t.Fatal("original new-day tasks", err)
	}
	var days int64
	if err := db.GetIns().Model(&db.DailyTaskState{}).Where("player_id=?", id).Count(&days).Error; err != nil || days != 2 {
		t.Fatal("same owner did not retain two real daily-task periods", days, err)
	}
	var product db.PaymentProduct
	if err := db.GetIns().Where("kind=? AND channels LIKE ?", "month", "%gf_default%").Order("id").First(&product).Error; err != nil {
		t.Fatal(err)
	}
	p3DemoProviderFund(t, f, id, "clock-original-month", product.Cents)
	f.s.P3AutoConfirmDemoPayments = true
	real := time.Now()
	out, err := f.s.generatePayment(ctx, id, p3DemoProviderRequest(t, product.ID), 12001, real)
	if err != nil {
		t.Fatal("original funded order", err)
	}
	orderID := out[0].(*pb.SCGenerateOrder).GetOrderId()
	var order db.PaymentOrder
	if err = db.GetIns().First(&order, "id=?", orderID).Error; err != nil || order.Status != constUtil.P3PaymentConfirmed || order.CreatedAt != real.UnixMilli() || order.ConfirmedAt != real.UnixMilli() || order.ExpiresAt != real.UnixMilli()+constUtil.P3PaymentOrderLifetimeMS {
		t.Fatal("play-day changed original accounting time", order, err)
	}
	var mapped db.LocalPlayPaymentTime
	if err = db.GetIns().First(&mapped, "order_id=?", orderID).Error; err != nil || mapped.RealAt != order.ConfirmedAt || mapped.GameAt != LocalPlayAt(ctx, real).UnixMilli() || mapped.OffsetDays != 1 {
		t.Fatal("actual paid game-time projection", mapped, err)
	}
	p, _ := f.profile(t)
	if p.GetMonthCardEndTime() != LocalPlayAt(ctx, real).UnixMilli()+constUtil.P3MonthCardDurationMS {
		t.Fatal("membership did not start on actual purchase play day", p.GetMonthCardEndTime())
	}
	var debit db.LocalWalletEntry
	if err = db.GetIns().First(&debit, "order_id=?", orderID).Error; err != nil || debit.At != real.UnixMilli() || debit.DeltaCents != -product.Cents {
		t.Fatal("clock changed wallet money/real ledger time", debit, err)
	}
	if _, err = f.s.membership(ctx, id, &pb.CSClaimCard{CardType: proto.String("month")}, LocalPlayAt(ctx, real)); err != nil {
		t.Fatal("actual paid play-day member reward", err)
	}
	if _, err = f.s.LocalPlayTimeChange(ctx, id, "reset", "back-to-real"); err != nil {
		t.Fatal(err)
	}
	ctx = localClockBind(t, f, owner)
	before, _ := f.profile(t)
	if _, err = f.s.monthSign(ctx, id, &pb.CSReceiveMonthSign{}, 11003); err == nil {
		t.Fatal("rewound calendar granted an already claimed sign day")
	}
	if _, err = f.s.membership(ctx, id, &pb.CSClaimCard{CardType: proto.String("month")}, LocalPlayAt(ctx, real)); err == nil {
		t.Fatal("membership was usable before its actual paid play day")
	}
	after, _ := f.profile(t)
	if !proto.Equal(before, after) {
		t.Fatal("rejected rewind claims changed original wallet/inventory")
	}
	var coldOrder db.PaymentOrder
	if err = db.GetIns().First(&coldOrder, "id=?", orderID).Error; err != nil || !reflect.DeepEqual(order, coldOrder) {
		t.Fatal("clock reset rewrote the real payment order", err)
	}
	f.restart(t)
	ctx = localClockBind(t, f, owner)
	if _, err = f.s.monthSign(ctx, id, &pb.CSReceiveMonthSign{}, 11004); err == nil {
		t.Fatal("cold rewind replayed a claimed sign day")
	}
}
