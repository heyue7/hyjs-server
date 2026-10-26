package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func p3DemoProviderFixture(t *testing.T) (*p1Fixture, int64) {
	t.Helper()
	f := p1Setup(t)
	if err := f.s.LoadPaymentTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeGameplayPools(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP2(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializePayments(f.ctx); err != nil {
		t.Fatal(err)
	}
	account, err := LoginAccount(f.ctx, fmt.Sprintf("demo-auto-%s-%d", t.Name(), time.Now().UnixNano()), "isolated-local-demo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, account.Id, "试玩订单", "hero_043"); err != nil {
		t.Fatal(err)
	}
	if err = f.s.InitializeLocalWallet(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.LocalWalletPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	f.s.P3LocalWalletEnabled = true
	var owner db.Profile
	if err = db.GetIns().First(&owner, account.Id).Error; err != nil {
		t.Fatal(err)
	}
	f.ctx = context.WithValue(f.ctx, constUtil.LocalWalletAPIContextKey, true)
	f.ctx = context.WithValue(f.ctx, constUtil.LocalWalletSessionContextKey, append([]byte(nil), owner.Session...))
	return f, account.Id
}

func p3DemoProviderFund(t *testing.T, f *p1Fixture, id int64, key string, cents int64) {
	t.Helper()
	if _, err := f.s.LocalWalletAdd(f.ctx, id, key, cents); err != nil {
		t.Fatal(err)
	}
}

func p3DemoProviderRequest(t *testing.T, productID string) *pb.CSGenerateOrder {
	t.Helper()
	var product db.PaymentProduct
	if err := db.GetIns().First(&product, "id=?", productID).Error; err != nil {
		t.Fatal(err)
	}
	return &pb.CSGenerateOrder{PaymentId: proto.String(product.ID), ShopId: proto.String(product.Shop), ChannelId: proto.String("AND0003"), MoneyType: proto.String(product.Currency), Money: proto.Float64(float64(product.Cents) / 100)}
}

func TestP3DemoProviderDefaultClosedAndProtectedAccount(t *testing.T) {
	f, demoID := p3DemoProviderFixture(t)
	request := p3DemoProviderRequest(t, "djsy.wuyou.andp0006")
	for _, test := range []struct {
		id    int64
		auto  bool
		stamp int64
	}{{demoID, false, 101}, {f.id, true, 102}} {
		f.s.P3AutoConfirmDemoPayments = test.auto
		var before db.Profile
		if err := db.GetIns().First(&before, test.id).Error; err != nil {
			t.Fatal(err)
		}
		out, err := f.s.generatePayment(f.ctx, test.id, request, test.stamp, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		var order db.PaymentOrder
		if err = db.GetIns().First(&order, "id=?", out[0].(*pb.SCGenerateOrder).GetOrderId()).Error; err != nil {
			t.Fatal(err)
		}
		var after db.Profile
		if err = db.GetIns().First(&after, test.id).Error; err != nil {
			t.Fatal(err)
		}
		if order.Status != constUtil.P3PaymentPending || order.Credited != 0 || *before.Player != *after.Player || *before.Game != *after.Game {
			t.Fatal("closed provider or protected account mutated a save")
		}
	}
}

func TestP3DemoProviderActualConcurrentReceiptColdAndRedBoxUse(t *testing.T) {
	f, id := p3DemoProviderFixture(t)
	f.s.P3AutoConfirmDemoPayments = true
	request := p3DemoProviderRequest(t, "djsy.wuyou.andp0030")
	var product db.PaymentProduct
	if err := db.GetIns().First(&product, "id=?", request.GetPaymentId()).Error; err != nil {
		t.Fatal(err)
	}
	// Red boxes are earned from a finite recharge activity, while the original
	// directly sold red-box products remain disabled under current policy.
	activity := db.PeriodInstance{ID: "rechargeTotal_p4_native_box75", Enabled: true, Title: "隔离充值奖励", Kind: "payment_amount", Mode: "cumulative", Goal: product.Cents, Rewards: "item_023,1", StartAt: time.Now().Add(-time.Hour).UnixMilli(), EndAt: time.Now().Add(time.Hour).UnixMilli(), RewardEndAt: time.Now().Add(2 * time.Hour).UnixMilli(), Source: "finite isolated actual-payment activity fixture"}
	if err := db.GetIns().Create(&activity).Error; err != nil {
		t.Fatal(err)
	}
	p3DemoProviderFund(t, f, id, "red-card-original-price", product.Cents)
	f.id = id
	before, _ := f.profile(t)
	now := time.Now()
	var workers sync.WaitGroup
	errors := make(chan error, 8)
	orders := make(chan string, 8)
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			out, err := f.s.generatePayment(f.ctx, id, request, 201, now)
			if err != nil {
				errors <- err
				return
			}
			orders <- out[0].(*pb.SCGenerateOrder).GetOrderId()
		}()
	}
	workers.Wait()
	close(errors)
	close(orders)
	for err := range errors {
		t.Fatal(err)
	}
	orderID := ""
	for id := range orders {
		if orderID != "" && id != orderID {
			t.Fatal("one native retry created multiple orders")
		}
		orderID = id
	}
	var order db.PaymentOrder
	if err := db.GetIns().First(&order, "id=?", orderID).Error; err != nil {
		t.Fatal(err)
	}
	var confirmations int64
	if err := db.GetIns().Model(&db.PaymentConfirmation{}).Where("order_id=?", orderID).Count(&confirmations).Error; err != nil {
		t.Fatal(err)
	}
	after, _ := f.profile(t)
	if order.Status != constUtil.P3PaymentConfirmed || confirmations != 1 || order.Credited < product.Charge || after.GetDiamond()-before.GetDiamond() != order.Credited || after.GetChargeCount()-before.GetChargeCount() != 1 {
		t.Fatal("automatic demo confirmation did not use the once-only order ledger")
	}
	wallet, err := f.s.LocalWalletSnapshot(f.ctx, id)
	if err != nil || wallet.BalanceCents != 0 || wallet.Revision != 2 || len(wallet.Entries) != 2 || wallet.Entries[0].AmountCents != -product.Cents {
		t.Fatal("automatic provider bypassed the funded wallet", wallet, err)
	}
	if _, err := f.s.P1Activity(f.ctx, id, &pb.CSActivityReward{ActivityId: proto.String(activity.ID), Index: proto.Int32(1), GetResources: []*pb.BaseResource{{Id: proto.String("item_023"), Num: proto.Int32(1)}}}); err != nil {
		t.Fatal("actual recharge activity box", err)
	}
	// The earned native box uses the original P2 path. Payment creates no warrior.
	for index, item := range []struct {
		id    string
		index int32
	}{{"item_023", 5}, {"item_053", 1}} {
		out, err := f.s.itemUse(f.ctx, id, &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String(item.id), Num: proto.Int32(1)}, GetItemIndex: proto.Int32(item.index)}, int64(202+index))
		if err != nil {
			t.Fatal(err)
		}
		if index == 1 {
			gain := out[0].(*pb.SCItemUse).GetGet()
			if len(gain.GetWars()) != 1 || gain.GetWars()[0].GetId() != "hero_001" || gain.GetWars()[0].GetGuid() == "" {
				t.Fatal("paid box did not create an owned original red warrior")
			}
		}
	}
	paid, game := f.profile(t)
	f.restart(t)
	if _, err := f.s.generatePayment(f.ctx, id, request, 201, now); err != nil {
		t.Fatal("cold accepted native order did not recover", err)
	}
	cold, coldGame := f.profile(t)
	if !proto.Equal(paid, cold) || len(coldGame.Warriors) != len(game.Warriors) {
		t.Fatal("cold order replay changed wallet or actual owned warrior")
	}
}

func TestP3DemoProviderMembershipIsGrantedAndConsumedFromActualOrder(t *testing.T) {
	f, id := p3DemoProviderFixture(t)
	f.s.P3AutoConfirmDemoPayments = true
	f.id = id
	for index, kind := range []string{"month", "permanent"} {
		var product db.PaymentProduct
		if err := db.GetIns().Where("kind=? AND channels LIKE ?", kind, "%gf_default%").Order("id").First(&product).Error; err != nil {
			t.Fatal(err)
		}
		request := p3DemoProviderRequest(t, product.ID)
		p3DemoProviderFund(t, f, id, "member-"+kind, product.Cents)
		if _, err := f.s.generatePayment(f.ctx, id, request, int64(301+index), time.Now()); err != nil {
			t.Fatal(err)
		}
		before, _ := f.profile(t)
		if kind == "month" && before.GetMonthCardEndTime() <= time.Now().UnixMilli() || kind == "permanent" && before.GetPermanentCardValid() != 1 {
			t.Fatal("actual card order did not persist original membership")
		}
		claim := &pb.CSClaimCard{CardType: proto.String(kind)}
		if _, err := f.s.membership(f.ctx, id, claim, time.Now()); err != nil {
			t.Fatal(err)
		}
		after, _ := f.profile(t)
		if after.GetDiamond()-before.GetDiamond() != product.Free {
			t.Fatal("membership daily reward differs from its paid snapshot")
		}
		if _, err := f.s.membership(f.ctx, id, claim, time.Now()); err == nil {
			t.Fatal("duplicate member daily reward")
		}
		if _, err := f.s.generatePayment(f.ctx, id, request, int64(401+index), time.Now()); err == nil {
			t.Fatal("automatic provider bypassed the existing card renewal guard")
		}
	}
	before, _ := f.profile(t)
	f.restart(t)
	for _, kind := range []string{"month", "permanent"} {
		if _, err := f.s.membership(f.ctx, id, &pb.CSClaimCard{CardType: proto.String(kind)}, time.Now()); err == nil {
			t.Fatal("cold membership claim duplicated a reward")
		}
	}
	after, _ := f.profile(t)
	if !proto.Equal(before, after) {
		t.Fatal("cold duplicate claims changed actual membership save")
	}
}

func TestP3DemoProviderInsufficientPendingFundAndSameOriginalRequestPaysOnce(t *testing.T) {
	f, id := p3DemoProviderFixture(t)
	f.s.P3AutoConfirmDemoPayments = true
	f.id = id
	request := p3DemoProviderRequest(t, "djsy.wuyou.andp0006")
	before, _ := f.profile(t)
	now := time.Now()
	first, err := f.s.generatePayment(f.ctx, id, request, 501, now)
	if err != nil {
		t.Fatal(err)
	}
	orderID := first[0].(*pb.SCGenerateOrder).GetOrderId()
	var order db.PaymentOrder
	if err = db.GetIns().First(&order, "id=?", orderID).Error; err != nil || order.Status != constUtil.P3PaymentPending {
		t.Fatal("unfunded automatic order lost pending", err)
	}
	unfunded, _ := f.profile(t)
	if !proto.Equal(before, unfunded) {
		t.Fatal("unfunded automatic order granted a product")
	}
	p3DemoProviderFund(t, f, id, "fund-pending-original", order.Cents+99)
	paid, err := f.s.generatePayment(f.ctx, id, request, 501, now)
	if err != nil || paid[0].(*pb.SCGenerateOrder).GetOrderId() != orderID {
		t.Fatal("same original request did not resume the original pending", err)
	}
	if err = db.GetIns().First(&order, "id=?", orderID).Error; err != nil || order.Status != constUtil.P3PaymentConfirmed {
		t.Fatal("funded pending did not finish", err)
	}
	after, _ := f.profile(t)
	wallet, err := f.s.LocalWalletSnapshot(f.ctx, id)
	if err != nil || wallet.BalanceCents != 99 || wallet.Revision != 2 || len(wallet.PendingOrders) != 0 || after.GetDiamond()-before.GetDiamond() != order.Credited {
		t.Fatal("funded automatic order had wrong wallet/product state", wallet, err)
	}
	f.restart(t)
	if _, err = f.s.generatePayment(f.ctx, id, request, 501, now); err != nil {
		t.Fatal(err)
	}
	cold, _ := f.profile(t)
	wallet, err = f.s.LocalWalletSnapshot(f.ctx, id)
	if err != nil || wallet.BalanceCents != 99 || wallet.Revision != 2 || !proto.Equal(after, cold) {
		t.Fatal("cold native retry repeated debit or grant", wallet, err)
	}
}
