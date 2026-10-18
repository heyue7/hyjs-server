package service

import (
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP3VIP6GrowthOriginalFixedParcelClosedAndOperatorPreserved(t *testing.T) {
	f, _ := p3DemoProviderFixture(t)
	if err := f.s.seedP3VIP6GrowthProduct(db.GetIns()); err != nil {
		t.Fatal(err)
	}
	var product db.VIPShopProduct
	if err := db.GetIns().First(&product, "id=?", constUtil.P3VIP6GrowthProduct).Error; err != nil || product.Enabled || product.MinVIP != 6 || product.Cost != 700 || product.Reward != "item_040,1" || product.Source != constUtil.P3VIP6GrowthSource {
		t.Fatal("default-off named original parcel", product, err)
	}
	if err := db.GetIns().Model(&product).Updates(map[string]any{"cost": 710, "enabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.seedP3VIP6GrowthProduct(db.GetIns()); err != nil {
		t.Fatal(err)
	}
	var current db.VIPShopProduct
	if err := db.GetIns().First(&current, "id=?", product.ID).Error; err != nil || !current.Enabled || current.Cost != 710 {
		t.Fatal("seed overwrote operator quote", err)
	}
}

// The only finite qualification is Player15 for the original wear gate. Actual
// wallet payment produces VIP/diamonds; the paid source produces both treasures.
func TestP3VIP6GrowthActualWalletParcelTwoTreasuresAndCold(t *testing.T) {
	f, id := localPlayActivitiesFixture(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP5(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.seedP3VIP6GrowthProduct(db.GetIns()); err != nil {
		t.Fatal(err)
	}
	state := func() (*pb.Player, *model.GameState) {
		t.Helper()
		var row db.Profile
		if err := db.GetIns().First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		p, g, err := decodeProfile(&row)
		if err != nil {
			t.Fatal(err)
		}
		return p, g
	}
	now := time.Now()
	buy := &pb.CSShopBuy{ShopType: proto.Int32(2), ShopItemId: proto.String(constUtil.P3VIP6GrowthProduct), BuyCount: proto.Int32(1)}
	if _, err := f.s.vipShop(f.ctx, id, buy, now); err == nil {
		t.Fatal("closed source purchased")
	}
	if err := db.GetIns().Model(&db.VIPShopProduct{}).Where("id=?", constUtil.P3VIP6GrowthProduct).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	// The prior closed attempted quote is intentionally immutable today. A new
	// personal date may adopt publication; never delete the old frozen quote.
	if _, err := f.s.LocalPlayTimeChange(f.ctx, id, "nextDay", "vip6-next-publication-day"); err != nil {
		t.Fatal(err)
	}
	var clockOwner db.Profile
	if err := db.GetIns().First(&clockOwner, id).Error; err != nil {
		t.Fatal(err)
	}
	var bindErr error
	f.ctx, bindErr = f.s.BindLocalPlayContext(f.ctx, &clockOwner)
	if bindErr != nil {
		t.Fatal(bindErr)
	}
	now = LocalPlayNow(f.ctx)
	if _, err := f.s.vipShop(f.ctx, id, buy, now); err == nil {
		t.Fatal("unpaid account purchased")
	}
	f.s.P3AutoConfirmDemoPayments = true
	p3DemoProviderFund(t, f, id, "vip6-real-wallet-order", 200000)
	if _, err := f.s.generatePayment(f.ctx, id, p3DemoProviderRequest(t, "djsy.wuyou.andp2000"), 996001, time.Now()); err != nil {
		t.Fatal(err)
	}
	p, _ := state()
	if p.GetVip() != 7 || p.GetChargeDiamond() != 20000 || p.GetDiamond() != 23000 {
		t.Fatal("original actual order did not fund membership")
	}
	// The paid game-time fact is created during confirmation. Consume it at a
	// fresh request time; the earlier pre-payment observation must not become
	// the as-of cutoff for this later purchase.
	now = LocalPlayNow(f.ctx)
	paidCents, paidCharge, paidCount, err := PaymentTotals(db.GetIns().WithContext(f.ctx), id, 0, now.UnixMilli()+1)
	if err != nil || paidCents != 200000 || paidCharge != 20000 || paidCount != 1 {
		t.Fatal("fresh request did not see its real confirmed payment", paidCents, paidCharge, paidCount, err)
	}
	if _, err := f.s.vipShop(f.ctx, id, buy, now); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.vipShop(f.ctx, id, buy, now); err == nil {
		t.Fatal("same day parcel bought twice")
	}
	open := &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_040"), Num: proto.Int32(1)}}
	opened, err := f.s.p5BoxUse(f.ctx, id, open, 996002)
	if err != nil {
		t.Fatal(err)
	}
	award := opened[0].(*pb.SCItemUse).GetGet()
	if len(award.GetTreas()) != 2 || award.GetTreas()[0].GetTreasureId() != "trea_011" || award.GetTreas()[1].GetTreasureId() != "trea_013" {
		t.Fatal("original full parcel treasure family", award)
	}
	p, g := state()
	if p.GetDiamond() != 22300 || simpleOwned(p, g, "item_040") != 0 || simpleOwned(p, g, "item_005") != 1000 || simpleOwned(p, g, "item_048") != 4 || simpleOwned(p, g, "item_002") != 10 {
		t.Fatal("original complete fixed parcel award changed")
	}
	if err := updateProfile(f.ctx, id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(15); return nil }); err != nil {
		t.Fatal(err)
	}
	for _, gear := range award.GetTreas() {
		if gear.GetGuid() == "" {
			t.Fatal("real treasure GUID missing")
		}
		if _, err := f.s.treasureFormationOn(f.ctx, id, &pb.CSFormationOn{Id: gear.TreasureId, Guid: gear.Guid, FormatPos: proto.Int32(1)}); err != nil {
			t.Fatal("actual parcel wear consumer", err)
		}
		if _, err := f.s.treasureFormationOff(f.ctx, id, &pb.CSFormationOff{Id: gear.TreasureId, Guid: gear.Guid, FormatPos: proto.Int32(1)}); err != nil {
			t.Fatal(err)
		}
	}
	p, g = state()
	before, _ := json.Marshal(g)
	f.restart(t)
	afterP, afterG := state()
	after, _ := json.Marshal(afterG)
	if !proto.Equal(p, afterP) || string(before) != string(after) {
		t.Fatal("cold purchased parcel ownership changed")
	}
	if _, err := f.s.vipShop(f.ctx, id, buy, now); err == nil {
		t.Fatal("cold old-day claim was repurchased")
	}
}
