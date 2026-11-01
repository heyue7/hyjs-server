package service

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestVIPShopUnpurchasedCardPreservesEligibilityAndReceipt(t *testing.T) {
	f, id, _, now := vipShop75Fixture(t, 0)
	var cfg db.VIPShopProduct
	if err := db.GetIns().First(&cfg, "id=?", constUtil.P3VIP10GrowthProduct).Error; err != nil {
		t.Fatal(err)
	}
	cfg.Enabled = true
	if err := db.GetIns().Save(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	list := func() *pb.ShopItem {
		t.Helper()
		out, err := f.s.vipShop(f.ctx, id, &pb.CSShopProductList{ShopType: proto.Int32(2)}, now)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range out[0].(*pb.SCShopProductList).GetShopItemList() {
			if item.GetId() == vipShopSKU(cfg) {
				return item
			}
		}
		t.Fatal("VIP10 product missing")
		return nil
	}
	item := list()
	if item.GetCanBuy() != 1 || item.GetDayLeftTimes() != 1 || item.GetCondition().GetValue() != 10 {
		t.Fatal("unbought VIP10 card should retain its quantity and required level", item)
	}
	buy := &pb.CSShopBuy{ShopType: proto.Int32(2), ShopItemId: item.Id, BuyCount: proto.Int32(1)}
	before, _ := f.profile(t)
	if _, err := f.s.vipShop(f.ctx, id, buy, now); err == nil {
		t.Fatal("VIP7 role purchased a VIP10 product")
	}
	after, _ := f.profile(t)
	if after.GetDiamond() != before.GetDiamond() || p1Count(t, &db.MembershipClaim{}, "player_id=? AND kind=?", id, "vip-shop:"+cfg.ID) != 0 {
		t.Fatal("ineligible purchase modified the wallet or claim")
	}
	if err := updateProfile(f.ctx, id, func(p *pb.Player, _ *model.GameState) error {
		p.Vip = proto.Int32(10)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Temporarily remove confirmed-payment eligibility in this isolated fixture.
	if err := db.GetIns().Model(&db.PaymentOrder{}).Where("player_id=? AND status=?", id, constUtil.P3PaymentConfirmed).Update("status", constUtil.P3PaymentPending).Error; err != nil {
		t.Fatal(err)
	}
	if list().GetCanBuy() != 1 {
		t.Fatal("unpaid but unbought card was labeled purchased")
	}
	if _, err := f.s.vipShop(f.ctx, id, buy, now); err == nil {
		t.Fatal("unpaid role purchased a VIP product")
	}
	if err := db.GetIns().Model(&db.PaymentOrder{}).Where("player_id=? AND status=?", id, constUtil.P3PaymentPending).Update("status", constUtil.P3PaymentConfirmed).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.vipShop(f.ctx, id, buy, now); err != nil {
		t.Fatal("eligible purchase failed", err)
	}
	if list().GetCanBuy() != 0 {
		t.Fatal("completed purchase was not displayed as purchased")
	}
	if _, err := f.s.vipShop(f.ctx, id, buy, now); err == nil {
		t.Fatal("duplicate purchase accepted")
	}
	f.restart(t)
	if list().GetCanBuy() != 0 || p1Count(t, &db.MembershipClaim{}, "player_id=? AND kind=?", id, "vip-shop:"+cfg.ID) != 1 {
		t.Fatal("purchase history changed across restart")
	}
}
