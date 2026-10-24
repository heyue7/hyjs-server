package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func TestGGOShopStock63LastUnitAndLateRollback(t *testing.T) {
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
	var profile db.Profile
	db.GetIns().First(&profile, id)
	if err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, g, e := decodeProfile(&profile)
		if e != nil {
			return e
		}
		if _, e = f.s.GrantFixedReward(p, g, constUtil.GGOCurrencyID+",10"); e != nil {
			return e
		}
		return saveProfile(tx, &profile, p, g)
	}); err != nil {
		t.Fatal(err)
	}
	var cfg db.P4CommerceRule
	db.GetIns().First(&cfg, "id=?", constUtil.GGOSeedRuleID)
	var rules p4CommerceRules
	json.Unmarshal(cfg.Rules, &rules)
	rules.Products[0].Stock = 1
	rules.Products[0].DailyLimit = 10
	rules.Products[0].TotalLimit = 20
	if err := db.GetIns().Model(&cfg).Update("rules", adminJSON(rules)).Error; err != nil {
		t.Fatal(err)
	}
	page, err := f.s.GGOShopCatalogue(ctx, id, profile.Session)
	if err != nil {
		t.Fatal(err)
	}
	req := model.GGOShopBuyRequest{PlayerID: id, ProductID: page.Items[0].ID, Version: page.Version, RequestID: uuid.NewString()}
	beforeWallet, err := accountWalletBalance(db.GetIns(), profile.AccountID, false)
	if err != nil {
		t.Fatal(err)
	}
	var before db.Profile
	db.GetIns().First(&before, id)
	if err := db.GetIns().Callback().Create().Before("gorm:create").Register("ggo63:fail-buy-receipt", func(tx *gorm.DB) {
		if receipt, ok := tx.Statement.Dest.(*db.GGOShopReceipt); ok && receipt.RequestID == req.RequestID {
			tx.AddError(errors.New("isolated late receipt failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	_, failed := f.s.GGOShopBuy(ctx, req, profile.Session)
	if err := db.GetIns().Callback().Create().Remove("ggo63:fail-buy-receipt"); err != nil {
		t.Fatal(err)
	}
	if failed == nil {
		t.Fatal("injected failure accepted")
	}
	var after db.Profile
	db.GetIns().First(&after, id)
	afterWallet, err := accountWalletBalance(db.GetIns(), profile.AccountID, false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal([]byte(*before.Player), []byte(*after.Player)) || !bytes.Equal([]byte(*before.Game), []byte(*after.Game)) || *beforeWallet != *afterWallet {
		t.Fatal("late failure leaked currency/reward/terminal state")
	}
	for _, table := range []any{&db.GGOShopReceipt{}, &db.ShopState{}} {
		var count int64
		db.GetIns().Model(table).Where("player_id=?", id).Count(&count)
		if count != 0 {
			t.Fatal("late failure persisted receipt/limit", count)
		}
	}
	var used int64
	db.GetIns().Model(&db.P4CommerceStock{}).Select("COALESCE(SUM(used),0)").Where("rule_id=?", cfg.ID).Scan(&used)
	if used != 0 {
		t.Fatal("late failure reserved shared stock", used)
	}
	success, err := f.s.GGOShopBuy(ctx, req, profile.Session)
	if err != nil || success.Catalogue.Balance != 9 {
		t.Fatal("last stock unit unavailable after rollback", err)
	}
	var stock db.P4CommerceStock
	if err := db.GetIns().First(&stock, "rule_id=? AND product_id=?", cfg.ID, rules.Products[0].ID).Error; err != nil || stock.Used != 1 {
		t.Fatal("source stock not exactly one", stock, err)
	}
	another := req
	another.RequestID = uuid.NewString()
	if _, err = f.s.GGOShopBuy(ctx, another, profile.Session); err == nil {
		t.Fatal("oversold final stock")
	}
	retry, err := f.s.GGOShopBuy(ctx, req, profile.Session)
	if err != nil || !bytes.Equal(adminJSON(retry), adminJSON(success)) {
		t.Fatal("successful UUID changed after stock exhausted", err)
	}
}
