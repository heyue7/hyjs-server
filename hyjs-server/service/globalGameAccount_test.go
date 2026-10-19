package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/config"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestGlobalAccountsLegacyMigrationAndWalletOrders exercises the actual old-credit receipt, migration, password and native recharge path.
func TestGlobalAccountsLegacyMigrationAndWalletOrders(t *testing.T) {
	f, id, ctx := localWalletFixture(t)
	localWalletEnable(t)
	f.id = id
	if _, err := f.s.LocalWalletAdd(ctx, id, "legacy-account-funds", 2500); err != nil {
		t.Fatal(err)
	}
	var old db.Profile
	if err := db.GetIns().First(&old, id).Error; err != nil {
		t.Fatal(err)
	}
	var oldEntries []db.LocalWalletEntry
	if err := db.GetIns().Where("player_id=?", id).Find(&oldEntries).Error; err != nil {
		t.Fatal(err)
	}
	oldRaw, _ := json.Marshal(oldEntries)
	if err := f.s.InitializeAdminGameServers(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeGlobalGameAccounts(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeAccountWallets(ctx); err != nil {
		t.Fatal(err)
	}
	var migrated db.Profile
	if err := db.GetIns().First(&migrated, id).Error; err != nil {
		t.Fatal(err)
	}
	if migrated.AccountID < 1 || old.Uid != migrated.Uid || !bytes.Equal(old.Session, migrated.Session) || !bytes.Equal(old.PasswordSalt, migrated.PasswordSalt) || !bytes.Equal(old.PasswordHash, migrated.PasswordHash) || *old.Player != *migrated.Player || *old.Game != *migrated.Game {
		t.Fatal("migration rewrote legacy credentials/session/save")
	}
	view, err := f.s.LocalWalletAdd(ctx, id, "legacy-account-funds", 2500)
	if err != nil || !view.AlreadyApplied || view.BalanceCents != 2500 || len(view.Entries) != 1 {
		t.Fatal("legacy credit replay repeated funds", view, err)
	}
	if err = f.s.InitializeGlobalGameAccounts(ctx); err != nil {
		t.Fatal(err)
	}
	if err = f.s.InitializeAccountWallets(ctx); err != nil {
		t.Fatal(err)
	}
	if view, err = f.s.LocalWalletSnapshot(ctx, id); err != nil || view.BalanceCents != 2500 || len(view.Entries) != 1 {
		t.Fatal("repeat migration repeated funds", view, err)
	}
	var entries []db.LocalWalletEntry
	if err = db.GetIns().Where("player_id=?", id).Find(&entries).Error; err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(entries)
	if !bytes.Equal(raw, oldRaw) {
		t.Fatal("legacy entry rows changed")
	}
	before, _ := f.profile(t)
	product := p3DemoProviderRequest(t, "djsy.wuyou.andp0006")
	response, err := f.s.generatePayment(ctx, id, product, 781221, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	orderID := response[0].(*pb.SCGenerateOrder).GetOrderId()
	var order db.PaymentOrder
	if err = db.GetIns().First(&order, "id=?", orderID).Error; err != nil {
		t.Fatal(err)
	}
	f.s.P3AutoConfirmDemoPayments = true
	paid, err := f.s.LocalWalletPay(ctx, id, orderID)
	if err != nil || paid.BalanceCents != 2500-order.Cents || len(paid.Entries) != 2 {
		t.Fatal("account debit/native grant", paid, err)
	}
	if err = db.GetIns().First(&order, "id=?", orderID).Error; err != nil {
		t.Fatal(err)
	}
	after, _ := f.profile(t)
	if after.GetDiamond()-before.GetDiamond() != order.Credited || after.GetChargeCount() != before.GetChargeCount()+1 {
		t.Fatal("native recharge grant missing")
	}
	if _, err = f.s.LocalWalletPay(ctx, id, orderID); err != nil {
		t.Fatal("native payment replay", err)
	}
	final, _ := f.profile(t)
	if !proto.Equal(final, after) {
		t.Fatal("native payment replay repeated reward")
	}
	account, err := CurrentGlobalGameAccount(ctx, id, migrated.Session)
	if err != nil || account.Nickname == "" {
		t.Fatal("account profile", err)
	}
	if _, err = UpdateCurrentGlobalGameAccount(ctx, id, migrated.Session, "", "wrong-password", "updated-password"); err == nil {
		t.Fatal("wrong old password accepted")
	}
	updated, err := UpdateCurrentGlobalGameAccount(ctx, id, migrated.Session, "", "isolated-local-demo", "updated-password")
	if err != nil || !updated.PasswordChanged {
		t.Fatal("self password update", updated, err)
	}
	if _, err = AuthorizeAccount(ctx, id, migrated.Session); err != nil {
		t.Fatal("self password change expired current session", err)
	}
	if _, err = LoginAccount(ctx, old.Uid, "isolated-local-demo"); err == nil {
		t.Fatal("old global password still valid")
	}
	login, err := LoginAccount(ctx, old.Uid, "updated-password")
	if err != nil || login.Id != id {
		t.Fatal("updated global password", err)
	}
	var oldOwner db.Profile
	if err = db.GetIns().First(&oldOwner, 1).Error; err != nil {
		t.Fatal(err)
	}
	if GameAccountWalletOwner(db.GetIns(), &oldOwner) {
		t.Fatal("historical protected identity opened wallet")
	}
}

// TestGlobalAccountsThreeSchemaRealms uses independently persisted role DBs and one shared identity schema.
func TestGlobalAccountsThreeSchemaRealms(t *testing.T) {
	shared, secondName := os.Getenv("HYJS_TEST_ACCOUNTS_SCHEMA"), os.Getenv("HYJS_TEST_SECOND_REALM_SCHEMA")
	if shared == "" || secondName == "" {
		t.Skip("requires managed three-schema MySQL lease")
	}
	f, id, ctx := localWalletFixture(t)
	localWalletEnable(t)
	previous := config.AppConfigIns
	current := config.AppConfig{}
	if previous != nil {
		current = *previous
	}
	current.Game.AccountsDatabase = shared
	config.AppConfigIns = &current
	t.Cleanup(func() { config.AppConfigIns = previous })
	if err := f.s.InitializeAdminGameServers(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeGlobalGameAccounts(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeAccountWallets(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.LocalWalletAdd(ctx, id, "realm-one-credit", 4000); err != nil {
		t.Fatal(err)
	}
	var first db.Profile
	if err := db.GetIns().First(&first, id).Error; err != nil {
		t.Fatal(err)
	}
	firstDB := db.GetIns()
	firstSQL, _ := firstDB.DB()
	secondCfg := f.mysql
	secondCfg.Database = secondName
	if err := db.InitGameMySQL(ctx, secondCfg); err != nil {
		t.Fatal(err)
	}
	second := &GameService{ServerID: 2, P3LocalWalletEnabled: true}
	if err := second.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := second.LoadPaymentTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{second.InitializeGameplayPools, second.InitializeP2, second.InitializePayments} {
		if err := initialize(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := second.InitializeP1(ctx); err != nil {
		t.Fatal(err)
	}
	if err := second.InitializeAdminGameServers(ctx); err != nil {
		t.Fatal(err)
	}
	if err := second.InitializeGlobalGameAccounts(ctx); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{second.InitializeLocalWallet, second.InitializeAccountWallets, second.InitializePlayerOpeningCalendar} {
		if err := initialize(ctx); err != nil {
			t.Fatal(err)
		}
	}
	localWalletEnable(t)
	secondContext := context.WithValue(ctx, constUtil.GameServiceContextKey, second)
	role, err := LoginAccount(secondContext, first.Uid, "isolated-local-demo")
	if err != nil {
		t.Fatal("global login in second real runtime", err)
	}
	if role.Id != 1 || role.AccountID != first.AccountID || AdminProfileServerID(*role) != 2 {
		t.Fatal("account did not create independent first role", role.Id, role.AccountID)
	}
	if _, err = CreatePlayer(secondContext, role.Id, "二服角色", "hero_043"); err != nil {
		t.Fatal(err)
	}
	var roleTwo db.Profile
	if err = db.GetIns().First(&roleTwo, role.Id).Error; err != nil {
		t.Fatal(err)
	}
	if !GameAccountWalletOwner(db.GetIns(), &roleTwo) {
		t.Fatal("normal second-realm role one treated as protected")
	}
	var anchor db.PlayerOpeningCalendar
	if err = db.GetIns().First(&anchor, "player_id=?", role.Id).Error; err != nil {
		t.Fatal("normal first role opening calendar blocked", err)
	}
	secondContext = context.WithValue(secondContext, constUtil.LocalWalletSessionContextKey, role.Session)
	view, err := second.LocalWalletSnapshot(secondContext, role.Id)
	if err != nil || view.BalanceCents != 4000 {
		t.Fatal("wallet not shared across role databases", view, err)
	}
	if view, err = second.LocalWalletAdd(secondContext, role.Id, "realm-two-credit", 500); err != nil || view.BalanceCents != 4500 {
		t.Fatal("second realm credit", view, err)
	}
	request := p3DemoProviderRequest(t, "djsy.wuyou.andp0006")
	generated, e := second.generatePayment(secondContext, role.Id, request, 781991, time.Now())
	if e != nil {
		t.Fatal("cross-schema native order", e)
	}
	orderID := generated[0].(*pb.SCGenerateOrder).GetOrderId()
	var paidOrder db.PaymentOrder
	if e = db.GetIns().First(&paidOrder, "id=?", orderID).Error; e != nil {
		t.Fatal(e)
	}
	expectedBalance := int64(4500) - paidOrder.Cents
	second.P3AutoConfirmDemoPayments = true
	if view, e = second.LocalWalletPay(secondContext, role.Id, orderID); e != nil || view.BalanceCents != expectedBalance {
		t.Fatal("cross-schema native payment", view, e)
	}
	if e = db.GetIns().First(&paidOrder, "id=?", orderID).Error; e != nil {
		t.Fatal(e)
	}
	var paidRole db.Profile
	if e = db.GetIns().First(&paidRole, role.Id).Error; e != nil {
		t.Fatal(e)
	}
	player, _, e := decodeProfile(&paidRole)
	if e != nil || int64(player.GetDiamond()) != paidOrder.Credited || player.GetChargeCount() != 1 {
		t.Fatal("second realm native recharge reward", e)
	}
	secondSQL, _ := db.GetIns().DB()
	secondSQL.Close()
	firstSQL.Close()
	f.open(t)
	if view, err = f.s.LocalWalletSnapshot(ctx, id); err != nil || view.BalanceCents != expectedBalance {
		t.Fatal("first realm did not read shared credit", view, err)
	}
	if _, err = UpdateCurrentGlobalGameAccount(ctx, id, first.Session, "", "isolated-local-demo", "cross-realm-updated"); err != nil {
		t.Fatal(err)
	}
	if _, err = AuthorizeAccount(ctx, id, first.Session); err != nil {
		t.Fatal("current first-realm session was not preserved", err)
	}
	firstSQL, _ = db.GetIns().DB()
	firstSQL.Close()
	if err = db.InitGameMySQL(secondContext, secondCfg); err != nil {
		t.Fatal(err)
	}
	if _, err = AuthorizeAccount(secondContext, role.Id, role.Session); err == nil {
		t.Fatal("old other-realm session remained authorized")
	}
	if _, err = LoginAccount(secondContext, first.Uid, "cross-realm-updated"); err != nil {
		t.Fatal("global password was not shared", err)
	}
	// Restore the original owned DB so fixture cleanup never closes another task's connection.
	secondSQL, _ = db.GetIns().DB()
	secondSQL.Close()
	f.open(t)
	var global db.GameAccount
	if err = db.AccountsTable(db.GetIns(), "game_accounts").First(&global, first.AccountID).Error; err != nil {
		t.Fatal(err)
	}
	dto, err := gameAccountProjection(db.GetIns(), global)
	if err != nil || len(dto.Players) != 2 {
		t.Fatal("global role index missing real realm", dto, err)
	}
}

// TestGlobalAccountsAdminCreateRoleFromRegisteredShell preserves the identity/session already issued during registration.
func TestGlobalAccountsAdminCreateRoleFromRegisteredShell(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.InitializeAdminGameServers(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeGlobalGameAccounts(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().AutoMigrate(&db.AdminUser{}, &db.AdminAudit{}); err != nil {
		t.Fatal(err)
	}
	operator := db.AdminUser{Username: "account-shell-operator", PasswordHash: "isolated-no-login", Role: constUtil.AdminRoleOperator, Enabled: true}
	if err := db.GetIns().Create(&operator).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: operator.ID, Username: operator.Username, Role: operator.Role}
	registered, err := RegisterGameAccount(f.ctx, fmt.Sprintf("shell_%x", time.Now().UnixNano()), "isolated-shell-password", "全局昵称")
	if err != nil {
		t.Fatal(err)
	}
	var original db.Profile
	if err = db.GetIns().First(&original, registered.PlayerID).Error; err != nil {
		t.Fatal(err)
	}
	request := model.AdminGamePlayerCreateRequest{AccountID: original.AccountID, ServerID: 1, Name: "新角色", HeroID: "hero_043", Reason: "隔离注册壳创角代表"}
	role, err := f.s.AdminCreateGamePlayer(f.ctx, actor, request)
	if err != nil || role.ID != original.Id || role.AccountID != original.AccountID || role.Nickname != "全局昵称" {
		t.Fatal("registered shell could not receive native starter", role, err)
	}
	var after db.Profile
	if err = db.GetIns().First(&after, original.Id).Error; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original.Session, after.Session) || !bytes.Equal(original.PasswordHash, after.PasswordHash) || !bytes.Equal(original.PasswordSalt, after.PasswordSalt) {
		t.Fatal("admin role creation rewrote registered identity/session")
	}
	if _, err = f.s.AdminCreateGamePlayer(f.ctx, actor, request); err == nil {
		t.Fatal("existing native character was replaced")
	}
}
