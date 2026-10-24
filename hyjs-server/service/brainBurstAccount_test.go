package service

import (
	"bytes"
	"testing"

	"hyjs-server/db"
)

func TestBrainBurstRegistrationRealmRoleAndName(t *testing.T) {
	f, _, ctx, _ := terminalFixture(t)
	registered, err := registerGlobalGameAccount(ctx, "brainburst-new-account", "isolated-brain-burst-pass", "账号名称")
	if err != nil || !registered.Registered || registered.PlayerID != 0 {
		t.Fatal("register identity only", registered, err)
	}
	var count int64
	if err = db.GetIns().Model(&db.Profile{}).Where("uid=?", registered.Username).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("registration created a realm role", count, err)
	}
	owner, err := LoginAccount(ctx, registered.Username, "isolated-brain-burst-pass")
	if err != nil || owner.AccountID < 1 || owner.Player != nil {
		t.Fatal("realm login shell", owner, err)
	}
	beforeRole, err := CurrentGlobalGameAccount(ctx, owner.Id, owner.Session)
	if err != nil || len(beforeRole.Players) != 0 {
		t.Fatal("login shell counted as a character", beforeRole, err)
	}
	first, err := CreatePlayer(ctx, owner.Id, "角色一", "hero_043")
	if err != nil {
		t.Fatal(err)
	}
	originalPlayer, originalGame := *first.Player, *first.Game
	second, err := CreatePlayer(ctx, owner.Id, "角色二", "hero_001")
	if err != nil || second.Id != first.Id || *second.Player != originalPlayer || *second.Game != originalGame {
		t.Fatal("duplicate role rewrote first character", err)
	}
	dto, err := UpdateCurrentGlobalGameAccount(ctx, owner.Id, owner.Session, "修改账号名称", "", "")
	if err != nil || dto.Nickname != "修改账号名称" || len(dto.Players) != 1 || dto.Players[0].Name != "角色一" {
		t.Fatal("account name changed realm character", dto, err)
	}
	if err = f.s.InitializeGlobalGameAccounts(ctx); err != nil {
		t.Fatal(err)
	}
	var after db.Profile
	if err = db.GetIns().First(&after, owner.Id).Error; err != nil || *after.Player != originalPlayer || *after.Game != originalGame {
		t.Fatal("identity restart rewrote character", err)
	}
}

func TestBrainBurstLogoutReplayAndNewLoginIsolation(t *testing.T) {
	_, id, ctx, _ := terminalFixture(t)
	var before db.Profile
	if err := db.GetIns().First(&before, id).Error; err != nil {
		t.Fatal(err)
	}
	session := append([]byte(nil), before.Session...)
	result, err := LogoutCurrentGlobalGameAccount(ctx, id, session)
	if err != nil || !result.LoggedOut {
		t.Fatal("logout", result, err)
	}
	if _, err = AuthorizeAccount(ctx, id, session); err == nil {
		t.Fatal("logged-out session still authorizes")
	}
	if result, err = LogoutCurrentGlobalGameAccount(ctx, id, session); err != nil || !result.LoggedOut {
		t.Fatal("lost-response retry", result, err)
	}
	var after db.Profile
	if err = db.GetIns().First(&after, id).Error; err != nil {
		t.Fatal(err)
	}
	if *before.Player != *after.Player || *before.Game != *after.Game || !bytes.Equal(before.PasswordHash, after.PasswordHash) {
		t.Fatal("logout changed persisted character or password")
	}
	// A later login must survive a delayed retry from the released session.
	next, err := LoginAccount(ctx, before.Uid, "isolated-local-demo")
	if err != nil {
		t.Fatal("new login", err)
	}
	if _, err = LogoutCurrentGlobalGameAccount(ctx, id, session); err != nil {
		t.Fatal(err)
	}
	if _, err = AuthorizeAccount(ctx, id, next.Session); err != nil {
		t.Fatal("old logout revoked newer login", err)
	}
}
