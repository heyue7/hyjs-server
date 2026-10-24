package service

import (
	"bytes"
	"gorm.io/gorm"
	"hyjs-server/db"
	"testing"
	"time"
)

func TestGlobalGameAccountSessionLifecycle(t *testing.T) {
	_, id, ctx, _ := terminalFixture(t)
	var before db.Profile
	if e := db.GetIns().First(&before, id).Error; e != nil {
		t.Fatal(e)
	}
	first, e := issueGlobalAccountSession(ctx, before.Uid, "isolated-local-demo")
	if e != nil {
		t.Fatal(e)
	}
	second, e := issueGlobalAccountSession(ctx, before.Uid, "isolated-local-demo")
	if e != nil {
		t.Fatal(e)
	}
	if len(first.AccountSession) != 64 || first.Account.CurrentPlayerID != 0 {
		t.Fatal("bad pre-realm response")
	}
	var after db.Profile
	db.GetIns().First(&after, id)
	if !bytes.Equal(before.Session, after.Session) || *before.Player != *after.Player || *before.Game != *after.Game {
		t.Fatal("account login changed realm state")
	}
	var stored db.GameAccountSession
	if e = db.GetIns().Where("token_hash=?", accountTokenHash(first.AccountSession)).Take(&stored).Error; e != nil || stored.TokenHash == first.AccountSession {
		t.Fatal("token storage", e)
	}
	if _, e = CurrentGlobalGameAccountSession(ctx, first.AccountSession, id, bytes.Repeat([]byte{9}, 16)); e == nil {
		t.Fatal("bad role accepted")
	}
	changed, e := UpdateGlobalGameAccountSession(ctx, first.AccountSession, id, before.Session, "", "isolated-local-demo", "changed-session-password")
	if e != nil || !changed.PasswordChanged {
		t.Fatal("change password", e)
	}
	if _, e = CurrentGlobalGameAccountSession(ctx, second.AccountSession, 0, nil); e == nil {
		t.Fatal("other token survived")
	}
	if _, e = CurrentGlobalGameAccountSession(ctx, first.AccountSession, id, before.Session); e != nil {
		t.Fatal("current token/role lost", e)
	}
	if _, e = LogoutGlobalGameAccountSession(ctx, first.AccountSession, id, bytes.Repeat([]byte{7}, 16)); e == nil {
		t.Fatal("bad logout role accepted")
	}
	if _, e = CurrentGlobalGameAccountSession(ctx, first.AccountSession, 0, nil); e != nil {
		t.Fatal("bad logout revoked token", e)
	}
	if _, e = LogoutGlobalGameAccountSession(ctx, first.AccountSession, id, before.Session); e != nil {
		t.Fatal(e)
	}
	next, e := LoginAccount(ctx, before.Uid, "changed-session-password")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = LogoutGlobalGameAccountSession(ctx, first.AccountSession, id, before.Session); e != nil {
		t.Fatal(e)
	}
	if _, e = AuthorizeAccount(ctx, id, next.Session); e != nil {
		t.Fatal("logout retry revoked newer role", e)
	}
	third, e := issueGlobalAccountSession(ctx, before.Uid, "changed-session-password")
	if e != nil {
		t.Fatal(e)
	}
	db.GetIns().Model(&db.GameAccountSession{}).Where("token_hash=?", accountTokenHash(third.AccountSession)).Update("expires_at", time.Now().Add(-time.Second).UnixMilli())
	if _, e = CurrentGlobalGameAccountSession(ctx, third.AccountSession, 0, nil); e == nil {
		t.Fatal("expired token accepted")
	}
}

func TestGlobalGameAccountSessionNoRoleAndRevocation(t *testing.T) {
	_, id, ctx, _ := terminalFixture(t)
	if _, e := registerGlobalGameAccount(ctx, "account-only-session", "account-only-password", "账号"); e != nil {
		t.Fatal(e)
	}
	first, e := issueGlobalAccountSession(ctx, "account-only-session", "account-only-password")
	if e != nil {
		t.Fatal(e)
	}
	var existing db.Profile
	db.GetIns().First(&existing, id)
	if _, e = CurrentGlobalGameAccountSession(ctx, first.AccountSession, id, existing.Session); e == nil {
		t.Fatal("cross-account role accepted")
	}
	var count int64
	db.GetIns().Model(&db.Profile{}).Where("uid=?", "account-only-session").Count(&count)
	if count != 0 {
		t.Fatal("created role")
	}
	if _, e = issueGlobalAccountSession(ctx, "account-only-session", "wrong-password"); e == nil {
		t.Fatal("wrong password accepted")
	}
	if _, e = issueGlobalAccountSession(ctx, "absent-session-user", "wrong-password"); e == nil {
		t.Fatal("unknown accepted")
	}
	other, e := issueGlobalAccountSession(ctx, "account-only-session", "account-only-password")
	if e != nil {
		t.Fatal(e)
	}
	changed, e := UpdateGlobalGameAccountSession(ctx, first.AccountSession, 0, nil, "新昵称", "", "")
	if e != nil || changed.Nickname != "新昵称" {
		t.Fatal(e)
	}
	if _, e = CurrentGlobalGameAccountSession(ctx, other.AccountSession, 0, nil); e != nil {
		t.Fatal("nickname revoked token", e)
	}
	db.AccountsTable(db.GetIns(), "game_accounts").Where("id=?", first.Account.ID).Update("auth_revision", 99)
	if _, e = CurrentGlobalGameAccountSession(ctx, first.AccountSession, 0, nil); e == nil {
		t.Fatal("admin reset retained token")
	}
}

func TestGlobalGameAccountSessionLoginThrottle(t *testing.T) {
	_, id, ctx, _ := terminalFixture(t)
	if e := InitializeLoginSecurity(ctx); e != nil {
		t.Fatal(e)
	}
	var role db.Profile
	db.GetIns().First(&role, id)
	for i := 0; i < 5; i++ {
		if _, e := LoginGlobalGameAccount(ctx, role.Uid, "wrong-password", "198.51.100.78"); e == nil {
			t.Fatal("wrong password accepted")
		}
	}
	var throttle db.GameLoginThrottle
	if e := db.AccountsTable(db.GetIns(), "game_login_throttles").Where("account_key=?", gameLoginKey(role.Uid)).Take(&throttle).Error; e != nil || throttle.Failures != 5 || throttle.RetryAt <= time.Now().UnixMilli() {
		t.Fatal("failed login throttle", throttle, e)
	}
	if _, e := LoginGlobalGameAccount(ctx, role.Uid, "isolated-local-demo", "198.51.100.78"); e == nil {
		t.Fatal("retry deadline ignored")
	}
	if e := ClearGameLoginFailures(ctx, role.Uid); e != nil {
		t.Fatal(e)
	}
	token, e := LoginGlobalGameAccount(ctx, role.Uid, "isolated-local-demo", "198.51.100.78")
	if e != nil {
		t.Fatal(e)
	}
	db.AccountsTable(db.GetIns(), "game_accounts").Where("id=?", token.Account.ID).Update("enabled", false)
	if _, e = CurrentGlobalGameAccountSession(ctx, token.AccountSession, 0, nil); e == nil {
		t.Fatal("disabled account accepted")
	}
}

func TestGlobalGameAccountSessionSnapshotChangeRejectsIssue(t *testing.T) {
	_, id, ctx, _ := terminalFixture(t)
	var role db.Profile
	db.GetIns().First(&role, id)
	changed := false
	name := "test:account-session-snapshot-revoke"
	d := db.GetIns()
	if e := d.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
		if changed || tx.Statement.Table != "game_accounts" {
			return
		}
		if snapshot, ok := tx.Statement.Dest.(*db.GameAccount); ok && snapshot.ID == role.AccountID {
			changed = true
			if e := d.Model(&db.GameAccount{}).Where("id=?", role.AccountID).UpdateColumn("auth_revision", snapshot.AuthRevision+1).Error; e != nil {
				t.Error(e)
			}
		}
	}); e != nil {
		t.Fatal(e)
	}
	defer d.Callback().Query().Remove(name)
	if _, e := issueGlobalAccountSession(ctx, role.Uid, "isolated-local-demo"); e == nil {
		t.Fatal("snapshot change issued token")
	}
	var count int64
	d.Model(&db.GameAccountSession{}).Where("account_id=?", role.AccountID).Count(&count)
	if !changed || count != 0 {
		t.Fatal("snapshot guard not exercised", changed, count)
	}
}

func TestGlobalGameAccountSessionRevokedLogoutPreservesNewRole(t *testing.T) {
	for _, reason := range []string{"expired", "revision", "disabled"} {
		t.Run(reason, func(t *testing.T) {
			_, id, ctx, _ := terminalFixture(t)
			var original db.Profile
			if e := db.GetIns().First(&original, id).Error; e != nil {
				t.Fatal(e)
			}
			token, e := issueGlobalAccountSession(ctx, original.Uid, "isolated-local-demo")
			if e != nil {
				t.Fatal(e)
			}
			// Model a later real login: it must never be changed by revoked-token logout.
			next, e := LoginAccount(ctx, original.Uid, "isolated-local-demo")
			if e != nil {
				t.Fatal(e)
			}
			switch reason {
			case "expired":
				e = db.AccountsTable(db.GetIns(), "game_account_sessions").Where("token_hash=?", accountTokenHash(token.AccountSession)).Update("expires_at", time.Now().Add(-time.Second).UnixMilli()).Error
			case "revision":
				e = db.AccountsTable(db.GetIns(), "game_accounts").Where("id=?", original.AccountID).UpdateColumn("auth_revision", gorm.Expr("auth_revision+1")).Error
			case "disabled":
				e = db.AccountsTable(db.GetIns(), "game_accounts").Where("id=?", original.AccountID).Update("enabled", false).Error
			}
			if e != nil {
				t.Fatal(e)
			}
			// Supply the actual newer role pair; the invalid global token grants no right to touch it.
			out, e := LogoutGlobalGameAccountSession(ctx, token.AccountSession, id, next.Session)
			if e != nil || !out.LoggedOut {
				t.Fatal("revoked logout", e)
			}
			var count int64
			if e = db.AccountsTable(db.GetIns(), "game_account_sessions").Where("token_hash=?", accountTokenHash(token.AccountSession)).Count(&count).Error; e != nil || count != 0 {
				t.Fatal("token not removed", e, count)
			}
			var after db.Profile
			if e = db.GetIns().First(&after, id).Error; e != nil || !bytes.Equal(after.Session, next.Session) || after.AccountAuthRevision != next.AccountAuthRevision {
				t.Fatal("revoked logout mutated newer role", e)
			}
			if _, e = LogoutGlobalGameAccountSession(ctx, token.AccountSession, id, next.Session); e != nil {
				t.Fatal("retry", e)
			}
		})
	}
}
