package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func adminWalletAdjustFixture(t *testing.T) (*p1Fixture, int64, context.Context, *model.AdminPrincipal, db.Profile) {
	t.Helper()
	adminWalletTestEnvironment(t)
	f, id, ctx, actor := terminalFixture(t)
	var owner db.Profile
	if err := db.GetIns().First(&owner, id).Error; err != nil {
		t.Fatal(err)
	}
	return f, id, ctx, actor, owner
}

func adminWalletAdjustRequest(action string, points, revision int64) model.AdminWalletAdjustRequest {
	return model.AdminWalletAdjustRequest{RequestID: uuid.NewString(), Action: action, Points: points, Reason: "隔离钱包调整验证", Revision: &revision}
}

func adminWalletAdjustCounts(t *testing.T, accountID int64, entries, incomes, audits int64) {
	t.Helper()
	for _, check := range []struct {
		table string
		want  int64
	}{
		{"account_wallet_entries", entries}, {"terminal_incomes", incomes},
	} {
		var count int64
		if err := db.AccountsTable(db.GetIns(), check.table).Where("account_id=? AND source=?", accountID, constUtil.AdminWalletAdjustSource).Count(&count).Error; err != nil || count != check.want {
			t.Fatalf("%s count=%d want=%d err=%v", check.table, count, check.want, err)
		}
	}
	var count int64
	if err := db.GetIns().Model(&db.AdminAudit{}).Where("action=? AND target=?", constUtil.AdminWalletAdjustAuditAction, fmt.Sprint(accountID)).Count(&count).Error; err != nil || count != audits {
		t.Fatalf("audit count=%d want=%d err=%v", count, audits, err)
	}
}

func TestAdminWalletAdjustCreditDebitReplayAndColdSharedBalance(t *testing.T) {
	f, id, ctx, actor, owner := adminWalletAdjustFixture(t)
	credit := adminWalletAdjustRequest(constUtil.AdminWalletAdjustCredit, 25, 0)
	first, err := AdminWalletAdjust(ctx, actor, id, credit)
	if err != nil || first.BalanceCents != 2500 || first.Revision != 1 || first.AmountCents != 2500 || first.AlreadyApplied {
		t.Fatal("credit", first, err)
	}
	debit := adminWalletAdjustRequest(constUtil.AdminWalletAdjustDebit, 9, 1)
	second, err := AdminWalletAdjust(ctx, actor, id, debit)
	if err != nil || second.BalanceCents != 1600 || second.Revision != 2 || second.AmountCents != -900 || second.AlreadyApplied {
		t.Fatal("debit", second, err)
	}
	if _, err = AdminWalletAdjust(ctx, actor, id, adminWalletAdjustRequest(constUtil.AdminWalletAdjustDebit, 17, 2)); !errors.Is(err, ErrLocalWalletInsufficient) {
		t.Fatal("insufficient funds", err)
	}
	if _, err = AdminWalletAdjust(ctx, actor, id, adminWalletAdjustRequest(constUtil.AdminWalletAdjustCredit, 1, 1)); !errors.Is(err, constUtil.ErrAdminWalletAdjustRevision) {
		t.Fatal("stale revision", err)
	}
	retry, err := AdminWalletAdjust(ctx, actor, id, credit)
	if err != nil || !retry.AlreadyApplied || retry.BalanceCents != 1600 || retry.Revision != 2 || retry.AmountCents != 2500 {
		t.Fatal("replay after later transaction", retry, err)
	}
	adminWalletAdjustCounts(t, owner.AccountID, 2, 1, 2)
	var income db.TerminalIncome
	if err = db.AccountsTable(db.GetIns(), "terminal_incomes").First(&income, "account_id=? AND id=?", owner.AccountID, constUtil.AdminWalletAdjustKeyPrefix+credit.RequestID).Error; err != nil || income.Label != constUtil.AdminWalletAdjustCreditLabel+credit.Reason || income.AmountCents != 2500 || income.Revision != 1 {
		t.Fatal("credit income frozen fields", income, err)
	}
	// 同账号另一个区服角色读取同一余额和支出兼容投影。
	other := owner
	other.Id, other.ServerID, other.Uid = 0, 2, "adjust-other-"+uuid.NewString()
	if err = db.GetIns().Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	otherCtx := context.WithValue(ctx, constUtil.AdminServerScopeContextKey, int64(2))
	view, err := AdminWalletLedger(otherCtx, actor, other.Id, 0, 0, "payment", 1, 20)
	if err != nil || view.BalanceCents != 1600 || view.Revision != 2 || view.Total != 1 || len(view.Entries) != 1 || view.Entries[0].Kind != "payment" || view.Entries[0].AmountCents != -900 || view.Entries[0].Label != constUtil.AdminWalletAdjustDebitLabel+debit.Reason {
		t.Fatal("shared account or debit projection", view, err)
	}
	f.restart(t)
	retry, err = AdminWalletAdjust(ctx, actor, id, debit)
	if err != nil || !retry.AlreadyApplied || retry.BalanceCents != 1600 || retry.Revision != 2 || retry.AmountCents != -900 {
		t.Fatal("cold debit replay", retry, err)
	}
	adminWalletAdjustCounts(t, owner.AccountID, 2, 1, 2)
}

func TestAdminWalletAdjustConcurrentReplayAndPayloadConflict(t *testing.T) {
	_, id, ctx, actor, owner := adminWalletAdjustFixture(t)
	req := adminWalletAdjustRequest(constUtil.AdminWalletAdjustCredit, 11, 0)
	var (
		wg      sync.WaitGroup
		results = make(chan *model.AdminWalletAdjustResponse, 8)
		errorsC = make(chan error, 8)
	)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := AdminWalletAdjust(ctx, actor, id, req)
			results <- out
			errorsC <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errorsC)
	for err := range errorsC {
		if err != nil {
			t.Fatal("concurrent request", err)
		}
	}
	var applied int
	for out := range results {
		if out == nil || out.BalanceCents != 1100 || out.Revision != 1 {
			t.Fatal("concurrent receipt", out)
		}
		if !out.AlreadyApplied {
			applied++
		}
	}
	if applied != 1 {
		t.Fatal("new transactions", applied)
	}
	for _, mutate := range []func(*model.AdminWalletAdjustRequest){
		func(r *model.AdminWalletAdjustRequest) { r.Reason += "变更" },
		func(r *model.AdminWalletAdjustRequest) { r.Points++ },
		func(r *model.AdminWalletAdjustRequest) { r.Action = constUtil.AdminWalletAdjustDebit },
		func(r *model.AdminWalletAdjustRequest) { revision := int64(1); r.Revision = &revision },
	} {
		changed := req
		mutate(&changed)
		if _, err := AdminWalletAdjust(ctx, actor, id, changed); !errors.Is(err, constUtil.ErrAdminWalletAdjustReplay) {
			t.Fatal("reused request ID accepted changed payload", changed, err)
		}
	}
	other := owner
	other.Id, other.Uid = 0, "adjust-same-account-"+uuid.NewString()
	if err := db.GetIns().Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := AdminWalletAdjust(ctx, actor, other.Id, req); !errors.Is(err, constUtil.ErrAdminWalletAdjustReplay) {
		t.Fatal("request replay changed owning role", err)
	}
	adminWalletAdjustCounts(t, owner.AccountID, 1, 1, 1)
}

func TestAdminWalletAdjustIndependentAccountsConcurrentFirstCredit(t *testing.T) {
	_, id, ctx, actor, owner := adminWalletAdjustFixture(t)
	registered, err := registerGlobalGameAccount(ctx, "adjust-independent-"+uuid.NewString(), "isolated-terminal-pass", "独立钱包账号")
	if err != nil {
		t.Fatal(err)
	}
	var other db.Profile
	if err = db.GetIns().First(&other, registered.PlayerID).Error; err != nil {
		t.Fatal(err)
	}
	if other.AccountID < 1 || other.AccountID == owner.AccountID {
		t.Fatal("independent account fixture", other.AccountID, owner.AccountID)
	}
	var (
		owners = []db.Profile{owner, other}
		ids    = []int64{id, other.Id}
		before [2]db.AccountWalletBalance
		wg     sync.WaitGroup
		start  = make(chan struct{})
		out    [2]*model.AdminWalletAdjustResponse
		errs   [2]error
	)
	for index, account := range owners {
		balance, readErr := accountWalletBalance(db.GetIns(), account.AccountID, false)
		if readErr != nil {
			t.Fatal(readErr)
		}
		before[index] = *balance
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			req := adminWalletAdjustRequest(constUtil.AdminWalletAdjustCredit, 3, before[index].Revision)
			out[index], errs[index] = AdminWalletAdjust(ctx, actor, ids[index], req)
		}(index)
	}
	close(start)
	wg.Wait()
	for index, account := range owners {
		if errs[index] != nil || out[index] == nil || out[index].AlreadyApplied || out[index].AmountCents != 300 || out[index].BalanceCents != before[index].BalanceCents+300 || out[index].Revision != before[index].Revision+1 {
			t.Fatalf("independent first credit account=%d result=%+v err=%v", account.AccountID, out[index], errs[index])
		}
		adminWalletAdjustCounts(t, account.AccountID, 1, 1, 1)
	}
}

func TestAdminWalletAdjustPermissionProtectionAndScope(t *testing.T) {
	f, id, ctx, actor, owner := adminWalletAdjustFixture(t)
	req := adminWalletAdjustRequest(constUtil.AdminWalletAdjustCredit, 5, 0)
	if _, err := AdminWalletAdjust(ctx, actor, id, req); err != nil {
		t.Fatal(err)
	}
	viewer := adminWalletTestActor(t, f, constUtil.AdminRoleViewer)
	for _, candidate := range []model.AdminWalletAdjustRequest{req, adminWalletAdjustRequest(constUtil.AdminWalletAdjustDebit, 1, 1)} {
		if _, err := AdminWalletAdjust(ctx, viewer, id, candidate); !errors.Is(err, constUtil.ErrAdminPermission) {
			t.Fatal("viewer adjustment/replay", err)
		}
		wrongRealm := context.WithValue(ctx, constUtil.AdminServerScopeContextKey, int64(2))
		if _, err := AdminWalletAdjust(wrongRealm, actor, id, candidate); !errors.Is(err, constUtil.ErrAdminPermission) {
			t.Fatal("cross realm adjustment/replay", err)
		}
	}
	if err := db.AccountsTable(db.GetIns(), "game_accounts").Where("id=?", owner.AccountID).Update("protected", true).Error; err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []model.AdminWalletAdjustRequest{req, adminWalletAdjustRequest(constUtil.AdminWalletAdjustDebit, 1, 1)} {
		if _, err := AdminWalletAdjust(ctx, actor, id, candidate); !errors.Is(err, constUtil.ErrAdminPermission) {
			t.Fatal("protected account adjustment/replay", err)
		}
	}
	noScope := context.WithValue(ctx, constUtil.AdminServerScopeContextKey, int64(0))
	if _, err := AdminWalletAdjust(noScope, actor, id, req); !errors.Is(err, constUtil.ErrAdminInput) {
		t.Fatal("missing explicit scope", err)
	}
	legacy := owner
	legacy.Id, legacy.AccountID, legacy.Uid = 0, 0, "adjust-legacy-"+uuid.NewString()
	if err := db.GetIns().Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := AdminWalletAdjust(ctx, actor, legacy.Id, req); !errors.Is(err, constUtil.ErrAdminWalletAdjustUnlinked) {
		t.Fatal("legacy unlinked account", err)
	}
	adminWalletAdjustCounts(t, owner.AccountID, 1, 1, 1)
}

func TestAdminWalletAdjustOverflowAndAuditAtomicity(t *testing.T) {
	_, id, ctx, actor, owner := adminWalletAdjustFixture(t)
	for _, balance := range []db.AccountWalletBalance{
		{AccountID: owner.AccountID, BalanceCents: math.MaxInt64 - 99, Revision: 0},
		{AccountID: owner.AccountID, BalanceCents: 100, Revision: math.MaxInt64},
	} {
		if err := db.AccountsTable(db.GetIns(), "account_wallet_balances").Save(&balance).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := AdminWalletAdjust(ctx, actor, id, adminWalletAdjustRequest(constUtil.AdminWalletAdjustCredit, 1, balance.Revision)); !errors.Is(err, constUtil.ErrAdminWalletAdjustOverflow) {
			t.Fatal("overflow", err)
		}
	}
	if err := db.AccountsTable(db.GetIns(), "account_wallet_balances").Where("account_id=?", owner.AccountID).Updates(map[string]any{"balance_cents": 0, "revision": 0}).Error; err != nil {
		t.Fatal(err)
	}
	failure := errors.New("isolated audit failure")
	callback := "admin-wallet-adjust-audit-failure"
	if err := db.GetIns().Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "admin_audits" {
			tx.AddError(failure)
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.GetIns().Callback().Create().Remove(callback)
	req := adminWalletAdjustRequest(constUtil.AdminWalletAdjustCredit, 2, 0)
	if _, err := AdminWalletAdjust(ctx, actor, id, req); !errors.Is(err, failure) {
		t.Fatal("audit failure not returned", err)
	}
	adminWalletAdjustCounts(t, owner.AccountID, 0, 0, 0)
	balance, err := accountWalletBalance(db.GetIns(), owner.AccountID, false)
	if err != nil || balance.BalanceCents != 0 || balance.Revision != 0 {
		t.Fatal("audit failure committed balance", balance, err)
	}
	if err = db.GetIns().Callback().Create().Remove(callback); err != nil {
		t.Fatal(err)
	}
	if _, err = AdminWalletAdjust(ctx, actor, id, req); err != nil {
		t.Fatal("retry after atomic rollback", err)
	}
	adminWalletAdjustCounts(t, owner.AccountID, 1, 1, 1)
}
