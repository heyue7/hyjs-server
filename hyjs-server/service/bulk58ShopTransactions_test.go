package service

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestBulk58GuildDualCostsReceiptCold(t *testing.T) {
	f := p4Setup(t)
	if err := f.s.LoadP13GuildTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, row := range []any{&db.P13Guild{ID: "bulk58-guild", Name: "批量有限公会", Level: 1}, &db.P13GuildMember{PlayerID: f.id, GuildID: "bulk58-guild", Contribution: 90}} {
		if err := db.GetIns().Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P13GuildPolicy{}).Where("id=1").Updates(map[string]any{"management_enabled": true, "economy_enabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	key := constUtil.P4GuildShopPrefix + now.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
	var cfg db.P4CommerceRule
	if err := db.GetIns().First(&cfg, "id=?", key).Error; err != nil {
		t.Fatal(err)
	}
	var rules p4GuildRules
	if err := json.Unmarshal(cfg.Rules, &rules); err != nil {
		t.Fatal(err)
	}
	product := &rules.Products[0]
	product.DailyLimit = 10
	product.TotalLimit = 12
	product.Stock = 10
	product.Cost = "item_311,2"
	product.Rewards = "item_005,4"
	product.ContributionCost = 3
	if err := db.GetIns().Model(&cfg).Updates(map[string]any{"enabled": true, "rules": adminJSON(rules)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(30)
		p.Contribution = proto.Int32(30)
		g.Items = []*pb.SimpleItem{{Id: proto.String("item_311"), Num: proto.Int32(20)}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	listed, err := f.s.p4GuildShop(f.ctx, f.id, &pb.CSShopProductList{ShopType: proto.Int32(8)}, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	req := &pb.CSShopBuy{ShopType: proto.Int32(8), ShopItemId: listed[0].(*pb.SCShopProductList).ShopItemList[0].Id, BuyCount: proto.Int32(3)}
	result, err := f.s.p4GuildShop(f.ctx, f.id, req, 58001, now)
	if err != nil {
		t.Fatal(err)
	}
	out := result[0].(*pb.SCShopBuy)
	if out.GetContribution() != 21 || len(out.GetCost()) != 2 || out.Cost[0].GetNum() != 6 || out.Cost[1].GetNum() != 9 {
		t.Fatal("batch dual cost mismatch", out)
	}
	p, g := f.profile(t)
	if simpleOwned(p, g, "item_311") != 14 || simpleOwned(p, g, "item_005") != 12 {
		t.Fatal("batch inventory mismatch")
	}
	var member db.P13GuildMember
	if err = db.GetIns().First(&member, f.id).Error; err != nil || member.Contribution != 90 {
		t.Fatal("ranking contribution changed", err)
	}
	changed := proto.Clone(req).(*pb.CSShopBuy)
	changed.BuyCount = proto.Int32(2)
	if _, err = f.s.p4GuildShop(f.ctx, f.id, changed, 58001, now); err == nil {
		t.Fatal("same stamp different count accepted")
	}
	for _, n := range []int32{0, 101, 8} {
		bad := proto.Clone(req).(*pb.CSShopBuy)
		bad.BuyCount = proto.Int32(n)
		if _, err = f.s.p4GuildShop(f.ctx, f.id, bad, 58002, now); err == nil {
			t.Fatal("invalid/overlimit batch accepted", n)
		}
	}
	after, ag := f.profile(t)
	if !proto.Equal(p, after) || simpleOwned(after, ag, "item_005") != 12 {
		t.Fatal("rejected batch changed assets")
	}
	f.restart(t)
	replay, err := f.s.p4GuildShop(f.ctx, f.id, req, 58001, now)
	if err != nil || !proto.Equal(out, replay[0]) {
		t.Fatal("cold receipt", err)
	}
	var stock db.P4CommerceStock
	if err = db.GetIns().First(&stock, "rule_id=? AND product_id=?", key, product.ID).Error; err != nil || stock.Used != 3 {
		t.Fatal("stock not purchase count", err, stock.Used)
	}
}

func TestBulk58GGOTotalCreditReceiptAndLegacy(t *testing.T) {
	f, id, ctx, _ := terminalFixture(t)
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, f.s)
	if err := f.s.InstallGGOItems(); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().AutoMigrate(&db.P4CommerceRule{}, &db.ShopState{}, &db.P4CommerceStock{}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeGGO(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeAdminShopAvailability(ctx); err != nil {
		t.Fatal(err)
	}
	var cfg db.P4CommerceRule
	if err := db.GetIns().First(&cfg, "id=?", constUtil.GGOSeedRuleID).Error; err != nil {
		t.Fatal(err)
	}
	var rules p4CommerceRules
	if err := json.Unmarshal(cfg.Rules, &rules); err != nil {
		t.Fatal(err)
	}
	rules.Products[0].DailyLimit = 10
	rules.Products[0].TotalLimit = 8
	rules.Products[0].Stock = 8
	rules.Products[0].Cost = constUtil.GGOCurrencyID + ",2"
	rules.Products[0].Rewards = constUtil.GGOOverclockID + ",3"
	if err := db.GetIns().Model(&cfg).Update("rules", adminJSON(rules)).Error; err != nil {
		t.Fatal(err)
	}
	var owner db.Profile
	if err := db.GetIns().First(&owner, id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, g, e := decodeProfile(&owner)
		if e != nil {
			return e
		}
		if _, e = f.s.GrantFixedReward(p, g, constUtil.GGOCurrencyID+",100"); e != nil {
			return e
		}
		return saveProfile(tx, &owner, p, g)
	}); err != nil {
		t.Fatal(err)
	}
	catalogue, err := f.s.GGOShopCatalogue(ctx, id, owner.Session)
	if err != nil {
		t.Fatal(err)
	}
	count := int64(3)
	req := model.GGOShopBuyRequest{PlayerID: id, ProductID: catalogue.Items[0].ID, Version: catalogue.Version, RequestID: uuid.NewString(), Count: &count}
	before, err := roleWalletBalance(db.GetIns(), AdminProfileServerID(owner), owner.Id, owner.AccountID, false)
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.s.GGOShopBuy(ctx, req, owner.Session)
	if err != nil {
		t.Fatal(err)
	}
	if result.RewardAmount != 9 || result.TerminalPointsAdded != 9 || result.Catalogue.Balance != 94 || result.Catalogue.Items[0].Remaining != 5 {
		t.Fatal("batch totals", result)
	}
	after, err := roleWalletBalance(db.GetIns(), AdminProfileServerID(owner), owner.Id, owner.AccountID, false)
	if err != nil || after.BalanceCents != before.BalanceCents+900 {
		t.Fatal("terminal credit amount", err)
	}
	for _, n := range []int64{0, -1, 101, 6, math.MaxInt64} {
		bad := req
		bad.RequestID = uuid.NewString()
		bad.Count = &n
		if _, err = f.s.GGOShopBuy(ctx, bad, owner.Session); err == nil {
			t.Fatal("invalid/overlimit count accepted", n)
		}
	}
	other := int64(2)
	bad := req
	bad.Count = &other
	if _, err = f.s.GGOShopBuy(ctx, bad, owner.Session); err == nil {
		t.Fatal("UUID count conflict accepted")
	}
	f.restart(t)
	if err = f.s.InstallGGOItems(); err != nil {
		t.Fatal(err)
	}
	replay, err := f.s.GGOShopBuy(ctx, req, owner.Session)
	if err != nil || string(adminJSON(replay)) != string(adminJSON(result)) {
		t.Fatal("cold batch receipt", err)
	}
	after, err = roleWalletBalance(db.GetIns(), AdminProfileServerID(owner), owner.Id, owner.AccountID, false)
	if err != nil || after.BalanceCents != before.BalanceCents+900 {
		t.Fatal("replay credited twice", err)
	}
	// Omitted count is the legacy single purchase; a historical zero is read as one without rewriting it.
	single := req
	single.Count = nil
	single.RequestID = uuid.NewString()
	singleResult, err := f.s.GGOShopBuy(ctx, single, owner.Session)
	if err != nil || singleResult.RewardAmount != 3 {
		t.Fatal("omitted legacy count", err)
	}
	if err = db.GetIns().Model(&db.GGOShopReceipt{}).Where("player_id=? AND request_id=?", id, single.RequestID).UpdateColumn("count", 0).Error; err != nil {
		t.Fatal(err)
	}
	one := int64(1)
	single.Count = &one
	singleReplay, err := f.s.GGOShopBuy(ctx, single, owner.Session)
	if err != nil || string(adminJSON(singleReplay)) != string(adminJSON(singleResult)) {
		t.Fatal("historical zero receipt", err)
	}
	var stored db.GGOShopReceipt
	if err = db.GetIns().First(&stored, "player_id=? AND request_id=?", id, single.RequestID).Error; err != nil || stored.Count != 0 {
		t.Fatal("historical count rewritten", err)
	}
}
