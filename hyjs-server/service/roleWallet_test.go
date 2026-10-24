package service

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"hyjs-server/db"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func roleWalletFixture(t *testing.T) context.Context {
	t.Helper()
	ctx := context.Background()
	if err := db.InitGame(ctx, filepath.Join(t.TempDir(), "role-wallet.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d, _ := db.GetIns().DB(); d.Close() })
	for _, fn := range []func(context.Context) error{db.InitializeGameAccountsSchema, initializeAccountWalletSchema, initializeTerminal} {
		if err := fn(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().AutoMigrate(&db.LocalWalletBalance{}, &db.LocalWalletEntry{}); err != nil {
		t.Fatal(err)
	}
	account := db.GameAccount{ID: 100, Username: "role-wallet-isolated", PasswordSalt: []byte{1}, PasswordHash: []byte{2}, Enabled: true}
	if err := db.GetIns().Create(&account).Error; err != nil {
		t.Fatal(err)
	}
	owner := db.Profile{Id: 7, ServerID: 1, AccountID: 100, Uid: "role-wallet-isolated", Session: make([]byte, 16)}
	if err := db.GetIns().Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	return ctx
}
func TestRoleWalletSamePlayerAcrossRealmsIsolation(t *testing.T) {
	roleWalletFixture(t)
	d := db.GetIns()
	now := time.Now()
	if err := d.Transaction(func(tx *gorm.DB) error {
		return terminalCredit(tx, 100, 1, 7, "same-request", "qa", "first", "", 300, now)
	}); err != nil {
		t.Fatal(err)
	}
	// The runtime profile belongs to realm 2 after switching the isolated fixture; shared receipts remain realm 1.
	if err := d.Model(&db.Profile{}).Where("id=?", 7).Update("server_id", 2).Error; err != nil {
		t.Fatal(err)
	}
	if err := d.Transaction(func(tx *gorm.DB) error {
		return terminalCredit(tx, 100, 2, 7, "same-request", "qa", "second", "", 500, now)
	}); err != nil {
		t.Fatal(err)
	}
	for _, server := range []int64{1, 2} {
		b, err := roleWalletBalance(d, server, 7, 100, false)
		want := int64(300)
		if server == 2 {
			want = 500
		}
		if err != nil || b.BalanceCents != want || b.Revision != 1 {
			t.Fatal(server, b, err)
		}
	}
	if err := d.Transaction(func(tx *gorm.DB) error { return terminalCredit(tx, 101, 2, 7, "bad-owner", "qa", "bad", "", 500, now) }); !errors.Is(err, ErrLocalWalletInvalid) {
		t.Fatal("owner mismatch", err)
	}
	var e db.LocalWalletEntry
	if err := walletFindEntry(d, 7, "same-request", "", &e); err != nil || e.Label != "second" {
		t.Fatal(e, err)
	}
	owner := db.Profile{Id: 7, AccountID: 100, ServerID: 2}
	state, err := terminalRoleState(d, &owner)
	if err != nil {
		t.Fatal(err)
	}
	state.SeenRevision = 1
	state.DeliveredRevision = 1
	if err = d.Save(state).Error; err != nil {
		t.Fatal(err)
	}
	owner.ServerID = 1
	first, err := terminalRoleState(d, &owner)
	if err != nil || first.SeenRevision != 0 || first.DeliveredRevision != 0 {
		t.Fatal(first, err)
	}
	for _, server := range []int64{1, 2} {
		receipt := db.TerminalRoleBurst{AccountID: 100, ServerID: server, PlayerID: 7, RequestID: "same-request", Day: "2026-10-25", AwardPoints: 7}
		if err = d.Create(&receipt).Error; err != nil {
			t.Fatal(err)
		}
	}
	duplicate := db.TerminalRoleBurst{AccountID: 100, ServerID: 2, PlayerID: 7, RequestID: "other", Day: "2026-10-25", AwardPoints: 7}
	if err = d.Create(&duplicate).Error; err == nil {
		t.Fatal("same role same day accepted")
	}
}
func TestRoleWalletLegacyExactMigrationRestartAndRollback(t *testing.T) {
	ctx := roleWalletFixture(t)
	d := db.GetIns()
	s := &GameService{}
	role := db.GameAccountRole{AccountID: 100, ServerID: 1, PlayerID: 7}
	if err := d.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	b := db.AccountWalletBalance{AccountID: 100, BalanceCents: 12345, Revision: 12}
	if err := d.Create(&b).Error; err != nil {
		t.Fatal(err)
	}
	order := "legacy-order"
	entry := db.AccountWalletEntry{AccountID: 100, ServerID: 1, PlayerID: 7, Key: "legacy:1:7:old", LegacyKey: "old", Kind: "add", DeltaCents: 12345, BalanceAfter: 12345, Revision: 12, OrderID: &order, At: 1760000, Source: "old", ReasonID: "kept", Label: "历史原文"}
	if err := d.Create(&entry).Error; err != nil {
		t.Fatal(err)
	}
	state := db.TerminalAccountState{AccountID: 100, SeenRevision: 8, DeliveredRevision: 12}
	d.Create(&state)
	income := db.TerminalIncome{AccountID: 100, ID: "income", Label: "原到账", AmountCents: 12345, CreatedAt: 1760000, Revision: 12, Source: "signup"}
	d.Create(&income)
	burst := db.TerminalBurst{AccountID: 100, RequestID: "daily", Day: "2026-10-25", AwardPoints: 17}
	d.Create(&burst)
	// Inject a target write failure after balance and entries: the transaction must leave no target rows or marker.
	if err := d.Exec("CREATE TRIGGER role_wallet_abort BEFORE INSERT ON terminal_role_incomes BEGIN SELECT RAISE(ABORT,'forced migration crash'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := s.InitializeRoleWallets(ctx); err == nil {
		t.Fatal("failure injection did not abort")
	}
	var n int64
	d.Model(&db.RoleWalletBalance{}).Count(&n)
	if n != 0 {
		t.Fatal("partial migration balance", n)
	}
	d.Model(&db.RoleWalletMigration{}).Count(&n)
	if n != 0 {
		t.Fatal("partial marker", n)
	}
	d.Exec("DROP TRIGGER role_wallet_abort")
	if err := s.InitializeRoleWallets(ctx); err != nil {
		t.Fatal(err)
	}
	var migrated db.RoleWalletEntry
	if err := d.First(&migrated).Error; err != nil {
		t.Fatal(err)
	}
	expected := db.RoleWalletEntry{AccountID: entry.AccountID, ServerID: 1, PlayerID: 7, Key: entry.Key, LegacyKey: entry.LegacyKey, Kind: entry.Kind, DeltaCents: entry.DeltaCents, BalanceAfter: entry.BalanceAfter, Revision: entry.Revision, OrderID: entry.OrderID, At: entry.At, Source: entry.Source, ReasonID: entry.ReasonID, Label: entry.Label}
	if !reflect.DeepEqual(migrated, expected) {
		t.Fatal(migrated, expected)
	}
	var gotState db.TerminalRoleState
	d.First(&gotState)
	if gotState.SeenRevision != 8 || gotState.DeliveredRevision != 12 {
		t.Fatal(gotState)
	}
	var gotIncome db.TerminalRoleIncome
	d.First(&gotIncome)
	if gotIncome.Label != income.Label || gotIncome.AmountCents != income.AmountCents || gotIncome.CreatedAt != income.CreatedAt || gotIncome.Revision != income.Revision {
		t.Fatal(gotIncome)
	}
	var gotBurst db.TerminalRoleBurst
	d.First(&gotBurst)
	if gotBurst.RequestID != burst.RequestID || gotBurst.Day != burst.Day || gotBurst.AwardPoints != burst.AwardPoints {
		t.Fatal(gotBurst)
	}
	if err := d.Transaction(func(tx *gorm.DB) error { return terminalCredit(tx, 100, 1, 7, "new", "qa", "new", "", 100, time.Now()) }); err != nil {
		t.Fatal(err)
	}
	if err := s.InitializeAccountWallets(ctx); err != nil {
		t.Fatal(err)
	}
	balance, err := walletReadBalance(d, 7, false)
	if err != nil || balance.BalanceCents != 12445 || balance.Revision != 13 {
		t.Fatal("restart overwrote new ledger", balance, err)
	}
	var original db.AccountWalletEntry
	d.First(&original)
	if !reflect.DeepEqual(original, entry) {
		t.Fatal("legacy altered", original)
	}
}
func TestRoleWalletAmbiguousLegacyRefused(t *testing.T) {
	ctx := roleWalletFixture(t)
	d := db.GetIns()
	d.Create(&db.AccountWalletBalance{AccountID: 100, BalanceCents: 100, Revision: 1})
	for _, r := range []db.GameAccountRole{{AccountID: 100, ServerID: 1, PlayerID: 7}, {AccountID: 100, ServerID: 2, PlayerID: 7}} {
		if err := d.Create(&r).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := (&GameService{}).InitializeRoleWallets(ctx); err == nil {
		t.Fatal("ambiguous legacy distributed")
	}
	var n int64
	d.Model(&db.RoleWalletMigration{}).Count(&n)
	if n != 0 {
		t.Fatal(n)
	}
}
