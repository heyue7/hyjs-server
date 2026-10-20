package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

func TestAdminAuthBootstrapSessionCSRFAndRoles(t *testing.T) {
	adminAuthBootstrapSessionCSRFAndRoles(t, false)
}
func TestAdminAuthBootstrapSessionCSRFAndRolesMySQL(t *testing.T) {
	adminAuthBootstrapSessionCSRFAndRoles(t, true)
}

// adminAuthBootstrapSessionCSRFAndRoles uses a temporary private bootstrap and isolated auth tables.
// It checks the real database mutations, no default password, revocation and stale-role protection.
func adminAuthBootstrapSessionCSRFAndRoles(t *testing.T, mysql bool) {
	_, _, _ = gemFixture(t, mysql)
	ctx := context.Background()
	if err := AdminInitialize(ctx, ""); err != nil {
		t.Fatal(err)
	}
	var n int64
	if err := db.GetIns().Model(&db.AdminUser{}).Count(&n).Error; err != nil || n != 0 {
		t.Fatal("empty bootstrap created default account", err)
	}
	file := filepath.Join(t.TempDir(), "bootstrap.json")
	raw := []byte(`{"username":"owner","password":"initial-test-secret-123"}`)
	if err := os.WriteFile(file, raw, 0644); err != nil {
		t.Fatal(err)
	}
	if err := AdminInitialize(ctx, file); !errors.Is(err, constUtil.ErrAdminConfiguration) {
		t.Fatal("public bootstrap permissions accepted", err)
	}
	if err := os.Chmod(file, 0600); err != nil {
		t.Fatal(err)
	}
	if err := AdminInitialize(ctx, file); err != nil {
		t.Fatal(err)
	}
	var owner db.AdminUser
	if err := db.GetIns().Where("username = ?", "owner").Take(&owner).Error; err != nil {
		t.Fatal(err)
	}
	if owner.Role != constUtil.AdminRoleAdmin || owner.PasswordHash == "initial-test-secret-123" || !strings.HasPrefix(owner.PasswordHash, "$argon2id$") {
		t.Fatal("bootstrap hash or role invalid")
	}
	// Reinitialization ignores changed bootstrap contents once an account exists.
	if err := os.WriteFile(file, []byte(`{"username":"owner","password":"replacement-secret-456"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := AdminInitialize(ctx, file); err != nil {
		t.Fatal(err)
	}
	var existing db.AdminUser
	if err := db.GetIns().First(&existing, owner.ID).Error; err != nil || existing.PasswordHash != owner.PasswordHash {
		t.Fatal("bootstrap overwrote established password", err)
	}
	login, err := AdminLogin(ctx, "owner", "initial-test-secret-123")
	if err != nil {
		t.Fatal(err)
	}
	principal, err := AdminAuthenticate(ctx, login.SessionToken)
	if err != nil || principal.Username != "owner" || principal.Role != constUtil.AdminRoleAdmin {
		t.Fatal("authenticate", err)
	}
	if !AdminCheckCSRF(principal, login.CsrfToken) || AdminCheckCSRF(principal, login.SessionToken) || AdminCheckCSRF(nil, login.CsrfToken) || AdminCheckCSRF(principal, "") {
		t.Fatal("CSRF not session bound")
	}
	var stored db.AdminSession
	if err := db.GetIns().Where("user_id = ?", owner.ID).Take(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.TokenHash == login.SessionToken || stored.CSRFHash == login.CsrfToken || len(stored.TokenHash) != 64 || len(stored.CSRFHash) != 64 || stored.ExpiresAtUnix-time.Now().Unix() > 43200 {
		t.Fatal("plaintext session token or lifetime invalid")
	}
	safe, _ := json.Marshal(login)
	if strings.Contains(string(safe), login.SessionToken) || strings.Contains(string(safe), login.CsrfToken) {
		t.Fatal("login secrets serialized into DTO")
	}
	viewer, err := AdminCreateUser(ctx, principal, "reader", "viewer-test-secret-123", constUtil.AdminRoleViewer)
	if err != nil {
		t.Fatal(err)
	}
	viewLogin, err := AdminLogin(ctx, "reader", "viewer-test-secret-123")
	if err != nil {
		t.Fatal(err)
	}
	viewPrincipal, err := AdminAuthenticate(ctx, viewLogin.SessionToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AdminListUsers(ctx, viewPrincipal); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("viewer listed accounts", err)
	}
	if err := AdminUpdateRole(ctx, principal, owner.ID, constUtil.AdminRoleViewer); !errors.Is(err, constUtil.ErrAdminLastAdministrator) {
		t.Fatal("last admin removed", err)
	}
	if err := AdminUpdateRole(ctx, principal, viewer.ID, constUtil.AdminRoleOperator); err != nil {
		t.Fatal(err)
	}
	if _, err := AdminAuthenticate(ctx, viewLogin.SessionToken); !errors.Is(err, constUtil.ErrAdminSession) {
		t.Fatal("role change retained old session", err)
	}
	rows, err := AdminListUsers(ctx, principal)
	if err != nil || len(rows) != 2 {
		t.Fatal("admin user list", err)
	}
	if err := AdminChangePassword(ctx, principal, viewer.ID, "", "changed-viewer-secret-456"); err != nil {
		t.Fatal(err)
	}
	if _, err := AdminLogin(ctx, "reader", "viewer-test-secret-123"); !errors.Is(err, constUtil.ErrAdminCredentials) {
		t.Fatal("reset retained old password", err)
	}
	next, err := AdminLogin(ctx, "reader", "changed-viewer-secret-456")
	if err != nil {
		t.Fatal(err)
	}
	if err := AdminLogout(ctx, next.SessionToken); err != nil {
		t.Fatal(err)
	}
	if err := AdminLogout(ctx, next.SessionToken); err != nil {
		t.Fatal("logout idempotence", err)
	}
	if _, err := AdminAuthenticate(ctx, next.SessionToken); !errors.Is(err, constUtil.ErrAdminSession) {
		t.Fatal("logged out session revived", err)
	}
	// Account mutations publish only minimal identifiers in the same committed audit log.
	var audit []db.AdminAudit
	if err := db.GetIns().Order("id ASC").Find(&audit).Error; err != nil {
		t.Fatal(err)
	}
	actions := map[string]bool{}
	for _, row := range audit {
		actions[row.Action] = true
		content := row.Actor + " " + row.Target + " " + row.Detail
		for _, secret := range []string{"initial-test-secret-123", "viewer-test-secret-123", "changed-viewer-secret-456", owner.PasswordHash, login.SessionToken, login.CsrfToken, viewLogin.SessionToken, viewLogin.CsrfToken, next.SessionToken, next.CsrfToken} {
			if strings.Contains(content, secret) {
				t.Fatal("credential or token retained in account audit")
			}
		}
	}
	for _, action := range []string{"user.bootstrap", "user.create", "user.role.update", "user.password.change"} {
		if !actions[action] {
			t.Fatal("account mutation audit missing", action)
		}
	}
	// Revoked or expired credentials are rejected by a fresh database read.
	if err := db.GetIns().Model(&db.AdminSession{}).Where("user_id = ?", owner.ID).Update("expires_at_unix", time.Now().Unix()-1).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := AdminAuthenticate(ctx, login.SessionToken); !errors.Is(err, constUtil.ErrAdminSession) {
		t.Fatal("expired session accepted", err)
	}
}

func TestAdminAuthPersistentCooldownAndPasswordChange(t *testing.T) {
	_, _, _ = gemFixture(t, false)
	ctx := context.Background()
	if err := AdminInitialize(ctx, ""); err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("owner-old-password-123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	owner := db.AdminUser{Username: "owner", PasswordHash: string(hash), Role: constUtil.AdminRoleAdmin, Enabled: true}
	if err := db.GetIns().Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 5; i++ {
		_, err := AdminLogin(ctx, "OWNER", "bad-password")
		expected := constUtil.ErrAdminCredentials
		if i == 5 {
			expected = constUtil.ErrAdminLoginCooldown
		}
		if !errors.Is(err, expected) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	if _, err := AdminLogin(ctx, "owner", "owner-old-password-123"); !errors.Is(err, constUtil.ErrAdminLoginCooldown) {
		t.Fatal("correct password bypassed persisted cooldown", err)
	}
	var throttle db.AdminLoginThrottle
	if err := db.GetIns().Where("username = ?", "owner").Take(&throttle).Error; err != nil || throttle.Failures != 5 || throttle.LockedUntilUnix <= time.Now().Unix() {
		t.Fatal("cooldown not persisted", err)
	}
	if err := db.GetIns().Model(&db.AdminLoginThrottle{}).Where("username = ?", "owner").Updates(map[string]any{"locked_until_unix": time.Now().Unix() - 1, "window_started_at_unix": time.Now().Unix() - 1000}).Error; err != nil {
		t.Fatal(err)
	}
	login, err := AdminLogin(ctx, "owner", "owner-old-password-123")
	if err != nil {
		t.Fatal(err)
	}
	principal, err := AdminAuthenticate(ctx, login.SessionToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := AdminChangePassword(ctx, principal, owner.ID, "wrong-old", "owner-new-password-456"); !errors.Is(err, constUtil.ErrAdminCredentials) {
		t.Fatal("self password changed without old password", err)
	}
	if err := AdminChangePassword(ctx, principal, owner.ID, "owner-old-password-123", "owner-new-password-456"); err != nil {
		t.Fatal(err)
	}
	if _, err := AdminAuthenticate(ctx, login.SessionToken); !errors.Is(err, constUtil.ErrAdminSession) {
		t.Fatal("password change retained old session", err)
	}
	if _, err := AdminLogin(ctx, "owner", "owner-new-password-456"); err != nil {
		t.Fatal(err)
	}
	// A caller authenticated before a role change cannot retain administrator powers.
	if err := db.GetIns().Model(&db.AdminUser{}).Where("id = ?", owner.ID).Update("role", constUtil.AdminRoleViewer).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := AdminCreateUser(ctx, principal, "intruder", "intruder-password-789", constUtil.AdminRoleAdmin); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("stale admin principal retained privilege", err)
	}
}
