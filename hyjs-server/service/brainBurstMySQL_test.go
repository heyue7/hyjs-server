package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/config"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// This test runs only through tests/brain_burst_mysql.py, with three managed schemas.
// Finite valid profiles isolate the financial contract; native recharge is checked separately.
func TestBrainBurstMySQLThreeSchemaRollbackAndConcurrency(t *testing.T) {
	if os.Getenv("HYJS_BB_MYSQL_LEASE") != "managed-three-schema" {
		t.Skip("requires exclusive managed MySQL runner")
	}
	ctx := context.Background()
	shared := os.Getenv("HYJS_TEST_ACCOUNTS_SCHEMA")
	second := os.Getenv("HYJS_TEST_SECOND_REALM_SCHEMA")
	for _, name := range []string{shared, second, os.Getenv("HYJS_MYSQL_DATABASE")} {
		if !strings.HasPrefix(name, "hyjs_test_") {
			t.Fatal("requires managed temporary schema")
		}
	}
	previous := config.AppConfigIns
	current := config.AppConfig{}
	if previous != nil {
		current = *previous
	}
	current.Game.AccountsDatabase = shared
	config.AppConfigIns = &current
	t.Cleanup(func() { config.AppConfigIns = previous })
	cfg := db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: os.Getenv("HYJS_MYSQL_DATABASE")}
	open := func(c db.MySQLConfig) *gorm.DB {
		t.Helper()
		if err := db.InitGameMySQL(ctx, c); err != nil {
			t.Fatal(err)
		}
		d := db.GetIns()
		pool, _ := d.DB()
		pool.SetMaxOpenConns(6)
		pool.SetMaxIdleConns(3)
		t.Cleanup(func() { pool.Close() })
		return d
	}
	firstDB := open(cfg)
	initRealm := func() {
		t.Helper()
		for _, fn := range []func(context.Context) error{db.InitializeGameAccountsSchema, initializeAccountWalletSchema, initializeTerminal} {
			if err := fn(ctx); err != nil {
				t.Fatal(err)
			}
		}
		if err := db.GetIns().AutoMigrate(&db.LocalWalletPolicy{}, &db.LocalWalletBalance{}, &db.LocalWalletEntry{}, &db.PaymentOrder{}); err != nil {
			t.Fatal(err)
		}
		if err := db.GetIns().Create(&db.LocalWalletPolicy{ID: 1, Enabled: true, Version: constUtil.LocalWalletVersion, Source: constUtil.LocalWalletSource}).Error; err != nil {
			t.Fatal(err)
		}
	}
	initRealm()
	account := db.GameAccount{ID: 100, Username: "bb-mysql-migration", Nickname: "迁移账号", PasswordSalt: []byte{1}, PasswordHash: []byte{2}, Enabled: true}
	if err := db.AccountsTable(firstDB, "game_accounts").Create(&account).Error; err != nil {
		t.Fatal(err)
	}
	profile := func(server, accountID, id int64, uid string) db.Profile {
		t.Helper()
		player, _ := json.Marshal(&pb.Player{PlayerId: proto.Int64(id), Name: proto.String("契约角色"), Level: proto.Int32(1)})
		game, _ := json.Marshal(&model.GameState{})
		p, g := string(player), string(game)
		return db.Profile{Id: id, ServerID: server, AccountID: accountID, Uid: uid, Session: []byte("0123456789abcdef"), Player: &p, Game: &g}
	}
	owner1 := profile(1, 100, 7, "bb-mysql-migration")
	if err := firstDB.Create(&owner1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AccountsTable(firstDB, "game_account_roles").Create(&db.GameAccountRole{AccountID: 100, ServerID: 1, PlayerID: 7, Name: "契约角色", Level: 1}).Error; err != nil {
		t.Fatal(err)
	}
	oldBalance := db.AccountWalletBalance{AccountID: 100, BalanceCents: 123456, Revision: 15}
	if err := db.AccountsTable(firstDB, "account_wallet_balances").Create(&oldBalance).Error; err != nil {
		t.Fatal(err)
	}
	oldOrder := "old-order"
	entries := []db.AccountWalletEntry{{AccountID: 100, ServerID: 1, PlayerID: 7, Key: "legacy:1:7:entry-A", LegacyKey: "entry-A", Kind: "add", DeltaCents: 123500, BalanceAfter: 123500, Revision: 12, At: 1760000000000, Source: "legacy", ReasonID: "why", Label: "历史完整原文😀"}, {AccountID: 100, ServerID: 1, PlayerID: 7, Key: "old-payment", Kind: "debit", DeltaCents: -44, BalanceAfter: 123456, Revision: 15, OrderID: &oldOrder, At: 1760000000001, Source: "payment", ReasonID: "old", Label: "原订单"}}
	if err := db.AccountsTable(firstDB, "account_wallet_entries").Create(&entries).Error; err != nil {
		t.Fatal(err)
	}
	state := db.TerminalAccountState{AccountID: 100, SeenRevision: 10, DeliveredRevision: 15}
	income := db.TerminalIncome{AccountID: 100, ID: "old-income", Label: "到账原文😀", AmountCents: 123500, CreatedAt: 1760000000000, Revision: 12, Source: "legacy"}
	burst := db.TerminalBurst{AccountID: 100, RequestID: "prior-day-request", Day: "2026-10-25", AwardPoints: 31}
	for _, v := range []struct {
		table string
		row   any
	}{{"terminal_account_states", &state}, {"terminal_incomes", &income}, {"terminal_bursts", &burst}} {
		if err := db.AccountsTable(firstDB, v.table).Create(v.row).Error; err != nil {
			t.Fatal(err)
		}
	}
	s1 := &GameService{ServerID: 1, P3LocalWalletEnabled: true}
	s2 := &GameService{ServerID: 2, P3LocalWalletEnabled: true}
	fail := errors.New("injected mid-migration failure")
	callback := "bb-mysql-migration-abort"
	if err := firstDB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if strings.HasSuffix(tx.Statement.Table, "terminal_role_incomes") {
			tx.AddError(fail)
		}
	}); err != nil {
		t.Fatal(err)
	}
	err := s1.InitializeRoleWallets(ctx)
	firstDB.Callback().Create().Remove(callback)
	if !errors.Is(err, fail) {
		t.Fatal("failure injection did not abort", err)
	}
	for _, table := range []string{"role_wallet_balances", "role_wallet_entries", "role_wallet_migrations", "terminal_role_states", "terminal_role_incomes", "terminal_role_bursts"} {
		var n int64
		if err := db.AccountsTable(firstDB, table).Where("account_id=?", 100).Count(&n).Error; err != nil || n != 0 {
			t.Fatal("partial migration survived rollback", table, n, err)
		}
	}
	if err = s1.InitializeRoleWallets(ctx); err != nil {
		t.Fatal(err)
	}
	var copied []db.RoleWalletEntry
	if err = db.AccountsTable(firstDB, "role_wallet_entries").Where("account_id=?", 100).Order("revision,`key`").Find(&copied).Error; err != nil {
		t.Fatal(err)
	}
	if len(copied) != len(entries) {
		t.Fatal("lost ledger rows")
	}
	for i, e := range entries {
		want := db.RoleWalletEntry{AccountID: e.AccountID, ServerID: e.ServerID, PlayerID: e.PlayerID, Key: e.Key, LegacyKey: e.LegacyKey, Kind: e.Kind, DeltaCents: e.DeltaCents, BalanceAfter: e.BalanceAfter, Revision: e.Revision, OrderID: e.OrderID, At: e.At, Source: e.Source, ReasonID: e.ReasonID, Label: e.Label}
		if !reflect.DeepEqual(want, copied[i]) {
			t.Fatal("ledger changed", i)
		}
	}
	var gotState db.TerminalRoleState
	var gotIncome db.TerminalRoleIncome
	var gotBurst db.TerminalRoleBurst
	var marker db.RoleWalletMigration
	if err = db.AccountsTable(firstDB, "terminal_role_states").First(&gotState, "server_id=1 AND player_id=7").Error; err != nil || gotState.SeenRevision != state.SeenRevision || gotState.DeliveredRevision != state.DeliveredRevision {
		t.Fatal("cursor lost", err)
	}
	if err = db.AccountsTable(firstDB, "terminal_role_incomes").First(&gotIncome, "server_id=1 AND player_id=7").Error; err != nil || !reflect.DeepEqual(gotIncome, db.TerminalRoleIncome{AccountID: 100, ServerID: 1, PlayerID: 7, ID: income.ID, Label: income.Label, AmountCents: income.AmountCents, CreatedAt: income.CreatedAt, Revision: income.Revision, Source: income.Source}) {
		t.Fatal("income changed", err)
	}
	if err = db.AccountsTable(firstDB, "terminal_role_bursts").First(&gotBurst, "server_id=1 AND player_id=7").Error; err != nil || !reflect.DeepEqual(gotBurst, db.TerminalRoleBurst{AccountID: 100, ServerID: 1, PlayerID: 7, RequestID: burst.RequestID, Day: burst.Day, AwardPoints: burst.AwardPoints}) {
		t.Fatal("daily receipt changed", err)
	}
	if err = db.AccountsTable(firstDB, "role_wallet_migrations").First(&marker, "account_id=100").Error; err != nil || marker.BalanceCents != oldBalance.BalanceCents || marker.Revision != oldBalance.Revision || marker.EntryCount != int64(len(entries)) {
		t.Fatal("marker mismatch", err)
	}
	if err = firstDB.Transaction(func(tx *gorm.DB) error {
		return terminalCredit(tx, 100, 1, 7, "new-after-migration", "qa", "新账", "", 100, time.Now())
	}); err != nil {
		t.Fatal(err)
	}
	if err = s1.InitializeAccountWallets(ctx); err != nil {
		t.Fatal(err)
	}
	b, err := walletReadBalance(firstDB, 7, false)
	if err != nil || b.BalanceCents != 123556 || b.Revision != 16 {
		t.Fatal("restart overwrote new money", b, err)
	}
	var frozen []db.AccountWalletEntry
	if err = db.AccountsTable(firstDB, "account_wallet_entries").Where("account_id=100").Order("revision,`key`").Find(&frozen).Error; err != nil || !reflect.DeepEqual(frozen, entries) {
		t.Fatal("source history changed", err)
	}
	t.Log("PASS MySQL migration: exact balance/revision/transaction fields, cursor/income/Burst; mid-copy rollback all six targets; marker preserves later credit")
	secondCfg := cfg
	secondCfg.Database = second
	secondDB := open(secondCfg)
	initRealm()
	owner2 := profile(2, 100, 7, "bb-mysql-migration")
	if err = secondDB.Create(&owner2).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.AccountsTable(secondDB, "game_account_roles").Create(&db.GameAccountRole{AccountID: 100, ServerID: 2, PlayerID: 7, Name: "契约角色", Level: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err = secondDB.Transaction(func(tx *gorm.DB) error {
		return terminalCredit(tx, 100, 2, 7, "new-after-migration", "qa", "二服独立", "", 10000, time.Now())
	}); err != nil {
		t.Fatal(err)
	}
	firstMoney, _ := roleWalletBalance(firstDB, 1, 7, 100, false)
	secondMoney, _ := roleWalletBalance(secondDB, 2, 7, 100, false)
	if firstMoney.BalanceCents != 123556 || secondMoney.BalanceCents != 10000 {
		t.Fatal("same player ID leaked balance")
	}
	view, err := walletLedger(secondDB, 7, 0, 0, "", 1, 20)
	if err != nil || view.Total != 1 || len(view.Entries) != 1 || view.Entries[0].Label != "二服独立" {
		t.Fatal("cross-schema ledger leaked", err)
	}
	callCtx := func(s *GameService, owner db.Profile) context.Context {
		c := context.WithValue(ctx, constUtil.GameServiceContextKey, s)
		c = context.WithValue(c, constUtil.LocalWalletAPIContextKey, true)
		return context.WithValue(c, constUtil.LocalWalletSessionContextKey, owner.Session)
	}
	now := time.Date(2026, 10, 25, 2, 0, 0, 0, time.UTC)
	secondCtx := callCtx(s2, owner2)
	v2, err := s2.localWalletBurstAt(secondCtx, 7, "same-burst-request", now)
	if err != nil || v2.AlreadyApplied {
		t.Fatal("second realm daily", err)
	}
	seen, err := s2.LocalWalletSeen(secondCtx, 7, v2.Terminal.LatestIncomeRevision)
	if err != nil || seen.Terminal.UnreadCount != 0 {
		t.Fatal("second realm seen", err)
	}
	// Switch only between sequential API calls. Explicit tx callbacks below keep their own realm handle.
	firstDB = open(cfg)
	firstCtx := callCtx(s1, owner1)
	snapshot, err := s1.LocalWalletSnapshot(firstCtx, 7)
	if err != nil || snapshot.Terminal.UnreadCount == 0 {
		t.Fatal("second realm cleared first unread", err)
	}
	if _, err = s1.localWalletBurstAt(firstCtx, 7, "another", now); !errors.Is(err, ErrLocalWalletConflict) {
		t.Fatal("migrated daily receipt lost", err)
	}
	next := now.Add(24 * time.Hour)
	v1, err := s1.localWalletBurstAt(firstCtx, 7, "same-burst-request", next)
	if err != nil || v1.AlreadyApplied {
		t.Fatal("same request collided across realms", err)
	}
	retry, err := s1.localWalletBurstAt(firstCtx, 7, "same-burst-request", next)
	if err != nil || !retry.AlreadyApplied || retry.Revision != v1.Revision {
		t.Fatal("daily replay", err)
	}
	t.Log("PASS three physical schemas: same playerID balances/ledger/unread/day/request isolated; legacy daily receipt retained; daily retry frozen")
	// The real payment debit hook runs under the same role balance lock as native recharge.
	order := db.PaymentOrder{ID: "concurrent-order", PlayerID: 7, Cents: 300, Source: constUtil.P3PaymentSource, Status: constUtil.P3PaymentPending, CreatedAt: time.Now().Add(-time.Minute).UnixMilli(), ExpiresAt: time.Now().Add(time.Hour).UnixMilli()}
	before, err := walletReadBalance(firstDB, 7, false)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- firstDB.Transaction(func(tx *gorm.DB) error { return s1.LocalWalletDebitOrder(tx, &owner1, &order, time.Now()) }, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal("concurrent native debit", e)
		}
	}
	after, err := walletReadBalance(firstDB, 7, false)
	if err != nil || after.BalanceCents != before.BalanceCents-300 || after.Revision != before.Revision+1 {
		t.Fatal("concurrent duplicate debit", after, err)
	}
	// Distinct orders race for one finite balance. Only one can commit, without an external retry loop.
	low := &db.LocalWalletBalance{PlayerID: 7, BalanceCents: 100, Revision: after.Revision}
	if err = firstDB.Transaction(func(tx *gorm.DB) error { return walletSaveBalance(tx, low) }); err != nil {
		t.Fatal(err)
	}
	errs = make(chan error, 8)
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			o := order
			o.ID = fmt.Sprintf("distinct-order-%d", i)
			o.Cents = 100
			errs <- firstDB.Transaction(func(tx *gorm.DB) error { return s1.LocalWalletDebitOrder(tx, &owner1, &o, time.Now()) }, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		}(i)
	}
	wg.Wait()
	close(errs)
	success := 0
	for e := range errs {
		if e == nil {
			success++
		} else if !errors.Is(e, ErrLocalWalletInsufficient) {
			t.Fatal("unexpected concurrent failure", e)
		}
	}
	if success != 1 {
		t.Fatal("overspent finite balance", success)
	}
	t.Log("PASS InnoDB row locks: 12 concurrent identical native debits => one entry; 8 distinct orders racing finite balance => one success, no negative balance")
	// Concurrent first-character signup hooks in separate realm transactions share only the account lock.
	signupAccount := db.GameAccount{ID: 200, Username: "bb-mysql-signup", Nickname: "新账号", PasswordSalt: []byte{3}, PasswordHash: []byte{4}, Enabled: true}
	if err = db.AccountsTable(firstDB, "game_accounts").Create(&signupAccount).Error; err != nil {
		t.Fatal(err)
	}
	for i, d := range []*gorm.DB{firstDB, secondDB} {
		owner := profile(int64(i+1), 200, 8, "bb-mysql-signup")
		if err = d.Create(&owner).Error; err != nil {
			t.Fatal(err)
		}
	}
	errs = make(chan error, 2)
	for i, d := range []*gorm.DB{firstDB, secondDB} {
		wg.Add(1)
		go func(i int, d *gorm.DB) {
			defer wg.Done()
			errs <- d.Transaction(func(tx *gorm.DB) error { return terminalSignup(tx, &signupAccount, int64(i+1), 8) }, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		}(i, d)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal("concurrent first signup", e)
		}
	}
	var grants int64
	if err = db.AccountsTable(firstDB, "role_wallet_entries").Where("account_id=? AND source=?", 200, "signup").Count(&grants).Error; err != nil || grants != 1 {
		t.Fatal("signup double granted", grants, err)
	}
	a, _ := roleWalletBalance(firstDB, 1, 8, 200, false)
	b2, _ := roleWalletBalance(secondDB, 2, 8, 200, false)
	if a.BalanceCents+b2.BalanceCents != 500000 {
		t.Fatal("signup sum changed")
	}
	var uniqueErr error
	duplicate := db.GameAccountRole{AccountID: 100, ServerID: 1, PlayerID: 999, Name: "不得多建", Level: 1}
	uniqueErr = db.AccountsTable(firstDB, "game_account_roles").Create(&duplicate).Error
	if uniqueErr == nil {
		t.Fatal("same account same realm second role allowed")
	}
	if err = s1.InitializeRoleWallets(ctx); err != nil {
		t.Fatal("existing marker rejected later second realm", err)
	}
	t.Log("PASS account lock: simultaneous first-role signup across realms => exactly one configured grant; DB rejects second role in same realm; migrated marker accepts later realm")
	// Existing legacy characters never receive a new signup grant when entering another realm.
	if err = secondDB.Transaction(func(tx *gorm.DB) error { return terminalSignup(tx, &account, 2, 7) }); err != nil {
		t.Fatal(err)
	}
	var oldSignup int64
	if err = db.AccountsTable(firstDB, "role_wallet_entries").Where("account_id=? AND source=?", 100, "signup").Count(&oldSignup).Error; err != nil || oldSignup != 0 {
		t.Fatal("legacy character gained second-realm signup", oldSignup, err)
	}
	// Independent processes invoke real CreatePlayer, each with its own physical realm DB.
	// Reuse the built test executable: no concurrent compilation or unsafe global DB swapping.
	fresh := db.GameAccount{ID: 300, Username: "bb-mysql-real-create", Nickname: "真实创角", PasswordSalt: []byte{5}, PasswordHash: []byte{6}, Enabled: true}
	if err = db.AccountsTable(firstDB, "game_accounts").Create(&fresh).Error; err != nil {
		t.Fatal(err)
	}
	for i, d := range []*gorm.DB{firstDB, secondDB} {
		shell := db.Profile{Id: 9, ServerID: int64(i + 1), AccountID: 300, Uid: fresh.Username, Session: []byte("0123456789abcdef")}
		if err = d.Create(&shell).Error; err != nil {
			t.Fatal(err)
		}
	}
	barrier := t.TempDir()
	executable, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	workerContext, cancelWorkers := context.WithCancel(ctx)
	t.Cleanup(func() { cancelWorkers(); wg.Wait() })
	outputs := make(chan string, 2)
	errs = make(chan error, 2)
	for realm := 1; realm <= 2; realm++ {
		wg.Add(1)
		go func(realm int) {
			defer wg.Done()
			command := exec.CommandContext(workerContext, executable, "-test.run=^TestBrainBurstMySQLConcurrentCreateWorker$", "-test.v=true", "-test.timeout=40s")
			command.Env = append(os.Environ(), fmt.Sprintf("HYJS_BB_CREATE_REALM=%d", realm), "HYJS_BB_CREATE_BARRIER="+barrier)
			raw, e := command.CombinedOutput()
			outputs <- string(raw)
			errs <- e
		}(realm)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		_, one := os.Stat(filepath.Join(barrier, "1.ready"))
		_, two := os.Stat(filepath.Join(barrier, "2.ready"))
		if one == nil && two == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("real create workers did not reach barrier")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err = os.WriteFile(filepath.Join(barrier, "go"), []byte("start"), 0600); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	close(errs)
	close(outputs)
	for output := range outputs {
		t.Log(strings.TrimSpace(output))
	}
	for e := range errs {
		if e != nil {
			t.Fatal("real concurrent CreatePlayer failed", e)
		}
	}
	var realGrants int64
	if err = db.AccountsTable(firstDB, "role_wallet_entries").Where("account_id=? AND source=?", 300, "signup").Count(&realGrants).Error; err != nil || realGrants != 1 {
		t.Fatal("real concurrent creation duplicated signup", realGrants, err)
	}
	var realRoles []db.GameAccountRole
	if err = db.AccountsTable(firstDB, "game_account_roles").Where("account_id=? AND name<>? AND level>=?", 300, "", 1).Find(&realRoles).Error; err != nil || len(realRoles) != 2 {
		t.Fatal("real completed roles missing", len(realRoles), err)
	}
	t.Log("PASS two simultaneous real CreatePlayer processes: both realm characters persisted, exactly one account signup grant; pre-existing legacy character prevents later signup")

}

func TestBrainBurstMySQLConcurrentCreateWorker(t *testing.T) {
	realm := os.Getenv("HYJS_BB_CREATE_REALM")
	if realm != "1" && realm != "2" {
		t.Skip("managed parent process worker only")
	}
	if os.Getenv("HYJS_BB_MYSQL_LEASE") != "managed-three-schema" {
		t.Fatal("missing managed parent lease")
	}
	schema := os.Getenv("HYJS_MYSQL_DATABASE")
	serverID := int64(1)
	if realm == "2" {
		schema = os.Getenv("HYJS_TEST_SECOND_REALM_SCHEMA")
		serverID = 2
	}
	current := config.AppConfig{}
	current.Game.AccountsDatabase = os.Getenv("HYJS_TEST_ACCOUNTS_SCHEMA")
	config.AppConfigIns = &current
	ctx := context.Background()
	cfg := db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: schema}
	if err := db.InitGameMySQL(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	pool, _ := db.GetIns().DB()
	defer pool.Close()
	pool.SetMaxOpenConns(2)
	service := &GameService{ServerID: serverID, P3LocalWalletEnabled: true}
	if err := service.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, service)
	barrier := os.Getenv("HYJS_BB_CREATE_BARRIER")
	if err := os.WriteFile(filepath.Join(barrier, realm+".ready"), []byte("ready"), 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(barrier, "go")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("parent did not release creation barrier")
		}
		time.Sleep(10 * time.Millisecond)
	}
	owner, err := CreatePlayer(ctx, 9, "真实并发角色"+realm, "hero_043")
	if err != nil || owner == nil || owner.Player == nil {
		t.Fatal("real CreatePlayer", err)
	}
	t.Log("real realm", serverID, "CreatePlayer committed")
}
