package service

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func localWalletFixture(t *testing.T) (*p1Fixture, int64, context.Context) {
	t.Helper()
	f, id := p3DemoProviderFixture(t)
	f.s.P3LocalWalletEnabled = true
	var prior db.LocalWalletPolicy
	existed := db.GetIns().Migrator().HasTable(&prior) && db.GetIns().First(&prior, 1).Error == nil
	t.Cleanup(func() {
		if existed {
			if err := db.GetIns().Save(&prior).Error; err != nil {
				t.Error(err)
			}
		} else if db.GetIns().Migrator().HasTable(&prior) {
			if err := db.GetIns().Where("id=1").Delete(&prior).Error; err != nil {
				t.Error(err)
			}
		}
	})
	if err := f.s.InitializeLocalWallet(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.LocalWalletPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	var owner db.Profile
	if err := db.GetIns().First(&owner, id).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(f.ctx, constUtil.LocalWalletAPIContextKey, true)
	ctx = context.WithValue(ctx, constUtil.LocalWalletSessionContextKey, append([]byte(nil), owner.Session...))
	return f, id, ctx
}

func localWalletEnable(t *testing.T) {
	t.Helper()
	if err := db.GetIns().Model(&db.LocalWalletPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
}

func TestLocalWalletDefaultOffAuthIntegerAndConcurrentCredit(t *testing.T) {
	f, id, ctx := localWalletFixture(t)
	view, err := f.s.LocalWalletSnapshot(ctx, id)
	if err != nil || view.Enabled || view.BalanceCents != 0 || view.Currency != "CNY" || !view.TestOnly {
		t.Fatal("default-off empty persisted account", view, err)
	}
	if _, err = f.s.LocalWalletAdd(ctx, id, "credit-1", 12345); !errors.Is(err, ErrLocalWalletDisabled) {
		t.Fatal("closed wallet accepted credit", err)
	}
	localWalletEnable(t)
	for _, n := range []int64{0, -1} {
		if _, err = f.s.LocalWalletAdd(ctx, id, "bad", n); !errors.Is(err, ErrLocalWalletInvalid) {
			t.Fatal("nonpositive cents", n, err)
		}
	}
	untrusted := context.WithValue(ctx, constUtil.LocalWalletAPIContextKey, false)
	if _, err = f.s.LocalWalletAdd(untrusted, id, "unauth", 1); !errors.Is(err, ErrLocalWalletInvalid) {
		t.Fatal("untrusted context", err)
	}
	bad := context.WithValue(ctx, constUtil.LocalWalletSessionContextKey, make([]byte, 16))
	if _, err = f.s.LocalWalletAdd(bad, id, "stale", 1); !errors.Is(err, ErrLocalWalletInvalid) {
		t.Fatal("stale authenticated session", err)
	}
	var workers sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() { defer workers.Done(); _, err := f.s.LocalWalletAdd(ctx, id, "credit-1", 12345); errs <- err }()
	}
	workers.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	view, err = f.s.LocalWalletSnapshot(ctx, id)
	if err != nil || view.BalanceCents != 12345 || view.Revision != 1 || len(view.Entries) != 1 || view.Entries[0].Kind != "credit" || view.Entries[0].AmountCents != 12345 {
		t.Fatal("eight retries did not credit exactly once", view, err)
	}
	if _, err = f.s.LocalWalletAdd(ctx, id, "credit-1", 12346); !errors.Is(err, ErrLocalWalletConflict) {
		t.Fatal("same key changed amount", err)
	}
	if _, err = f.s.LocalWalletAdd(ctx, id, "overflow", math.MaxInt64); !errors.Is(err, ErrLocalWalletInvalid) {
		t.Fatal("integer overflow", err)
	}
	f.restart(t)
	if err = f.s.InitializeLocalWallet(f.ctx); err != nil {
		t.Fatal(err)
	}
	again, err := f.s.LocalWalletAdd(ctx, id, "credit-1", 12345)
	if err != nil || !again.AlreadyApplied || again.BalanceCents != view.BalanceCents || again.Revision != view.Revision || len(again.Entries) != 1 {
		t.Fatal("cold idempotent balance", again, err)
	}
}

// Requires the root-owned confirmTestPaymentAt marker hook. It exercises the same original grant transaction.
func TestLocalWalletActualPendingRechargeInsufficientCreditConcurrentAndCold(t *testing.T) {
	f, id, ctx := localWalletFixture(t)
	localWalletEnable(t)
	f.id = id
	before, _ := f.profile(t)
	// Use the original APK recharge product; retired local reward bundles stay disabled.
	request := p3DemoProviderRequest(t, "djsy.wuyou.andp0006")
	now := time.Now()
	out, err := f.s.generatePayment(ctx, id, request, 8001, now)
	if err != nil {
		t.Fatal(err)
	}
	orderID := out[0].(*pb.SCGenerateOrder).GetOrderId()
	var order db.PaymentOrder
	if err = db.GetIns().First(&order, "id=?", orderID).Error; err != nil || order.Status != constUtil.P3PaymentPending {
		t.Fatal("original pending order", err)
	}
	view, err := f.s.LocalWalletSnapshot(ctx, id)
	if err != nil || len(view.PendingOrders) != 1 || view.PendingOrders[0].OrderID != orderID || view.PendingOrders[0].AmountCents != order.Cents {
		t.Fatal("frozen original pending projection", view, err)
	}
	trusted := context.WithValue(ctx, constUtil.P3TestPaymentContextKey, true)
	trusted = context.WithValue(trusted, constUtil.LocalWalletPaymentContextKey, true)
	if _, err = f.s.confirmTestPaymentAt(trusted, orderID, "wallet:"+orderID, order.Cents, constUtil.P3PaymentConfirmed, now); !errors.Is(err, ErrLocalWalletInsufficient) {
		t.Fatal("insufficient wallet credited original product", err)
	}
	afterInsufficient, _ := f.profile(t)
	if !proto.Equal(before, afterInsufficient) {
		t.Fatal("pending insufficient charge changed game wallet")
	}
	if err = db.GetIns().First(&order, "id=?", orderID).Error; err != nil || order.Status != constUtil.P3PaymentPending {
		t.Fatal("insufficient order lost pending", err)
	}
	if _, err = f.s.LocalWalletAdd(ctx, id, "fund-order", order.Cents+321); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, err := f.s.confirmTestPaymentAt(trusted, orderID, "wallet:"+orderID, order.Cents, constUtil.P3PaymentConfirmed, now)
			errs <- err
		}()
	}
	workers.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = db.GetIns().First(&order, "id=?", orderID).Error; err != nil || order.Status != constUtil.P3PaymentConfirmed {
		t.Fatal("funded pending order did not confirm", err)
	}
	paid, game := f.profile(t)
	view, err = f.s.LocalWalletSnapshot(ctx, id)
	if err != nil || view.BalanceCents != 321 || view.Revision != 2 || len(view.Entries) != 2 || view.Entries[0].Kind != "payment" || view.Entries[0].AmountCents != -order.Cents || len(view.PendingOrders) != 0 || paid.GetChargeCount() != before.GetChargeCount()+1 || paid.GetDiamond()-before.GetDiamond() != order.Credited {
		t.Fatal("payment/grant did not share one once-only transaction", view, err)
	}
	f.restart(t)
	if _, err = f.s.confirmTestPaymentAt(trusted, orderID, "wallet:"+orderID, order.Cents, constUtil.P3PaymentConfirmed, now); err != nil {
		t.Fatal(err)
	}
	cold, coldGame := f.profile(t)
	view, err = f.s.LocalWalletSnapshot(ctx, id)
	if err != nil || view.BalanceCents != 321 || view.Revision != 2 || !proto.Equal(paid, cold) || len(coldGame.Warriors) != len(game.Warriors) {
		t.Fatal("cold recharge repeated charge or original grant", view, err)
	}
}

// A later original membership eligibility failure must roll back the earlier wallet debit and grant.
func TestLocalWalletOriginalMembershipFailureRollsBackDebitAndGrant(t *testing.T) {
	f, id, ctx := localWalletFixture(t)
	localWalletEnable(t)
	f.id = id
	var product db.PaymentProduct
	if err := db.GetIns().Where("kind=? AND channels LIKE ?", "month", "%gf_default%").Order("id").First(&product).Error; err != nil {
		t.Fatal(err)
	}
	request := p3DemoProviderRequest(t, product.ID)
	now := time.Now()
	orders := []string{}
	for i := int64(0); i < 2; i++ {
		out, err := f.s.generatePayment(ctx, id, request, 9001+i, now)
		if err != nil {
			t.Fatal(err)
		}
		orders = append(orders, out[0].(*pb.SCGenerateOrder).GetOrderId())
	}
	if _, err := f.s.LocalWalletAdd(ctx, id, "two-original-month-orders", product.Cents*2); err != nil {
		t.Fatal(err)
	}
	trusted := context.WithValue(ctx, constUtil.P3TestPaymentContextKey, true)
	trusted = context.WithValue(trusted, constUtil.LocalWalletPaymentContextKey, true)
	if _, err := f.s.confirmTestPaymentAt(trusted, orders[0], "wallet:"+orders[0], product.Cents, constUtil.P3PaymentConfirmed, now); err != nil {
		t.Fatal(err)
	}
	paid, game := f.profile(t)
	if _, err := f.s.confirmTestPaymentAt(trusted, orders[1], "wallet:"+orders[1], product.Cents, constUtil.P3PaymentConfirmed, now); err == nil {
		t.Fatal("second pending month order bypassed original renewal window")
	}
	after, afterGame := f.profile(t)
	view, err := f.s.LocalWalletSnapshot(ctx, id)
	if err != nil || view.BalanceCents != product.Cents || view.Revision != 2 || len(view.Entries) != 2 || len(view.PendingOrders) != 1 || view.PendingOrders[0].OrderID != orders[1] || !proto.Equal(paid, after) || len(afterGame.Warriors) != len(game.Warriors) {
		t.Fatal("original failure left a wallet debit or product grant", view, err)
	}
	var count int64
	if err = db.GetIns().Model(&db.PaymentConfirmation{}).Where("order_id=?", orders[1]).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("failed transaction left a confirmation fact", count, err)
	}
}
