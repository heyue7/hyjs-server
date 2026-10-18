package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// adminWalletTestEnvironment reuses the existing isolated fixture and managed MySQL allocation.
func adminWalletTestEnvironment(t *testing.T) {
	t.Helper()
	if raw := os.Getenv("HYJS_MYSQL_TEST_DSN"); raw != "" {
		cfg, err := driver.ParseDSN(raw)
		if err != nil || cfg == nil || cfg.Net != "tcp" || !strings.HasPrefix(cfg.DBName, "hyjs_test_p1_") {
			t.Fatal("wallet tests require a valid managed hyjs_test_p1_ TCP database")
		}
		t.Setenv("HYJS_P1_MYSQL_HOST", cfg.Addr)
		t.Setenv("HYJS_P1_MYSQL_USER", cfg.User)
		t.Setenv("HYJS_P1_MYSQL_PASSWORD", cfg.Passwd)
		t.Setenv("HYJS_P1_MYSQL_DATABASE", cfg.DBName)
	}
}

func adminWalletTestActor(t *testing.T, f *p1Fixture, role string) *model.AdminPrincipal {
	t.Helper()
	if err := AdminInitialize(f.ctx, ""); err != nil {
		t.Fatal(err)
	}
	user := db.AdminUser{Username: "wallet-" + uuid.NewString(), PasswordHash: "unused-fixture", Role: role, Enabled: true}
	if err := db.GetIns().WithContext(f.ctx).Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	return &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
}

// TestAdminWalletReasonsRangesAndFrozenCredit checks seed contracts and immutable real credits.
func TestAdminWalletReasonsRangesAndFrozenCredit(t *testing.T) {
	adminWalletTestEnvironment(t)
	f, id, ctx := localWalletFixture(t)
	localWalletEnable(t)
	expected := map[string][3]int64{
		"change": {1, 5000, 35}, "refund": {1, 100000, 25}, "redpacket": {100, 20000, 24},
		"coffee": {500, 10000, 15}, "secondhand": {1000, 300000, 30}, "cashback": {1, 50000, 15},
		"dinner": {1000, 200000, 20}, "commute": {1000, 100000, 16}, "dailyjob": {5000, 200000, 30},
		"commission": {10000, 2000000, 30}, "salary": {100000, 5000000, 50}, "bonus": {50000, 20000000, 35},
		"annualbonus": {300000, 100000000, 28}, "client": {50000, 100000000, 30}, "store": {10000, 50000000, 30},
		"royalty": {100000, 500000000, 20}, "scholarship": {100000, 2000000, 14}, "birthday": {10000, 2000000, 15},
		"family": {10000, 10000000, 20}, "deposit": {100000, 1000000000, 25}, "dividend": {100000, 10000000000, 25},
		"competition": {10000, 100000000, 18}, "scratch": {500, 10000000, 8}, "lottery": {10000000, 5000000000, 35},
		"property": {10000000, 10000000000, 25}, "business": {100000000, 100000000000, 30},
		"jackpot": {1000000000, 1000000000000, 40}, "cosmic": {100000000000, constUtil.LocalWalletMaxCreditCents, 100},
	}
	var rows []db.LocalWalletReason
	if err := db.GetIns().WithContext(ctx).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(expected) {
		t.Fatal("default reason catalogue does not have 28 situations", len(rows))
	}
	for _, row := range rows {
		want, exists := expected[row.ID]
		if !exists || !row.Enabled || row.Label == "" || [3]int64{row.MinCents, row.MaxCents, row.Weight} != want {
			t.Fatal("reason range or relative weight changed", row.ID)
		}
		// Boundary draws must always select a situation valid for the requested amount.
		// Exact configured weights are checked above; stochastic frequencies are not asserted.
		for _, cents := range []int64{row.MinCents, row.MaxCents} {
			chosen, err := drawWalletReason(db.GetIns().WithContext(ctx), cents)
			if err != nil || chosen == nil || !chosen.Enabled || chosen.Weight < 1 || cents < chosen.MinCents || cents > chosen.MaxCents {
				t.Fatal("draw escaped its inclusive amount range", row.ID, cents, err)
			}
		}
	}
	one, err := drawWalletReason(db.GetIns().WithContext(ctx), 1)
	if err != nil || one == nil || (one.ID != "change" && one.ID != "refund" && one.ID != "cashback") {
		t.Fatal("one-cent credit selected an incompatible story", err)
	}
	view, err := f.s.LocalWalletAdd(ctx, id, "cosmic-frozen", constUtil.LocalWalletMaxCreditCents)
	if err != nil || len(view.Entries) != 1 || view.Entries[0].ReasonID != "cosmic" || view.BalanceCents != constUtil.LocalWalletMaxCreditCents || view.CreditLabel != view.Entries[0].Label || !strings.Contains(view.CreditLabel, "1000000000000.00") {
		t.Fatal("large integer credit or exact amount formatting", view, err)
	}
	entry := view.Entries[0]
	var reason db.LocalWalletReason
	if err := db.GetIns().WithContext(ctx).First(&reason, "id=?", "cosmic").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.GetIns().Save(&reason).Error; err != nil {
			t.Error(err)
		}
	})
	actor := adminWalletTestActor(t, f, constUtil.AdminRoleOperator)
	edit := model.AdminWalletReasonRequest{Label: "以后才使用的新文案 {amount}", MinCents: reason.MinCents, MaxCents: reason.MaxCents, Weight: reason.Weight, Enabled: false, Version: adminVersion(reason)}
	if err := AdminSaveWalletReason(ctx, actor, reason.ID, edit); err != nil {
		t.Fatal(err)
	}
	if err := AdminSaveWalletReason(ctx, actor, reason.ID, edit); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale reason edit was accepted", err)
	}
	if err := f.s.InitializeLocalWallet(ctx); err != nil {
		t.Fatal(err)
	}
	var saved db.LocalWalletReason
	if err := db.GetIns().First(&saved, "id=?", reason.ID).Error; err != nil || saved.Enabled || saved.Label != edit.Label {
		t.Fatal("initialization replaced operator settings", err)
	}
	f.restart(t)
	retry, err := f.s.LocalWalletAdd(ctx, id, "cosmic-frozen", constUtil.LocalWalletMaxCreditCents)
	if err != nil || !retry.AlreadyApplied || retry.Revision != 1 || retry.BalanceCents != view.BalanceCents || retry.CreditLabel != entry.Label || len(retry.Entries) != 1 || !reflect.DeepEqual(retry.Entries[0], entry) {
		t.Fatal("cold retry redrew or rewrote the frozen story", retry, err)
	}
}

// TestAdminWalletLedgerRealWindowPaginationAndIsolation uses actual credit and original recharge transactions.
func TestAdminWalletLedgerRealWindowPaginationAndIsolation(t *testing.T) {
	adminWalletTestEnvironment(t)
	f, id, ctx := localWalletFixture(t)
	localWalletEnable(t)
	base := time.Now().Add(-2 * time.Hour).Truncate(time.Millisecond)
	for index := int64(0); index < 32; index++ {
		if _, err := f.s.localWalletAddAt(ctx, id, fmt.Sprintf("ledger-%d", index), 100000+index, base.Add(time.Duration(index)*time.Millisecond)); err != nil {
			t.Fatal(err)
		}
	}
	request := p3DemoProviderRequest(t, "djsy.wuyou.andp0006")
	paidAt := base.Add(15 * time.Millisecond)
	generated, err := f.s.generatePayment(ctx, id, request, 88001, paidAt)
	if err != nil {
		t.Fatal(err)
	}
	orderID := generated[0].(*pb.SCGenerateOrder).GetOrderId()
	var order db.PaymentOrder
	if err := db.GetIns().First(&order, "id=?", orderID).Error; err != nil {
		t.Fatal(err)
	}
	trusted := context.WithValue(ctx, constUtil.P3TestPaymentContextKey, true)
	trusted = context.WithValue(trusted, constUtil.LocalWalletPaymentContextKey, true)
	if _, err := f.s.confirmTestPaymentAt(trusted, order.ID, "wallet:"+order.ID, order.Cents, constUtil.P3PaymentConfirmed, paidAt); err != nil {
		t.Fatal(err)
	}
	query := model.LocalWalletLedgerRequest{PlayerID: id, StartAt: base.Add(time.Millisecond).UnixMilli(), EndAt: base.Add(31 * time.Millisecond).UnixMilli(), Page: 1, PageSize: 20}
	first, err := f.s.LocalWalletLedger(ctx, query)
	want := model.LocalWalletSummary{StartAt: query.StartAt, EndAt: query.EndAt, IncomeCents: 3000465, ExpenseCents: order.Cents, NetCents: 3000465 - order.Cents, TransactionCount: 31}
	if err != nil || first.Total != 31 || len(first.Entries) != 20 || first.Summary != want || first.BalanceCents != 3200496-order.Cents || first.Entries[0].OrderID != orderID || first.Entries[0].AmountCents != -order.Cents || first.Entries[0].Kind != "payment" {
		t.Fatal("window summary or real original debit projection", first, err)
	}
	query.Page = 2
	second, err := f.s.LocalWalletLedger(ctx, query)
	if err != nil || second.Total != 31 || len(second.Entries) != 11 || second.Summary != want {
		t.Fatal("ledger truncated after the first 20 entries", second, err)
	}
	seen := map[string]bool{}
	for _, page := range []*model.LocalWalletView{first, second} {
		for _, entry := range page.Entries {
			if seen[entry.ID] || entry.CreatedAt < query.StartAt || entry.CreatedAt >= query.EndAt {
				t.Fatal("pagination duplicate or escaped half-open time window", entry.ID)
			}
			seen[entry.ID] = true
		}
	}
	query.Page, query.Kind = 1, "payment"
	payments, err := f.s.LocalWalletLedger(ctx, query)
	if err != nil || payments.Total != 1 || len(payments.Entries) != 1 || payments.Summary != want {
		t.Fatal("payment filter changed full-window summary", payments, err)
	}
	query.Kind, query.PageSize = "credit", 100
	credits, err := f.s.LocalWalletLedger(ctx, query)
	if err != nil || credits.Total != 30 || len(credits.Entries) != 30 || credits.Summary != want {
		t.Fatal("credit filter or exact summary", credits, err)
	}
	// Game calendar offsets cannot change wall-clock accounting windows.
	shifted := context.WithValue(ctx, constUtil.LocalPlayBoundKey, true)
	shifted = context.WithValue(shifted, constUtil.LocalPlayOffsetDaysKey, int64(3650))
	shiftedView, err := f.s.LocalWalletLedger(shifted, query)
	if err != nil || !reflect.DeepEqual(shiftedView, credits) {
		t.Fatal("game date changed accounting", err)
	}
	actor := adminWalletTestActor(t, f, constUtil.AdminRoleViewer)
	admin, err := AdminWalletLedger(ctx, actor, id, query.StartAt, query.EndAt, query.Kind, query.Page, query.PageSize)
	if err != nil || !reflect.DeepEqual(admin, credits) {
		t.Fatal("admin bill diverges from player's real ledger", err)
	}
	other, err := LoginAccount(f.ctx, "demo-ledger-other-"+uuid.NewString(), "isolated-local-demo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreatePlayer(f.ctx, other.Id, "另一个账单", "hero_043"); err != nil {
		t.Fatal(err)
	}
	query.PlayerID = other.Id
	if _, err := f.s.LocalWalletLedger(ctx, query); !errors.Is(err, ErrLocalWalletInvalid) {
		t.Fatal("another account's session read ledger", err)
	}
	var otherOwner db.Profile
	if err := db.GetIns().First(&otherOwner, other.Id).Error; err != nil {
		t.Fatal(err)
	}
	otherCtx := context.WithValue(ctx, constUtil.LocalWalletSessionContextKey, append([]byte(nil), otherOwner.Session...))
	otherView, err := f.s.LocalWalletLedger(otherCtx, query)
	if err != nil || otherView.Total != 0 || otherView.BalanceCents != 0 || otherView.Summary.IncomeCents != 0 || len(otherView.Entries) != 0 {
		t.Fatal("ledger crossed account ownership", otherView, err)
	}
	f.restart(t)
	query.PlayerID = id
	cold, err := f.s.LocalWalletLedger(ctx, query)
	if err != nil || !reflect.DeepEqual(cold, credits) {
		t.Fatal("cold ledger changed immutable history", err)
	}
}
