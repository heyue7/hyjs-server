package service

import (
	"context"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

func vipShop75Fixture(t *testing.T, total int64) (*p1Fixture, int64, db.VIPShopProduct, time.Time) {
	t.Helper()
	f, id := p3DemoProviderFixture(t)
	f.id = id
	p3DemoProviderFund(t, f, id, "vip75-fund", 600)
	f.s.P3AutoConfirmDemoPayments = true
	now := time.Now()
	if _, err := f.s.generatePayment(f.ctx, id, p3DemoProviderRequest(t, "djsy.wuyou.andp0006"), 751234, now); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
		p.Vip = proto.Int32(7)
		p.Diamond = proto.Int64(10000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var cfg db.VIPShopProduct
	if err := db.GetIns().First(&cfg, "id=?", "p3vip7_growth_local").Error; err != nil {
		t.Fatal(err)
	}
	cfg.Reward = "item_063,1"
	cfg.Cost = 1888
	cfg.CostResource = "diamond"
	cfg.TotalLimit = total
	cfg.Enabled = true
	cfg.Source = "isolated VIP75 finite paid qualification fixture"
	if err := db.GetIns().Save(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	return f, id, cfg, now
}
func vipShop75Buy(t *testing.T, f *p1Fixture, id int64, cfg db.VIPShopProduct, now time.Time) error {
	t.Helper()
	_, err := f.s.vipShop(f.ctx, id, &pb.CSShopBuy{ShopType: proto.Int32(2), ShopItemId: proto.String(vipShopSKU(cfg)), BuyCount: proto.Int32(1)}, now)
	return err
}
func TestVIPShop75TotalLimitCrossDateColdHistoryAndQuote(t *testing.T) {
	f, id, cfg, now := vipShop75Fixture(t, 1)
	stale := cfg
	stale.Cost = 800
	if err := vipShop75Buy(t, f, id, stale, now); err == nil {
		t.Fatal("old800 quote remained accepted")
	}
	before, _ := f.profile(t)
	if err := vipShop75Buy(t, f, id, cfg, now); err != nil {
		t.Fatal(err)
	}
	after, g := f.profile(t)
	if before.GetDiamond()-after.GetDiamond() != 1888 || simpleOwned(after, g, "item_063") != 1 {
		t.Fatal("VIP cost/reward mismatch")
	}
	for _, at := range []time.Time{now, now.AddDate(0, 0, 1), now.AddDate(0, 0, -1), now.AddDate(0, 1, 0)} {
		if err := vipShop75Buy(t, f, id, cfg, at); err == nil {
			t.Fatal("role total reset by date")
		}
	}
	f.restart(t)
	if err := vipShop75Buy(t, f, id, cfg, now.AddDate(0, 0, 7)); err == nil {
		t.Fatal("cold restart reset total")
	}
	var claims int64
	if err := db.GetIns().Model(&db.MembershipClaim{}).Where("player_id=? AND kind=?", id, "vip-shop:"+cfg.ID).Count(&claims).Error; err != nil || claims != 1 {
		t.Fatal("claim count changed", claims, err)
	}
	items, err := f.s.p3VIPShopList(db.GetIns(), after, now.AddDate(0, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range items {
		if item.GetId() == vipShopSKU(cfg) {
			found = true
			if item.GetTotalTimes() != 1 || item.GetTotalLeftTimes() != 0 || item.GetCanBuy() != 0 || item.GetDayLeftTimes() != 0 {
				t.Fatal("total cap missing from original response", item)
			}
		}
	}
	if !found {
		t.Fatal("configured lifetime product missing")
	}
	final, _ := f.profile(t)
	if final.GetDiamond() != after.GetDiamond() {
		t.Fatal("rejected duplicate consumed diamonds")
	}
}
func TestVIPShop75MigratedHistoricalPurchaseCountsAgainstNewTotal(t *testing.T) {
	f, id, cfg, now := vipShop75Fixture(t, 1)
	claim := db.MembershipClaim{PlayerID: id, Kind: "vip-shop:" + cfg.ID, Period: "2026-01-01", Reward: "item_041,1", At: now.AddDate(0, 0, -30).UnixMilli()}
	if err := db.GetIns().Create(&claim).Error; err != nil {
		t.Fatal(err)
	}
	if err := vipShop75Buy(t, f, id, cfg, now); err == nil {
		t.Fatal("legacy accepted purchase excluded from lifetime cap")
	}
	var still db.MembershipClaim
	if err := db.GetIns().First(&still, "player_id=? AND kind=? AND period=?", id, claim.Kind, claim.Period).Error; err != nil || still.Reward != claim.Reward {
		t.Fatal("historical purchase rewritten", err)
	}
}
func TestVIPShop75ZeroRetainsDailyAndAdminTotalEditable(t *testing.T) {
	f, id, cfg, now := vipShop75Fixture(t, 0)
	if err := vipShop75Buy(t, f, id, cfg, now); err != nil {
		t.Fatal(err)
	}
	f.ctx = context.WithValue(f.ctx, constUtil.LocalPlayBoundKey, true)
	f.ctx = context.WithValue(f.ctx, constUtil.LocalPlayOwnerKey, id)
	f.ctx = context.WithValue(f.ctx, constUtil.LocalPlayOffsetDaysKey, int64(1))
	if err := vipShop75Buy(t, f, id, cfg, now.AddDate(0, 0, 1)); err != nil {
		t.Fatal("other daily VIP behavior changed", err)
	}
	if err := AdminInitialize(f.ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err := InitializeAdminOperations(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeAdminShopAvailability(f.ctx); err != nil {
		t.Fatal(err)
	}
	user := db.AdminUser{Username: "vip75-" + uuid.NewString(), PasswordHash: "fixture", Role: constUtil.AdminRoleOperator, Enabled: true}
	if err := db.GetIns().Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
	shop, err := f.s.AdminShop(f.ctx, "vip")
	if err != nil {
		t.Fatal(err)
	}
	var product model.AdminShopProduct
	for _, p := range shop.Items {
		if p.ID == cfg.ID {
			product = p
		}
	}
	product.TotalLimit = 2
	if _, err = f.s.AdminSaveShopProduct(f.ctx, actor, "vip", cfg.ID, model.AdminShopProductSaveRequest{AdminShopProduct: product, Version: shop.Version}, false); err != nil {
		t.Fatal(err)
	}
	var saved db.VIPShopProduct
	if err = db.GetIns().First(&saved, "id=?", cfg.ID).Error; err != nil || saved.TotalLimit != 2 {
		t.Fatal("admin total limit not saved", err)
	}
	before, _ := f.profile(t)
	if err = vipShop75Buy(t, f, id, saved, now.AddDate(0, 0, 2)); err == nil {
		t.Fatal("price edit reset two historical claims")
	}
	after, _ := f.profile(t)
	if before.GetDiamond() != after.GetDiamond() {
		t.Fatal("over-cap purchase deducted")
	}
}
