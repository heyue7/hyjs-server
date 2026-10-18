package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Only Player10 is a finite eligibility prerequisite. Every diamond and training
// material below is obtained through the real funded order and original shop.
// Reuse the P1 fixture's managed-MySQL mapping when root supplies that window.
func TestLocalPlayTrainingRewindActualCostsRebateAndCold(t *testing.T) {
	f, id, owner := localClockFixture(t)
	for _, load := range []func(string) error{f.s.LoadP5Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeWarriorGrowth, f.s.InitializeShop} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	if err := updateProfile(f.ctx, id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(10); return nil }); err != nil {
		t.Fatal(err)
	}
	localClockEnable(t)
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	ctx := localClockBind(t, f, owner)
	change := func(action, key string) {
		t.Helper()
		if _, err := f.s.LocalPlayTimeChange(ctx, id, action, key); err != nil {
			t.Fatal(err)
		}
		ctx = localClockBind(t, f, owner)
	}
	requestContext := func(request proto.Message) context.Context {
		return context.WithValue(ctx, constUtil.GameRequestContextKey, request)
	}
	count := func(item string) int32 {
		t.Helper()
		_, g := f.profile(t)
		var total int32
		for _, row := range g.Items {
			if row.GetId() == item {
				total += row.GetNum()
			}
		}
		return total
	}
	p3DemoProviderFund(t, f, id, "training-calendar-funded30", 3000)
	f.s.P3AutoConfirmDemoPayments = true
	request := p3DemoProviderRequest(t, "djsy.wuyou.andp0030")
	real := time.Now()
	paid, err := f.s.generatePayment(requestContext(request), id, request, 70100, real)
	if err != nil {
		t.Fatal("actual wallet-funded original30 order", err)
	}
	var order db.PaymentOrder
	if err = db.GetIns().First(&order, "id=?", paid[0].(*pb.SCGenerateOrder).GetOrderId()).Error; err != nil || order.Status != constUtil.P3PaymentConfirmed || order.ConfirmedAt != real.UnixMilli() {
		t.Fatal("real payment ledger", err)
	}
	paidPlayer, _ := f.profile(t)
	if paidPlayer.GetDiamond() < 150 || count("item_007") != 0 {
		t.Fatal("order did not produce actual diamond budget or fixture injected material")
	}
	guid := paidPlayer.GetHeroWarId()
	change("previousDay", "training-rewind-one-day")
	if _, err = f.s.LocalPlayActivitiesApply(ctx, id, "open", "CostReturn", "00000000-0000-4000-8000-000000007101"); err != nil {
		t.Fatal(err)
	}
	key := "consumeRet_p4_" + localPlayCycleID(t, f, id, "CostReturn")
	shop := &pb.CSShopBuy{ShopType: proto.Int32(1), ShopItemId: proto.String("itemShopId_9"), BuyCount: proto.Int32(4)}
	if _, err = f.s.shopBuy(requestContext(shop), id, shop, 70101); err != nil {
		t.Fatal("actual original training material shop", err)
	}
	if count("item_007") != 20 {
		t.Fatal("four paid original portions did not produce20 materials")
	}
	train := func(stamp int64, numType, expectedMaterials, expectedDiamond int32) {
		t.Helper()
		before, _ := f.profile(t)
		materials := count("item_007")
		b := &pb.CSWarriorTraining{Guid: proto.String(guid), CostType: proto.Int32(3), NumType: proto.Int32(numType)}
		out, e := f.s.warriorTraining(requestContext(b), id, b, stamp)
		if e != nil {
			t.Fatal("actual training in bound personal day", e)
		}
		after, _ := f.profile(t)
		if before.GetDiamond()-after.GetDiamond() != int64(expectedDiamond) || materials-count("item_007") != expectedMaterials {
			t.Fatal("original training cost changed")
		}
		retry, e := f.s.warriorTraining(requestContext(b), id, b, stamp)
		same, _ := f.profile(t)
		if e != nil || !proto.Equal(out[0], retry[0]) || same.GetDiamond() != after.GetDiamond() || count("item_007") != materials-expectedMaterials {
			t.Fatal("same-stamp training double spent", e)
		}
		var fact db.EventFact
		if e = db.GetIns().First(&fact, "player_id=? AND source_key=? AND kind=?", id, fmt.Sprintf("p5:training:%s:%d", guid, stamp), "warrior_training").Error; e != nil || fact.Day != p1Day(LocalPlayNow(ctx)) || fact.Amount != 1 || fact.OccurredAt > LocalPlayNow(ctx).UnixMilli() {
			t.Fatal("actual training fact used wall day", fact, e)
		}
		confirm := &pb.CSWarriorTrainingReplace{Guid: proto.String(guid), Result: proto.Int32(1)}
		if _, e = f.s.warriorTrainingReplace(requestContext(confirm), id, confirm, stamp+1); e != nil {
			t.Fatal("original training confirmation", e)
		}
	}
	claim := func(stamp int64, amount int32) {
		t.Helper()
		info := &pb.CSActivityInfo{ActivityId: proto.String(key)}
		out, e := f.s.p4Rebate(requestContext(info), id, info, stamp, LocalPlayNow(ctx))
		if e != nil {
			t.Fatal(e)
		}
		item := out[0].(*pb.SCActivityInfo).GetItemList()[0]
		if item.GetStatus() != 1 || item.GetLeft() != amount {
			t.Fatal("same personal-day actual consumption rebate", item)
		}
		before, _ := f.profile(t)
		b := &pb.CSConsumeReturnReward{ActivityId: proto.String(key)}
		first, e := f.s.p4Rebate(requestContext(b), id, b, stamp+1, LocalPlayNow(ctx))
		if e != nil {
			t.Fatal(e)
		}
		after, _ := f.profile(t)
		if after.GetDiamond()-before.GetDiamond() != int64(amount) {
			t.Fatal("actual rebate not credited")
		}
		retry, e := f.s.p4Rebate(requestContext(b), id, b, stamp+1, LocalPlayNow(ctx))
		current, _ := f.profile(t)
		if e != nil || !proto.Equal(first[0], retry[0]) || current.GetDiamond() != after.GetDiamond() {
			t.Fatal("rebate same stamp double credited", e)
		}
	}
	train(70200, 1, 3, 5)
	claim(70300, 12) // Actual120 shop +5 training, floor10%=12.
	change("nextDay", "training-forward-one-day")
	train(70400, 2, 15, 25)
	claim(70500, 2) // New personal day: only actual25 training, floor10%=2.
	final, game := f.profile(t)
	if final.GetDiamond() != paidPlayer.GetDiamond()-150+14 || count("item_007") != 2 {
		t.Fatal("two-day source/consumer conservation")
	}
	beforeCold, err := proto.Marshal(final)
	if err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	ctx = localClockBind(t, f, owner)
	cold, coldGame := f.profile(t)
	afterCold, err := proto.Marshal(cold)
	if err != nil || string(beforeCold) != string(afterCold) || count("item_007") != 2 {
		t.Fatal("training/paid inventory cold recovery", err)
	}
	original, err := p5Warrior(game, guid)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := p5Warrior(coldGame, guid)
	if err != nil || !proto.Equal(original, recovered) || trainingPending(recovered.GetTempTraining()) {
		t.Fatal("saved training/cold confirmation", err)
	}
	var facts []db.EventFact
	if err = db.GetIns().Where("player_id=? AND kind=?", id, "warrior_training").Find(&facts).Error; err != nil || len(facts) != 2 || facts[0].Day == facts[1].Day {
		t.Fatal("training event denominator/days", err)
	}
	var recoveredOrder db.PaymentOrder
	if err = db.GetIns().First(&recoveredOrder, "id=?", order.ID).Error; err != nil || recoveredOrder.ConfirmedAt != order.ConfirmedAt || recoveredOrder.CreatedAt != order.CreatedAt {
		t.Fatal("calendar changed real accounting timestamps", err)
	}
}
