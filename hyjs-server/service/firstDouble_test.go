package service

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func firstDoublePreservePolicy(t *testing.T) {
	t.Helper()
	var before db.FirstDoublePolicy
	existed := false
	if db.GetIns().Migrator().HasTable(&before) {
		err := db.GetIns().First(&before, "id=?", 1).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatal(err)
		}
		existed = err == nil
	}
	// Cleanup runs before the fixture closes its database. Preserve a shared
	// suite's prior enable/stop exactly; a policy created only here is removed.
	t.Cleanup(func() {
		if existed {
			if err := db.GetIns().Save(&before).Error; err != nil {
				t.Error(err)
			}
		} else if db.GetIns().Migrator().HasTable(&before) {
			if err := db.GetIns().Where("id=?", 1).Delete(&db.FirstDoublePolicy{}).Error; err != nil {
				t.Error(err)
			}
		}
	})
}

func firstDoubleSetPolicy(t *testing.T, enabled bool) {
	t.Helper()
	if err := db.GetIns().Model(&db.FirstDoublePolicy{}).Where("id=?", 1).Update("enabled", enabled).Error; err != nil {
		t.Fatal(err)
	}
}

func firstDoubleOrder(t *testing.T, f *p1Fixture, id int64, sku string, stamp int64) db.PaymentOrder {
	t.Helper()
	out, err := f.s.generatePayment(f.ctx, id, p3DemoProviderRequest(t, sku), stamp, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var order db.PaymentOrder
	if err = db.GetIns().First(&order, "id=?", out[0].(*pb.SCGenerateOrder).GetOrderId()).Error; err != nil {
		t.Fatal(err)
	}
	return order
}

func firstDoubleSnapshot(t *testing.T, order db.PaymentOrder) db.PaymentProduct {
	t.Helper()
	var product db.PaymentProduct
	if err := json.Unmarshal([]byte(order.Snapshot), &product); err != nil {
		t.Fatal(err)
	}
	return product
}

func TestFirstDoubleDefaultOffOldPendingHistoryAndCold(t *testing.T) {
	var (
		f, id = p3DemoProviderFixture(t)
		sku   = "djsy.wuyou.andp0006"
	)
	firstDoublePreservePolicy(t)
	if err := f.s.InitializeFirstDouble(f.ctx); err != nil {
		t.Fatal(err)
	}
	firstDoubleSetPolicy(t, false)
	old := firstDoubleOrder(t, f, id, sku, 19501)
	if old.Status != constUtil.P3PaymentPending || firstDoubleSnapshot(t, old).FirstDouble {
		t.Fatal("default-off changed the original pending snapshot")
	}
	firstDoubleSetPolicy(t, true)
	var product db.PaymentProduct
	if err := db.GetIns().First(&product, "id=?", sku).Error; err != nil {
		t.Fatal(err)
	}
	chargeType, rate, err := f.s.firstDoublePaymentInfo(f.ctx, id, product)
	if err != nil || chargeType != "first" || rate != 1 {
		t.Fatalf("original 2x badge: %s/%d %v", chargeType, rate, err)
	}
	p3DemoProviderFund(t, f, id, "first-double-old-pending", 1200)
	f.s.P3AutoConfirmDemoPayments = true
	if _, err = f.s.LocalWalletPay(f.ctx, id, old.ID); err != nil {
		t.Fatal(err)
	}
	old = firstDoubleOrder(t, f, id, sku, 19501)
	if old.Credited != 66 || firstDoubleSnapshot(t, old).FirstDouble {
		t.Fatal("enabling retroactively upgraded old pending")
	}
	next := firstDoubleOrder(t, f, id, sku, 19502)
	if next.Credited != 66 || firstDoubleSnapshot(t, next).FirstDouble {
		t.Fatal("an original historical confirmed payment was reset")
	}
	chargeType, rate, err = f.s.firstDoublePaymentInfo(f.ctx, id, product)
	if err != nil || chargeType != "general" || rate != 1 {
		t.Fatal("already-paid SKU still advertises first double")
	}
	firstDoubleSetPolicy(t, false)
	f.restart(t)
	if err = f.s.InitializeFirstDouble(f.ctx); err != nil {
		t.Fatal(err)
	}
	enabled, err := firstDoubleEnabled(db.GetIns().WithContext(f.ctx))
	if err != nil || enabled {
		t.Fatal("cold initialization reopened an operator stop")
	}
	wallet, err := f.s.LocalWalletSnapshot(f.ctx, id)
	if err != nil || wallet.BalanceCents != 0 || len(wallet.Entries) != 3 {
		t.Fatal("two real prices did not survive cold recovery", err)
	}
}

func TestFirstDoubleTwoPendingOneLifetimeBonusRealWalletAndNoReset(t *testing.T) {
	var (
		f, id = p3DemoProviderFixture(t)
		sku   = "djsy.wuyou.andp0006"
	)
	firstDoublePreservePolicy(t)
	if err := f.s.InitializeFirstDouble(f.ctx); err != nil {
		t.Fatal(err)
	}
	firstDoubleSetPolicy(t, true)
	var orders []db.PaymentOrder
	for i := int64(0); i < 2; i++ {
		order := firstDoubleOrder(t, f, id, sku, 19510+i)
		if !firstDoubleSnapshot(t, order).FirstDouble {
			t.Fatal("new eligible pending did not freeze first-double policy")
		}
		orders = append(orders, order)
	}
	// Closing publication leaves the two accepted snapshots intact. Both orders
	// compete through the original profile lock and debit the real wallet once.
	firstDoubleSetPolicy(t, false)
	p3DemoProviderFund(t, f, id, "first-double-two-pending", 1200)
	f.s.P3AutoConfirmDemoPayments = true
	var workers sync.WaitGroup
	errors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func(index int) {
			defer workers.Done()
			_, err := f.s.LocalWalletPay(f.ctx, id, orders[index%2].ID)
			errors <- err
		}(i)
	}
	workers.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var credited int64
	for _, original := range orders {
		var paid db.PaymentOrder
		if err := db.GetIns().First(&paid, "id=?", original.ID).Error; err != nil {
			t.Fatal(err)
		}
		if paid.Status != constUtil.P3PaymentConfirmed || (paid.Credited != 66 && paid.Credited != 126) {
			t.Fatal("original charge/free/extra-charge result differs")
		}
		credited += paid.Credited
	}
	if credited != 192 {
		t.Fatal("concurrent distinct pending orders duplicated or lost the lifetime bonus", credited)
	}
	f.id = id
	player, _ := f.profile(t)
	if player.GetDiamond() != 192 || player.GetChargeDiamond() != 120 || player.GetChargeCount() != 2 {
		t.Fatal("bonus changed VIP paid-charge accounting")
	}
	wallet, err := f.s.LocalWalletSnapshot(f.ctx, id)
	if err != nil || wallet.BalanceCents != 0 || wallet.Revision != 3 || len(wallet.Entries) != 3 {
		t.Fatal("actual wallet prices or idempotency changed", err)
	}
	firstDoubleSetPolicy(t, true)
	var product db.PaymentProduct
	if err = db.GetIns().First(&product, "id=?", sku).Error; err != nil {
		t.Fatal(err)
	}
	for _, date := range []time.Time{time.Now().AddDate(0, 0, 1), time.Now().AddDate(0, 0, -1)} {
		out, infoErr := f.s.firstDoubleBoonInfo(f.ctx, id, &pb.CSCopyBoonInfo{ActivityId: proto.String("RechangeDouble_local")}, date)
		if infoErr != nil || out[0].(*pb.SCCopyBoonInfo).GetInformation() == "" {
			t.Fatal("original explanation callback missing", infoErr)
		}
	}
	chargeType, _, err := f.s.firstDoublePaymentInfo(f.ctx, id, product)
	if err != nil || chargeType != "general" {
		t.Fatal("republication or period explanation reset paid SKU")
	}
	// Independent SKUs retain their own first-ever eligibility, while a local
	// box and a modified original price are never admitted by the new policy.
	var other db.PaymentProduct
	if err = db.GetIns().First(&other, "id=?", "djsy.wuyou.andp0030").Error; err != nil {
		t.Fatal(err)
	}
	chargeType, _, err = f.s.firstDoublePaymentInfo(f.ctx, id, other)
	if err != nil || chargeType != "first" {
		t.Fatal("separate original SKU lost its first-ever eligibility")
	}
	other.Cents++
	if f.s.firstDoubleOriginalSKU(&other) {
		t.Fatal("modified original economic fields entered policy")
	}
	var local db.PaymentProduct
	if err = db.GetIns().First(&local, "id=?", "local-p3-red-card").Error; err != nil {
		t.Fatal(err)
	}
	if f.s.firstDoubleOriginalSKU(&local) {
		t.Fatal("local reward box entered original-SKU policy")
	}
}
