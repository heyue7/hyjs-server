package service

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"testing"
	"time"
)

func terminalFixture(t *testing.T) (*p1Fixture, int64, context.Context, *model.AdminPrincipal) {
	t.Helper()
	f, id, ctx := localWalletFixture(t)
	localWalletEnable(t)
	if err := f.s.InitializeAdminGameServers(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeGlobalGameAccounts(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeAccountWallets(ctx); err != nil {
		t.Fatal(err)
	}
	actor := adminWalletTestActor(t, f, constUtil.AdminRoleOperator)
	ctx = context.WithValue(ctx, constUtil.AdminServerScopeContextKey, int64(1))
	return f, id, ctx, actor
}
func TestTerminalSignupSettingsAndLegacyIsolation(t *testing.T) {
	f, id, ctx, actor := terminalFixture(t)
	view, err := f.s.LocalWalletSnapshot(ctx, id)
	if err != nil || view.BalanceCents != 0 || view.Terminal.UnreadCount != 0 {
		t.Fatal("migration must not signup-credit legacy", view, err)
	}
	cfg, err := AdminTerminalSettings(ctx, actor)
	if err != nil || cfg.SignupPoints != 5000 || len(cfg.Weekly) != 0 {
		t.Fatal(cfg, err)
	}
	stale := *cfg
	cfg.SignupPoints = 4321
	saved, err := AdminSaveTerminalSettings(ctx, actor, *cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = AdminSaveTerminalSettings(ctx, actor, stale); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale settings", err)
	}
	viewer := adminWalletTestActor(t, f, constUtil.AdminRoleViewer)
	if _, err = AdminSaveTerminalSettings(ctx, viewer, *saved); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("viewer wrote", err)
	}
	account, err := registerGlobalGameAccount(ctx, "terminal-new-"+uuid.NewString(), "isolated-terminal-pass", "新用户")
	if err != nil {
		t.Fatal(err)
	}
	shell, err := LoginAccount(ctx, account.Username, "isolated-terminal-pass")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(ctx, shell.Id, "新用户角色", "hero_043"); err != nil {
		t.Fatal(err)
	}
	account.PlayerID = shell.Id
	var owner db.Profile
	if err = db.GetIns().First(&owner, account.PlayerID).Error; err != nil {
		t.Fatal(err)
	}
	balance, err := walletReadBalance(db.GetIns(), owner.Id, false)
	if err != nil || balance.BalanceCents != 432100 {
		t.Fatal("actual registration grant without a player", balance, err)
	}
	var n int64
	db.AccountsTable(db.GetIns(), "terminal_role_incomes").Where("account_id=? AND source=?", owner.AccountID, "signup").Count(&n)
	if n != 1 {
		t.Fatal("signup card missing", n)
	}
	// Creating a second realm role must not repeat the account's first-character signup grant.
	second := owner
	second.Id = 0
	second.ServerID = 2
	second.Uid = "terminal-second-" + uuid.NewString()
	if err = db.GetIns().Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		var global db.GameAccount
		if err := db.AccountsTable(tx, "game_accounts").First(&global, owner.AccountID).Error; err != nil {
			return err
		}
		return terminalSignup(tx, &global, 2, second.Id)
	}); err != nil {
		t.Fatal(err)
	}
	secondBalance, err := walletReadBalance(db.GetIns(), second.Id, false)
	if err != nil || secondBalance.BalanceCents != 0 || secondBalance.Revision != 0 {
		t.Fatal("signup repeated across realm", secondBalance, err)
	}
	f.restart(t)
	if err = f.s.InitializeLocalWallet(ctx); err != nil {
		t.Fatal(err)
	}
	cfg, err = AdminTerminalSettings(ctx, actor)
	if err != nil || cfg.SignupPoints != 4321 || cfg.Version != saved.Version {
		t.Fatal("cold settings", cfg, err)
	}
	balance, err = walletReadBalance(db.GetIns(), owner.Id, false)
	if err != nil || balance.BalanceCents != 432100 {
		t.Fatal("cold registration ledger", balance, err)
	}
}
func TestTerminalBurstDailyReplayUnreadAndRealDate(t *testing.T) {
	f, id, ctx, actor := terminalFixture(t)
	var owner db.Profile
	db.GetIns().First(&owner, id)
	now := time.Date(2026, 10, 21, 15, 59, 59, 0, time.UTC)
	key := uuid.NewString()
	if _, err := f.s.localWalletBurstAt(ctx, id, key, now); !errors.Is(err, ErrLocalWalletInsufficient) {
		t.Fatal("insufficiency", err)
	}
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		return terminalCredit(tx, owner.AccountID, 1, id, "fixture-credit", "admin_manual", "隔离QA", "", 10000, now)
	}); err != nil {
		t.Fatal(err)
	}
	cfg, err := AdminTerminalSettings(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	cfg.BurstMinPoints = 7
	cfg.BurstMaxPoints = 7
	cfg, err = AdminSaveTerminalSettings(ctx, actor, *cfg)
	if err != nil {
		t.Fatal(err)
	}
	view, err := f.s.localWalletBurstAt(ctx, id, key, now)
	if err != nil || view.BurstAwardPoints != 7 || view.BalanceCents != 10600 || view.Terminal.UnreadCount != 2 || view.Terminal.CanBurst {
		t.Fatal("daily action", view, err)
	}
	retry, err := f.s.localWalletBurstAt(ctx, id, key, now)
	if err != nil || !retry.AlreadyApplied || retry.Revision != view.Revision || retry.BalanceCents != view.BalanceCents {
		t.Fatal("retry", retry, err)
	}
	if _, err = f.s.localWalletBurstAt(ctx, id, uuid.NewString(), now); !errors.Is(err, ErrLocalWalletConflict) {
		t.Fatal("second same day", err)
	}
	// A separate realm role starts with its own balance and daily allowance.
	var crossRole db.Profile
	db.GetIns().First(&crossRole, id)
	crossRole.Id = 0
	crossRole.Uid = "cross-burst-" + uuid.NewString()
	crossRole.ServerID = 2
	if err = db.GetIns().Create(&crossRole).Error; err != nil {
		t.Fatal(err)
	}
	crossService := GameService{ServerID: 2, P3LocalWalletEnabled: true}
	crossCtx := context.WithValue(ctx, constUtil.GameServiceContextKey, &crossService)
	if err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		return terminalCredit(tx, owner.AccountID, 2, crossRole.Id, "fixture-credit", "admin_manual", "独立余额", "", 1000, now)
	}); err != nil {
		t.Fatal(err)
	}
	if cross, err := crossService.localWalletBurstAt(crossCtx, crossRole.Id, key, now); err != nil || cross.AlreadyApplied {
		t.Fatal("realm role daily allowance", cross, err)
	}
	revision := view.Terminal.LatestIncomeRevision
	if _, err = f.s.LocalWalletSeen(ctx, id, revision+1); !errors.Is(err, ErrLocalWalletInvalid) {
		t.Fatal("unreturned income ack", err)
	}
	if err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		return terminalCredit(tx, owner.AccountID, 1, id, "fixture-new-income", "admin_manual", "并发新收入", "", 100, now)
	}); err != nil {
		t.Fatal(err)
	}
	seen, err := f.s.LocalWalletSeen(ctx, id, revision)
	if err != nil || seen.Terminal.UnreadCount != 1 {
		t.Fatal("ack cleared concurrent income", seen, err)
	}
	if _, err = f.s.localWalletBurstAt(ctx, id, uuid.NewString(), now.Add(2*time.Second)); err != nil {
		t.Fatal("Beijing midnight reset", err)
	}
	cfg.BurstEnabled = false
	if _, err = AdminSaveTerminalSettings(ctx, actor, *cfg); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.localWalletBurstAt(ctx, id, uuid.NewString(), now.Add(24*time.Hour)); !errors.Is(err, ErrLocalWalletDisabled) {
		t.Fatal("closed daily action", err)
	}
	f.restart(t)
	retry, err = f.s.localWalletBurstAt(ctx, id, key, now.Add(48*time.Hour))
	if err != nil || !retry.AlreadyApplied || retry.BurstAwardPoints != 7 {
		t.Fatal("cold UUID replay", retry, err)
	}
	var entries []db.RoleWalletEntry
	db.AccountsTable(db.GetIns(), "role_wallet_entries").Where("server_id=? AND player_id=? AND source=?", int64(1), owner.Id, "burst").Find(&entries)
	if len(entries) != 4 {
		t.Fatal("cost and income must be separate, twice", len(entries))
	}
}
func TestTerminalDistributionResumeScopeAndWeekly(t *testing.T) {
	f, id, ctx, actor := terminalFixture(t)
	cfg, err := AdminTerminalSettings(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	// A second realm role shares the account and must never double-credit it.
	var owner db.Profile
	db.GetIns().First(&owner, id)
	other := owner
	other.Id = 0
	other.Uid = "terminal-other-" + uuid.NewString()
	other.ServerID = 2
	if err = db.GetIns().Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	duplicate := owner
	duplicate.Id = 0
	duplicate.Uid = "terminal-duplicate-" + uuid.NewString()
	if err = db.GetIns().Create(&duplicate).Error; err != nil {
		t.Fatal(err)
	}
	secondCtx := context.WithValue(ctx, constUtil.GameServiceContextKey, &GameService{ServerID: 2})
	remote, err := registerGlobalGameAccount(secondCtx, "terminal-remote-"+uuid.NewString(), "isolated-terminal-pass", "异服用户")
	if err != nil {
		t.Fatal(err)
	}
	shellremote, loginErr := LoginAccount(secondCtx, remote.Username, "isolated-terminal-pass")
	if loginErr != nil {
		t.Fatal(loginErr)
	}
	if _, err = CreatePlayer(secondCtx, shellremote.Id, "测试新角色", "hero_043"); err != nil {
		t.Fatal(err)
	}
	remote.PlayerID = shellremote.Id
	var remoteOwner db.Profile
	db.GetIns().First(&remoteOwner, remote.PlayerID)
	remoteOwner.Player = owner.Player
	remoteOwner.Game = owner.Game
	if err = db.GetIns().Save(&remoteOwner).Error; err != nil {
		t.Fatal(err)
	}
	req := model.TerminalDistributeRequest{RequestID: uuid.NewString(), Points: 25, Reason: "隔离QA", Version: cfg.Version}
	result, err := AdminTerminalDistribute(ctx, actor, req)
	if err != nil || result.Recipients != 1 || result.Credited != 1 {
		t.Fatal("realm selection/protection/account dedupe", result, err)
	}
	retry, err := AdminTerminalDistribute(ctx, actor, req)
	if err != nil || !retry.AlreadyApplied || retry.Credited != 1 {
		t.Fatal("manual retry", retry, err)
	}
	req.Points = 26
	if _, err = AdminTerminalDistribute(ctx, actor, req); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("UUID payload conflict", err)
	}
	var batch db.TerminalBatch
	if err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		var e error
		batch, _, e = terminalFreeze(tx, 1, uuid.NewString(), 10, "中断恢复QA", "admin_manual", cfg.Version, time.Now())
		return e
	}); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	if err = f.s.ProcessTerminalDistributions(ctx); err != nil {
		t.Fatal("cold pending resume", err)
	}
	db.GetIns().First(&batch, "id=?", batch.ID)
	if batch.Status != "complete" || batch.Credited != 1 {
		t.Fatal(batch)
	}
	now := time.Date(2026, 10, 21, 3, 0, 0, 0, time.UTC)
	cfg.Weekly = []model.TerminalWeeklyRule{{ID: "wed", Weekday: 3, Points: 11, Enabled: true}}
	if _, err = AdminSaveTerminalSettings(ctx, actor, *cfg); err != nil {
		t.Fatal(err)
	}
	if err = f.s.processTerminalDistributionsAt(ctx, now); err != nil {
		t.Fatal(err)
	}
	if err = f.s.processTerminalDistributionsAt(ctx, now); err != nil {
		t.Fatal(err)
	}
	if err = f.s.processTerminalDistributionsAt(ctx, now.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.GetIns().Model(&db.TerminalBatch{}).Where("kind=?", "admin_weekly").Count(&count)
	if count != 1 {
		t.Fatal("weekly duplicate/historical catchup", count)
	}
	balance, err := walletReadBalance(db.GetIns(), owner.Id, false)
	if err != nil || balance.BalanceCents != 4600 {
		t.Fatal("frozen distributions total", balance, err)
	}
	remoteBalance, err := walletReadBalance(db.GetIns(), remoteOwner.Id, false)
	if err != nil || remoteBalance.BalanceCents != 500000 {
		t.Fatal("distribution crossed realm", remoteBalance, err)
	}
	list, err := AdminTerminalDistributions(ctx, actor, 1, 20)
	if err != nil || list.Total != 3 {
		t.Fatal(list, err)
	}
}

func TestTerminalBlockedBatchDoesNotStopOtherAccountsAndPolicyClosure(t *testing.T) {
	f, id, ctx, actor := terminalFixture(t)
	var owner db.Profile
	db.GetIns().First(&owner, id)
	cfg, err := AdminTerminalSettings(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := registerGlobalGameAccount(ctx, "terminal-safe-"+uuid.NewString(), "isolated-terminal-pass", "正常账号")
	if err != nil {
		t.Fatal(err)
	}
	shellfresh, loginErr := LoginAccount(ctx, fresh.Username, "isolated-terminal-pass")
	if loginErr != nil {
		t.Fatal(loginErr)
	}
	if _, err = CreatePlayer(ctx, shellfresh.Id, "测试新角色", "hero_043"); err != nil {
		t.Fatal(err)
	}
	fresh.PlayerID = shellfresh.Id
	var other db.Profile
	db.GetIns().First(&other, fresh.PlayerID)
	other.Player = owner.Player
	other.Game = owner.Game
	if err = db.GetIns().Save(&other).Error; err != nil {
		t.Fatal(err)
	}
	var batch db.TerminalBatch
	if err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		var e error
		batch, _, e = terminalFreeze(tx, 1, uuid.NewString(), 1, "保护变更QA", "admin_manual", cfg.Version, time.Now())
		return e
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.AccountsTable(db.GetIns(), "game_accounts").Where("id=?", owner.AccountID).Update("protected", true).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.ProcessTerminalDistributions(ctx); err != nil {
		t.Fatal("one blocked receipt stopped worker", err)
	}
	db.GetIns().First(&batch, "id=?", batch.ID)
	if batch.Status != "blocked" || batch.Credited != 1 {
		t.Fatal("blocked status/progress", batch)
	}
	result, err := AdminTerminalDistribute(ctx, actor, model.TerminalDistributeRequest{RequestID: uuid.NewString(), Points: 2, Reason: "其他投放QA", Version: cfg.Version})
	if err != nil || result.Credited != 1 {
		t.Fatal("blocked batch stopped unrelated batch", result, err)
	}
	if err = db.AccountsTable(db.GetIns(), "game_accounts").Where("id=?", owner.AccountID).Update("protected", false).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.ProcessTerminalDistributions(ctx); err != nil {
		t.Fatal(err)
	}
	db.GetIns().First(&batch, "id=?", batch.ID)
	if batch.Status != "complete" || batch.Credited != 2 {
		t.Fatal("blocked receipt recovery", batch)
	}
	balance, err := walletReadBalance(db.GetIns(), other.Id, false)
	if err != nil || balance.BalanceCents != 500300 {
		t.Fatal("recovery duplicated healthy recipient", balance, err)
	}
	key := uuid.NewString()
	now := time.Now()
	if _, err = f.s.localWalletBurstAt(ctx, id, key, now); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.LocalWalletPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	retry, err := f.s.localWalletBurstAt(ctx, id, key, now.Add(24*time.Hour))
	if err != nil || !retry.AlreadyApplied {
		t.Fatal("policy closure blocked successful UUID retry", retry, err)
	}
	if _, err = f.s.localWalletBurstAt(ctx, id, uuid.NewString(), now.Add(24*time.Hour)); !errors.Is(err, ErrLocalWalletDisabled) {
		t.Fatal("policy closure permitted new daily action", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err = f.s.ServeTerminalDistributions(cancelled); err != nil {
		t.Fatal("cancelled lifecycle", err)
	}
}
