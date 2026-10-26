package service

import (
	"encoding/json"
	"testing"

	"hyjs-server/db"
)

func TestGameAccountTerminal75ReadOnlyRoleIsolation(t *testing.T) {
	roleWalletFixture(t)
	d := db.GetIns()
	roles := []db.GameAccountRole{
		{AccountID: 100, ServerID: 1, PlayerID: 7, Name: "一服角色", Level: 25},
		{AccountID: 100, ServerID: 2, PlayerID: 7, Name: "二服同号", Level: 35},
		{AccountID: 100, ServerID: 3, PlayerID: 9, Name: "未开钱包", Level: 10},
		{AccountID: 100, ServerID: 4, PlayerID: 10, Name: "", Level: 0},
		{AccountID: 101, ServerID: 4, PlayerID: 7, Name: "另一账号", Level: 80},
	}
	if err := d.Create(&roles).Error; err != nil {
		t.Fatal(err)
	}
	balances := []db.RoleWalletBalance{
		{ServerID: 1, PlayerID: 7, AccountID: 100, BalanceCents: 123400, Revision: 2},
		{ServerID: 2, PlayerID: 7, AccountID: 100, BalanceCents: 0, Revision: 4},
		{ServerID: 3, PlayerID: 9, AccountID: 101, BalanceCents: 999999, Revision: 99},
	}
	if err := d.Create(&balances).Error; err != nil {
		t.Fatal(err)
	}
	states := []db.TerminalRoleState{{AccountID: 100, ServerID: 1, PlayerID: 7, SeenRevision: 1}, {AccountID: 100, ServerID: 2, PlayerID: 7, SeenRevision: 4}}
	if err := d.Create(&states).Error; err != nil {
		t.Fatal(err)
	}
	incomes := []db.TerminalRoleIncome{
		{AccountID: 100, ServerID: 1, PlayerID: 7, ID: "old", Revision: 1},
		{AccountID: 100, ServerID: 1, PlayerID: 7, ID: "unread", Revision: 2},
		{AccountID: 100, ServerID: 2, PlayerID: 7, ID: "seen", Revision: 4},
		{AccountID: 101, ServerID: 3, PlayerID: 9, ID: "foreign", Revision: 99},
	}
	if err := d.Create(&incomes).Error; err != nil {
		t.Fatal(err)
	}
	var account db.GameAccount
	if err := d.First(&account, 100).Error; err != nil {
		t.Fatal(err)
	}
	view, err := gameAccountProjection(d, account)
	if err != nil || len(view.Players) != 3 {
		t.Fatal(view, err)
	}
	first, second, missing := view.Players[0], view.Players[1], view.Players[2]
	if first.TerminalBalanceCents == nil || *first.TerminalBalanceCents != 123400 || first.TerminalUnreadCount == nil || *first.TerminalUnreadCount != 1 {
		t.Fatal("first realm actual balance/unread", first)
	}
	if second.TerminalBalanceCents == nil || *second.TerminalBalanceCents != 0 || second.TerminalUnreadCount == nil || *second.TerminalUnreadCount != 0 {
		t.Fatal("second realm real zero was treated as missing", second)
	}
	if missing.TerminalBalanceCents != nil || missing.TerminalUnreadCount != nil {
		t.Fatal("foreign-account or nonexistent wallet exposed", missing)
	}
	raw, err := json.Marshal(missing)
	if err != nil || string(raw) != `{"serverId":3,"playerId":9,"name":"未开钱包","level":10,"terminalBalanceCents":null,"terminalUnreadCount":null}` {
		t.Fatal("nullable wire contract", string(raw), err)
	}
	admin, err := gameAccountProjection(d, account, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range admin.Players {
		if role.TerminalBalanceCents != nil || role.TerminalUnreadCount != nil {
			t.Fatal("admin list performed terminal projection", role)
		}
	}
	var afterBalances []db.RoleWalletBalance
	var afterIncomes []db.TerminalRoleIncome
	var afterStates []db.TerminalRoleState
	if err := d.Order("server_id,player_id").Find(&afterBalances).Error; err != nil {
		t.Fatal(err)
	}
	if err := d.Order("server_id,player_id,id").Find(&afterIncomes).Error; err != nil {
		t.Fatal(err)
	}
	if err := d.Order("server_id,player_id").Find(&afterStates).Error; err != nil {
		t.Fatal(err)
	}
	if len(afterBalances) != len(balances) || len(afterIncomes) != len(incomes) || len(afterStates) != len(states) || afterBalances[0].BalanceCents != 123400 || afterStates[0].SeenRevision != 1 || afterStates[0].DeliveredRevision != 0 {
		t.Fatal("read-only account projection initialized or acknowledged terminal state", afterBalances, afterStates)
	}
}

func TestGameAccountTerminal75UnavailableSchemaIsNull(t *testing.T) {
	roleWalletFixture(t)
	d := db.GetIns()
	role := db.GameAccountRole{AccountID: 100, ServerID: 1, PlayerID: 7, Name: "只有角色", Level: 10}
	if err := d.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	if err := d.Migrator().DropTable(&db.RoleWalletBalance{}); err != nil {
		t.Fatal(err)
	}
	var account db.GameAccount
	if err := d.First(&account, 100).Error; err != nil {
		t.Fatal(err)
	}
	view, err := gameAccountProjection(d, account)
	if err != nil || len(view.Players) != 1 || view.Players[0].TerminalBalanceCents != nil || view.Players[0].TerminalUnreadCount != nil {
		t.Fatal("unread schema became a fake zero balance", view, err)
	}
}
