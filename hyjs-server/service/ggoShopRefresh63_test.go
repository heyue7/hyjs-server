package service

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func TestGGOShopRefresh63FrozenLifecycle(t *testing.T) {
	f, id, ctx, _ := terminalFixture(t)
	if err := f.s.InstallGGOItems(); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().AutoMigrate(&db.P4CommerceRule{}, &db.P4CommerceState{}, &db.ShopState{}, &db.P4CommerceStock{}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP4(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeGGO(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeAdminShopAvailability(ctx); err != nil {
		t.Fatal(err)
	}
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, f.s)
	var owner db.Profile
	db.GetIns().First(&owner, id)
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
	var cfg db.P4CommerceRule
	db.GetIns().First(&cfg, "id=?", constUtil.GGOSeedRuleID)
	var rules p4CommerceRules
	json.Unmarshal(cfg.Rules, &rules)
	yes := true
	rules.RefreshEnabled = &yes
	rules.FreeCap = 1
	rules.FreeSeconds = 7200
	rules.ResourceRefreshCap = 2
	rules.RefreshCost = constUtil.GGOCurrencyID + ",7"
	rules.Products[0].DailyLimit = 2
	rules.Products[0].TotalLimit = 3
	rules.Products[0].Stock = 10
	rules.Products[0].QuantityTiers = []model.AdminShopQuantityTier{{Amount: 2, Weight: 1, Cost: model.AdminResourceAmount{ResourceID: constUtil.GGOCurrencyID, Amount: 3}}}
	saveRules := func() {
		t.Helper()
		if e := db.GetIns().Model(&cfg).Update("rules", adminJSON(rules)).Error; e != nil {
			t.Fatal(e)
		}
	}
	saveRules()
	read := func() *model.GGOShopCatalogueResponse {
		t.Helper()
		out, e := f.s.GGOShopCatalogue(ctx, id, owner.Session)
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	page := read()
	if len(page.Items) != 6 || page.Refresh == nil || page.Refresh.FreeLeft != 1 || page.Refresh.Generation != 1 || len(page.Version) != 64 {
		t.Fatal("initial catalogue", page)
	}
	for _, p := range page.Items {
		if p.Amount != 2 || p.Price != 3 || p.Remaining != 1 {
			t.Fatal("tier quote", p)
		}
	}
	if string(adminJSON(read())) != string(adminJSON(page)) {
		t.Fatal("query rerolled")
	}
	req := model.GGOShopBuyRequest{PlayerID: id, SessionHex: hex.EncodeToString(owner.Session), ProductID: page.Items[0].ID, Version: page.Version, RequestID: uuid.NewString()}
	first, e := f.s.GGOShopBuy(ctx, req, owner.Session)
	if e != nil || first.Catalogue.Balance != 97 || first.TerminalPointsAdded != 2 || first.Catalogue.Items[0].Remaining != 0 {
		t.Fatal("buy", first, e)
	}
	replay, e := f.s.GGOShopBuy(ctx, req, owner.Session)
	if e != nil || !bytes.Equal(adminJSON(first), adminJSON(replay)) {
		t.Fatal("buy replay", e)
	}
	if _, e = f.s.GGOShopRefresh(ctx, model.GGOShopRefreshRequest{PlayerID: id, Version: page.Version, RequestID: req.RequestID}, owner.Session); e == nil {
		t.Fatal("buy UUID reused for refresh")
	}
	rules.Products[0].QuantityTiers[0].Amount = 3
	rules.Products[0].QuantityTiers[0].Cost.Amount = 5
	rules.Products[0].Cost = constUtil.GGOCurrencyID + ",9"
	saveRules()
	stable := read()
	if stable.Version != page.Version || stable.Items[1].Amount != 2 || stable.Items[1].Price != 3 {
		t.Fatal("live edit changed accepted quote", stable)
	}
	req2 := req
	req2.RequestID = uuid.NewString()
	req2.ProductID = page.Items[1].ID
	second, e := f.s.GGOShopBuy(ctx, req2, owner.Session)
	if e != nil || second.Catalogue.Balance != 94 {
		t.Fatal("second slot", e)
	}
	for _, p := range second.Catalogue.Items {
		if p.Remaining != 0 {
			t.Fatal("source daily limit bypassed")
		}
	}
	refresh := model.GGOShopRefreshRequest{PlayerID: id, Version: page.Version, RequestID: uuid.NewString()}
	free, e := f.s.GGOShopRefresh(ctx, refresh, owner.Session)
	if e != nil || free.CostAmount != 0 || free.CostID != "" || free.Catalogue.Balance != 94 || free.Catalogue.Refresh.Generation != 2 || free.Catalogue.Refresh.FreeLeft != 0 {
		t.Fatal("free refresh", free, e)
	}
	for _, p := range free.Catalogue.Items {
		if p.Amount != 3 || p.Price != 5 || p.Remaining != 0 {
			t.Fatal("refresh quote/source limit", p)
		}
	}
	again, e := f.s.GGOShopRefresh(ctx, refresh, owner.Session)
	if e != nil || !bytes.Equal(adminJSON(free), adminJSON(again)) {
		t.Fatal("refresh replay", e)
	}
	cross := req
	cross.RequestID = refresh.RequestID
	cross.ProductID = constUtil.GGOShopRefreshReceiptProduct
	if _, e = f.s.GGOShopBuy(ctx, cross, owner.Session); e == nil {
		t.Fatal("refresh UUID reused for buy")
	}
	stale := refresh
	stale.RequestID = uuid.NewString()
	if _, e = f.s.GGOShopRefresh(ctx, stale, owner.Session); !errors.Is(e, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale refresh accepted", e)
	}
	paidReq := refresh
	paidReq.Version = free.Catalogue.Version
	paidReq.RequestID = uuid.NewString()
	paid, e := f.s.GGOShopRefresh(ctx, paidReq, owner.Session)
	if e != nil || paid.CostID != constUtil.GGOCurrencyID || paid.CostAmount != 7 || paid.Catalogue.Balance != 87 || paid.Catalogue.Refresh.PaidLeft != 1 {
		t.Fatal("paid refresh", paid, e)
	}
	// Force the final receipt write to fail: fee, generation and profile must roll back together.
	var beforeRollback db.P4CommerceState
	if e := db.GetIns().First(&beforeRollback, "player_id=? AND rule_id=?", id, ggoQuoteRuleID(cfg.ID)).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GetIns().Callback().Create().Before("gorm:create").Register("ggo63:reject-refresh-receipt", func(tx *gorm.DB) {
		if tx.Statement.Table == "ggo_shop_receipts" {
			if value, ok := tx.Statement.Dest.(map[string]any); ok && value["product_id"] == constUtil.GGOShopRefreshReceiptProduct {
				tx.AddError(errors.New("isolated rollback probe"))
			}
		}
	}); e != nil {
		t.Fatal(e)
	}

	rollbackReq := paidReq
	rollbackReq.RequestID = uuid.NewString()
	rollbackReq.Version = paid.Catalogue.Version
	_, rollbackErr := f.s.GGOShopRefresh(ctx, rollbackReq, owner.Session)
	if e := db.GetIns().Callback().Create().Remove("ggo63:reject-refresh-receipt"); e != nil {
		t.Fatal(e)
	}

	if rollbackErr == nil {
		t.Fatal("injected receipt failure accepted")
	}
	var afterRollback db.P4CommerceState
	db.GetIns().First(&afterRollback, "player_id=? AND rule_id=?", id, ggoQuoteRuleID(cfg.ID))
	if !bytes.Equal(beforeRollback.Payload, afterRollback.Payload) || read().Balance != 87 {
		t.Fatal("receipt failure leaked fee or generation")
	}
	// Turning all new features off retains accepted slots and successful bytes.
	no := false
	rules.RefreshEnabled = &no
	rules.Products[0].QuantityTiers = nil
	saveRules()
	stopped := read()
	if stopped.Refresh == nil || stopped.Refresh.Enabled || stopped.Items[0].Price != 5 || stopped.Items[0].Amount != 3 {
		t.Fatal("feature disable replaced quotes")
	}
	failReq := paidReq
	failReq.RequestID = uuid.NewString()
	failReq.Version = stopped.Version
	if _, e = f.s.GGOShopRefresh(ctx, failReq, owner.Session); e == nil {
		t.Fatal("disabled refresh accepted")
	}
	if read().Balance != 87 {
		t.Fatal("failure deducted")
	}
	f.restart(t)
	cold := read()
	if cold.Refresh.Generation != paid.Catalogue.Refresh.Generation || cold.Items[0].Price != 5 || cold.Items[0].Amount != 3 || cold.Balance != 87 {
		t.Fatal("cold state changed")
	}
	coldReceipt, e := f.s.GGOShopRefresh(ctx, paidReq, owner.Session)
	if e != nil || !bytes.Equal(adminJSON(coldReceipt), adminJSON(paid)) {
		t.Fatal("cold refresh replay", e)
	}
	coldBuy, e := f.s.GGOShopBuy(ctx, req, owner.Session)
	if e != nil || !bytes.Equal(adminJSON(coldBuy), adminJSON(first)) {
		t.Fatal("historic buy bytes changed", e)
	}
	var quotaRow db.P4CommerceState
	if e := db.GetIns().First(&quotaRow, "player_id=? AND rule_id=?", id, ggoQuoteRuleID(cfg.ID)).Error; e != nil {
		t.Fatal(e)
	}
	var quota ggoQuoteState
	json.Unmarshal(quotaRow.Payload, &quota)
	quota.Day = LocalPlayNow(ctx).AddDate(0, 0, -1).Format("2006-01-02")
	quota.PaidUsed = 2
	quota.FreeLeft = 0
	quota.FreeAt = LocalPlayNow(ctx).Add(-7200 * time.Second).UnixMilli()
	quota.SlotBought[quota.Slots[0].ID] = 1
	if e := db.GetIns().Model(&quotaRow).Update("payload", adminJSON(quota)).Error; e != nil {
		t.Fatal(e)
	}
	recovered := read()
	if recovered.Refresh.Generation != quota.Generation || recovered.Refresh.FreeLeft != 1 || recovered.Refresh.PaidLeft != 2 || recovered.Items[0].Remaining != 0 {
		t.Fatal("day/recovery reset slot or failed quota", recovered.Refresh)
	}
	rules.Products[0].Enabled = &no
	saveRules()
	closed := read()
	if len(closed.Items) != 0 {
		t.Fatal("disabled source revived")
	}
	var quote db.P4CommerceState
	db.GetIns().First(&quote, "player_id=? AND rule_id=?", id, ggoQuoteRuleID(cfg.ID))
	if len(quote.RuleID) > 64 || len(quote.Payload) > 262144 {
		t.Fatal("unbounded quote state")
	}
}

func TestGGOShopRefresh63OriginalMetadata(t *testing.T) {
	s := &GameService{}
	if e := s.LoadTables("../../analysis/tables"); e != nil {
		t.Fatal(e)
	}
	if e := s.LoadAdminCatalogue("../../analysis/tables", ""); e != nil {
		t.Fatal(e)
	}
	exp := s.ggoRewardMetadata("hero_exp_004")
	if exp == nil || exp["icon"] != "exp_icon_004" || exp["star"] != "5" || exp["name"] != "高级经验水晶" {
		t.Fatal("gold EXP metadata", exp)
	}
	if s.table("Item", "itemId", "hero_exp_004") != nil {
		t.Fatal("EXP was fabricated as Item")
	}
	frag := s.ggoRewardMetadata("frag_hero_001")
	if frag == nil || frag["icon"] == "" || frag["name"] == "" {
		t.Fatal("fragment original target metadata", frag)
	}
	if s.ggoRewardMetadata("hero_001") != nil {
		t.Fatal("ordinary recruitable hero accepted as item")
	}
}
