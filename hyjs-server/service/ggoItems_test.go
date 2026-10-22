package service

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestGGOConversionNoHitInvalidAndAtomicRollback(t *testing.T) {
	// An absent item cannot touch a database, even with a nil transaction.
	if err := ConvertGGOOverclockItem(nil, nil, nil, &model.GameState{}); err != nil {
		t.Fatal(err)
	}
	for _, items := range [][]*pb.SimpleItem{
		{{Id: proto.String(constUtil.GGOOverclockID), Num: proto.Int32(0)}},
		{{Id: proto.String(constUtil.GGOOverclockID), Num: proto.Int32(1)}, {Id: proto.String(constUtil.GGOOverclockID), Num: proto.Int32(1)}},
	} {
		if err := ConvertGGOOverclockItem(nil, &db.Profile{AccountID: 1}, nil, &model.GameState{Items: items}); err == nil {
			t.Fatal("invalid item accepted")
		}
	}
	if err := ConvertGGOOverclockItem(nil, &db.Profile{}, nil, &model.GameState{Items: []*pb.SimpleItem{{Id: proto.String(constUtil.GGOOverclockID), Num: proto.Int32(1)}}}); err == nil {
		t.Fatal("unlinked role accepted")
	}
	f, id, ctx, _ := terminalFixture(t)
	if err := f.s.InstallGGOItems(); err != nil {
		t.Fatal(err)
	}
	var owner db.Profile
	if err := db.GetIns().First(&owner, id).Error; err != nil {
		t.Fatal(err)
	}
	before, err := accountWalletBalance(db.GetIns(), owner.AccountID, false)
	if err != nil {
		t.Fatal(err)
	}
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked db.Profile
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, id).Error; e != nil {
			return e
		}
		p, g, e := decodeProfile(&locked)
		if e != nil {
			return e
		}
		if _, e = f.s.GrantFixedReward(p, g, constUtil.GGOOverclockID+",1"); e != nil {
			return e
		}
		if e = tx.Model(&db.Profile{}).Where("id=?", id).Update("account_id", 0).Error; e != nil {
			return e
		}
		locked.AccountID = 0
		return saveProfile(tx, &locked, p, g)
	})
	if err == nil {
		t.Fatal("unlinked account transaction accepted")
	}
	var unchanged db.Profile
	if err = db.GetIns().First(&unchanged, id).Error; err != nil || unchanged.AccountID != owner.AccountID || *unchanged.Game != *owner.Game {
		t.Fatal("unlinked rollback changed stored ownership/inventory", err)
	}
	rollback := errors.New("force rollback")
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		player, game, e := decodeProfile(&owner)
		if e != nil {
			return e
		}
		if _, e = f.s.GrantFixedReward(player, game, constUtil.GGOOverclockID+",2"); e != nil {
			return e
		}
		if e = ConvertGGOOverclockItem(tx, &owner, player, game); e != nil {
			return e
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	balance, err := accountWalletBalance(db.GetIns(), owner.AccountID, false)
	if err != nil || balance.BalanceCents != before.BalanceCents {
		t.Fatal("rollback balance", balance, err)
	}
	var count int64
	db.AccountsTable(db.GetIns(), "terminal_incomes").Where("account_id=? AND source=?", owner.AccountID, constUtil.GGOIncomeSource).Count(&count)
	if count != 0 {
		t.Fatal("rollback income", count)
	}
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, f.s)
	// A deliberately shifted game calendar must never shift financial timestamps.
	ctx = context.WithValue(ctx, constUtil.LocalPlayBoundKey, true)
	ctx = context.WithValue(ctx, constUtil.LocalPlayOffsetDaysKey, int64(7))
	wallStart := time.Now().UnixMilli()
	if LocalPlayNow(ctx).Sub(time.Now()) < 6*24*time.Hour {
		t.Fatal("shifted game calendar fixture missing")
	}

	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		player, game, e := decodeProfile(&owner)
		if e != nil {
			return e
		}
		if _, e = f.s.GrantFixedReward(player, game, constUtil.GGOOverclockID+",2"); e != nil {
			return e
		}
		// Explicit conversion also verifies the public hook independently from profile integration.
		if e = ConvertGGOOverclockItem(tx, &owner, player, game); e != nil {
			return e
		}
		return saveProfile(tx, &owner, player, game)
	})
	if err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	if err = db.GetIns().First(&owner, id).Error; err != nil {
		t.Fatal(err)
	}
	player, game, err := decodeProfile(&owner)
	if err != nil {
		t.Fatal(err)
	}
	if p4ResourceBalance(player, game, constUtil.GGOOverclockID) != 0 {
		t.Fatal("saved item retained")
	}
	if err = ConvertGGOOverclockItem(nil, &owner, player, game); err != nil {
		t.Fatal(err)
	}
	balance, err = accountWalletBalance(db.GetIns(), owner.AccountID, false)
	if err != nil || balance.BalanceCents != before.BalanceCents+200 {
		t.Fatal("credit/cold", balance, err)
	}
	db.AccountsTable(db.GetIns(), "terminal_incomes").Where("account_id=? AND source=?", owner.AccountID, constUtil.GGOIncomeSource).Count(&count)
	if count != 1 {
		t.Fatal("income count", count)
	}
	var income db.TerminalIncome
	var entry db.AccountWalletEntry
	if err = db.AccountsTable(db.GetIns(), "terminal_incomes").Where("account_id=? AND source=?", owner.AccountID, constUtil.GGOIncomeSource).First(&income).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.AccountsTable(db.GetIns(), "account_wallet_entries").Where("account_id=? AND source=?", owner.AccountID, constUtil.GGOIncomeSource).First(&entry).Error; err != nil {
		t.Fatal(err)
	}
	wallEnd := time.Now().UnixMilli()
	if income.CreatedAt < wallStart || income.CreatedAt > wallEnd || entry.At < wallStart || entry.At > wallEnd {
		t.Fatal("financial timestamp followed shifted calendar")
	}

}

func TestGGOShopAuthorityReceiptAndPriceEdit(t *testing.T) {
	f, id, ctx, actor := terminalFixture(t)
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
	var owner db.Profile
	if err := db.GetIns().First(&owner, id).Error; err != nil {
		t.Fatal(err)
	}
	catalogue, err := f.s.GGOShopCatalogue(ctx, id, owner.Session)
	if err != nil || !catalogue.Enabled || len(catalogue.Items) != 1 {
		t.Fatal(catalogue, err)
	}
	req := model.GGOShopBuyRequest{PlayerID: id, SessionHex: hex.EncodeToString(owner.Session), ProductID: catalogue.Items[0].ID, Version: catalogue.Version, RequestID: uuid.NewString()}
	if _, err = f.s.GGOShopBuy(ctx, req, owner.Session); err == nil {
		t.Fatal("empty currency purchase succeeded")
	}
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, f.s)
	if err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, g, e := decodeProfile(&owner)
		if e != nil {
			return e
		}
		if _, e = f.s.GrantFixedReward(p, g, constUtil.GGOCurrencyID+",10"); e != nil {
			return e
		}
		return saveProfile(tx, &owner, p, g)
	}); err != nil {
		t.Fatal(err)
	}
	result, err := f.s.GGOShopBuy(ctx, req, owner.Session)
	if err != nil || result.Catalogue.Balance != 9 || result.Catalogue.Items[0].Remaining != 0 || result.TerminalPointsAdded != 1 {
		t.Fatal(result, err)
	}
	replay, err := f.s.GGOShopBuy(ctx, req, owner.Session)
	if err != nil || string(adminJSON(replay)) != string(adminJSON(result)) {
		t.Fatal("replay", replay, err)
	}
	conflict := req
	conflict.ProductID = "another"
	if _, err = f.s.GGOShopBuy(ctx, conflict, owner.Session); err == nil {
		t.Fatal("UUID reused across products")
	}
	duplicate := req
	duplicate.RequestID = uuid.NewString()
	if _, err = f.s.GGOShopBuy(ctx, duplicate, owner.Session); err == nil {
		t.Fatal("daily limit ignored")
	}
	managed, err := f.s.AdminShop(ctx, constUtil.GGOShopKey)
	if err != nil {
		t.Fatal(err)
	}
	product := managed.Items[0]
	product.Cost.Amount = 2
	product.DailyLimit = 2
	if _, err = f.s.AdminSaveShopProduct(ctx, actor, constUtil.GGOShopKey, product.ID, model.AdminShopProductSaveRequest{Version: managed.Version, AdminShopProduct: product}, false); err != nil {
		t.Fatal(err)
	}
	after, err := f.s.GGOShopCatalogue(ctx, id, owner.Session)
	if err != nil || after.Items[0].Price != 2 || after.Items[0].Remaining != 1 || after.Version == catalogue.Version {
		t.Fatal("price edit", after, err)
	}
	if _, err = f.s.GGOShopBuy(ctx, duplicate, owner.Session); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale version", err)
	}
	duplicate.Version = after.Version
	second, err := f.s.GGOShopBuy(ctx, duplicate, owner.Session)
	if err != nil || second.Catalogue.Balance != 7 {
		t.Fatal(second, err)
	}
	// Re-initializing must preserve operator prices and existing receipts.
	if err = f.s.InitializeGGO(ctx); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	replay, err = f.s.GGOShopBuy(ctx, req, owner.Session)
	if err != nil || string(adminJSON(replay)) != string(adminJSON(result)) {
		t.Fatal("cold receipt", replay, err)
	}
	var stored db.Profile
	db.GetIns().First(&stored, id)
	p, g, err := decodeProfile(&stored)
	if err != nil || p4ResourceBalance(p, g, constUtil.GGOOverclockID) != 0 {
		t.Fatal("terminal reward remained in bag", err)
	}
	var cfg db.P4CommerceRule
	db.GetIns().First(&cfg, "id=?", constUtil.GGOSeedRuleID)
	var rules p4CommerceRules
	if json.Unmarshal(cfg.Rules, &rules) != nil || rules.Products[0].Cost != constUtil.GGOCurrencyID+",2" {
		t.Fatal("seed overwrote edited price")
	}
	managed, err = f.s.AdminShop(ctx, constUtil.GGOShopKey)
	if err != nil {
		t.Fatal(err)
	}
	settings := managed.Settings
	settings.Enabled = false
	if _, err = f.s.AdminSaveShopSettings(ctx, actor, constUtil.GGOShopKey, model.AdminShopSettingsSaveRequest{Version: managed.Version, AdminShopSettings: settings}); err != nil {
		t.Fatal(err)
	}
	closed, err := f.s.GGOShopCatalogue(ctx, id, owner.Session)
	if err != nil || closed.Enabled {
		t.Fatal("disabled shop", closed, err)
	}
	duplicate.Version = closed.Version
	duplicate.RequestID = uuid.NewString()
	if _, err = f.s.GGOShopBuy(ctx, duplicate, owner.Session); !errors.Is(err, constUtil.ErrP4CommerceNotOpen) {
		t.Fatal("disabled purchase", err)
	}
	if !ggoShopCostAllowed("ordinary", constUtil.GGOCurrencyID) || ggoShopCostAllowed("ggo", constUtil.GGOOverclockID) || !ggoShopCostAllowed("ordinary", "coin") {
		t.Fatal("currency boundaries")
	}
}

// TestGGOMySQLConcurrentTransactions runs only on the explicitly allocated MySQL schema.
func TestGGOMySQLConcurrentTransactions(t *testing.T) {
	if os.Getenv("HYJS_P1_MYSQL_DATABASE") == "" {
		t.Skip("explicit managed MySQL allocation required")
	}
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
	if err := f.s.InitializeServerGameClock(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.ServerGameClock{}).Where("server_id=?", f.s.serverGameClockID()).Updates(map[string]any{"offset_days": int64(7), "high_days": int64(7), "revision": int64(1)}).Error; err != nil {
		t.Fatal(err)
	}
	var owner db.Profile
	if err := db.GetIns().First(&owner, id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, id).Error; err != nil {
			return err
		}
		p, g, e := decodeProfile(&locked)
		if e != nil {
			return e
		}
		if _, e = f.s.GrantFixedReward(p, g, constUtil.GGOCurrencyID+",100"); e != nil {
			return e
		}
		return saveProfile(tx, &locked, p, g)
	}); err != nil {
		t.Fatal(err)
	}
	var seed db.P4CommerceRule
	if err := db.GetIns().First(&seed, "id=?", constUtil.GGOSeedRuleID).Error; err != nil {
		t.Fatal(err)
	}
	var definition p4CommerceRules
	if json.Unmarshal(seed.Rules, &definition) != nil || len(definition.Products) != 1 || definition.Products[0].DailyLimit != 1 || definition.Products[0].Cost != constUtil.GGOCurrencyID+",1" || definition.Products[0].Rewards != constUtil.GGOOverclockID+",1" {
		t.Fatal("initial seed contract mismatch")
	}
	catalogue, err := f.s.GGOShopCatalogue(ctx, id, owner.Session)
	if err != nil {
		t.Fatal(err)
	}
	req := model.GGOShopBuyRequest{PlayerID: id, ProductID: catalogue.Items[0].ID, Version: catalogue.Version, RequestID: uuid.NewString()}
	before, err := accountWalletBalance(db.GetIns(), owner.AccountID, false)
	if err != nil {
		t.Fatal(err)
	}
	run := func(same bool) (int, []*model.GGOShopBuyResponse, []error) {
		start := make(chan struct{})
		var wg sync.WaitGroup
		results := make([]*model.GGOShopBuyResponse, 10)
		errs := make([]error, 10)
		for i := range 10 {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				next := req
				if !same {
					next.RequestID = uuid.NewString()
				}
				<-start
				results[i], errs[i] = f.s.GGOShopBuy(ctx, next, owner.Session)
			}(i)
		}
		close(start)
		wg.Wait()
		success := 0
		for _, err := range errs {
			if err == nil {
				success++
			}
		}
		return success, results, errs
	}
	successes, results, errs := run(true)
	if successes != 10 {
		t.Fatalf("same UUID successes=%d errors=%v", successes, errs)
	}
	expected := string(adminJSON(results[0]))
	for _, out := range results {
		if string(adminJSON(out)) != expected {
			t.Fatal("same UUID differed")
		}
	}
	var receipts int64
	db.GetIns().Model(&db.GGOShopReceipt{}).Where("player_id=?", id).Count(&receipts)
	if receipts != 1 {
		t.Fatal("same UUID receipt count", receipts)
	}
	bound, e := f.s.BindLocalPlayContext(ctx, &owner)
	if e != nil {
		t.Fatal(e)
	}
	var firstState db.ShopState
	if e = db.GetIns().First(&firstState, "player_id=? AND product_id=?", id, constUtil.GGOShopStatePrefix+req.ProductID).Error; e != nil || firstState.Day != LocalPlayNow(bound).Format("2006-01-02") || !ServerGameClockContextBound(bound) {
		t.Fatal("shop day ignored server calendar", e)
	}
	// Move only this owned fixture's prior counter off the current day to exercise
	// ten distinct UUIDs against a fresh daily limit without changing the rule.
	if err := db.GetIns().Model(&db.ShopState{}).Where("player_id=?", id).Update("day", "2000-01-01").Error; err != nil {
		t.Fatal(err)
	}
	successes, _, errs = run(false)
	if successes != 1 {
		t.Fatalf("different UUID successes=%d errors=%v", successes, errs)
	}
	for _, err := range errs {
		if err != nil && err.Error() != "商品已下架或达到限购" {
			t.Fatal("concurrent request failed outside authoritative limit", err)
		}
	}

	balance, err := accountWalletBalance(db.GetIns(), owner.AccountID, false)
	if err != nil || balance.BalanceCents != before.BalanceCents+200 {
		t.Fatal("exact minimal-unit credit", balance, err)
	}
	var incomes, entries int64
	db.AccountsTable(db.GetIns(), "terminal_incomes").Where("account_id=? AND source=?", owner.AccountID, constUtil.GGOIncomeSource).Count(&incomes)
	db.AccountsTable(db.GetIns(), "account_wallet_entries").Where("account_id=? AND source=?", owner.AccountID, constUtil.GGOIncomeSource).Count(&entries)
	if incomes != 2 || entries != 2 {
		t.Fatal("ledger/income exactly once", entries, incomes)
	}
	db.GetIns().Model(&db.GGOShopReceipt{}).Where("player_id=?", id).Count(&receipts)
	if receipts != 2 {
		t.Fatal("different UUID receipt count", receipts)
	}
	if err = f.s.InitializeGGO(ctx); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	var cold db.P4CommerceRule
	db.GetIns().First(&cold, "id=?", constUtil.GGOSeedRuleID)
	if string(cold.Rules) != string(seed.Rules) || cold.Source != seed.Source || cold.Enabled != seed.Enabled {
		t.Fatal("restart/seed overwrote contract")
	}
	replay, err := f.s.GGOShopBuy(ctx, req, owner.Session)
	if err != nil || string(adminJSON(replay)) != expected {
		t.Fatal("cold concurrent receipt", err)
	}
	var stored db.Profile
	db.GetIns().First(&stored, id)
	p, g, err := decodeProfile(&stored)
	if err != nil || p4ResourceBalance(p, g, constUtil.GGOCurrencyID) != 98 || p4ResourceBalance(p, g, constUtil.GGOOverclockID) != 0 {
		t.Fatal("cold inventories", err)
	}
	// Pause the request after its outside-transaction session read, rotate the
	// session, then resume. An old session must not receive a historical receipt.
	raceKey := &struct{ name string }{"ggo-session-race"}
	raceCtx := context.WithValue(ctx, raceKey, true)
	read := make(chan struct{})
	resume := make(chan struct{})
	var once sync.Once
	callback := "ggo-session-rotation-test"
	if err = db.GetIns().Callback().Query().After("gorm:query").Register(callback, func(q *gorm.DB) {
		if q.Statement.Context.Value(raceKey) == true && q.Statement.Table == "profiles" {
			once.Do(func() { close(read); <-resume })
		}
	}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, e := f.s.GGOShopBuy(raceCtx, req, owner.Session); done <- e }()
	select {
	case <-read:
	case <-time.After(10 * time.Second):
		close(resume)
		t.Fatal("session race request did not reach authorization")
	}
	rotated := bytes.Repeat([]byte{7}, 16)
	updateErr := db.GetIns().Model(&db.Profile{}).Where("id=?", id).Update("session", rotated).Error
	close(resume)
	raceErr := <-done
	db.GetIns().Callback().Query().Remove(callback)
	if updateErr != nil || raceErr == nil {
		t.Fatal("stale session replay race accepted", updateErr, raceErr)
	}
	t.Log("MySQL: 10 same UUID requests -> 1 purchase / 1 immutable receipt; 10 distinct UUID requests -> 1 purchase under daily limit; each purchase +100 terminal minimal units, cold replay/seed preserved")
}

func TestGGOFullActivityInitializationPreservesShop(t *testing.T) {
	f, _ := adminModesFixture(t)
	if err := f.s.InstallGGOItems(); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeGGO(f.ctx); err != nil {
		t.Fatal(err)
	}
	// Cover every GGO rule alias, including future operator-created rule IDs.
	for _, id := range []string{"commerce:ggo:default", "commerce:ggo:custom-rule"} {
		if !adminActivityRedundantSource(id) {
			t.Fatal("own-menu GGO alias was accepted as an activity", id)
		}
	}
	if adminActivityRedundantSource("commerce:ggo-extra:default") || adminActivityRedundantSource("commerce:other:default") {
		t.Fatal("GGO prefix broadened other kinds")
	}
	for range 2 {
		if err := f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
			t.Fatal(err)
		}
		if err := f.s.InitializeReActivityCatalogue(f.ctx); err != nil {
			t.Fatal(err)
		}
		var rule db.P4CommerceRule
		if err := db.GetIns().First(&rule, "id=?", constUtil.GGOSeedRuleID).Error; err != nil || !rule.Enabled {
			t.Fatal("full initialization disabled GGO shop", err)
		}
		var imported int64
		if err := db.GetIns().Model(&db.AdminActivityDefinition{}).Where("source_activity_id LIKE ?", "commerce:"+constUtil.GGOShopKey+":%").Count(&imported).Error; err != nil || imported != 0 {
			t.Fatal("GGO activity alias imported", imported, err)
		}
	}
}
