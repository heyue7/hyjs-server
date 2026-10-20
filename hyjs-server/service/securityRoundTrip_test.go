package service

import (
	"bytes"
	"context"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/scrypt"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"os"
	"strings"
	"testing"
	"time"
)

// This test uses one owned real MySQL schema for credential migration, retry
// persistence, event aggregation and role revocation, without touching game data.
func TestSecurityRoundTripMySQL(t *testing.T) {
	name := os.Getenv("HYJS_MYSQL_TEST_DATABASE")
	if !strings.HasPrefix(name, "hyjs_test_") {
		t.Skip("owned MySQL schema required")
	}
	ctx := context.Background()
	cfg := db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: name}
	if err := db.InitGameMySQL(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	connection, _ := db.GetIns().DB()
	t.Cleanup(func() { connection.Close() })
	if err := AdminInitialize(ctx, ""); err != nil {
		t.Fatal(err)
	}
	salt := bytes.Repeat([]byte{17}, 16)
	legacy, err := scrypt.Key([]byte("legacy-game-password"), salt, 16384, 8, 1, 32)
	if err != nil {
		t.Fatal(err)
	}
	row := db.Profile{Uid: "legacy-game", Session: bytes.Repeat([]byte{9}, 16), PasswordSalt: salt, PasswordHash: legacy}
	if err := db.GetIns().Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	svc := &GameService{}
	if err := svc.InitializeGlobalGameAccounts(ctx); err != nil {
		t.Fatal(err)
	}
	var before db.GameAccount
	if err := db.GetIns().Where("username=?", row.Uid).First(&before).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := LoginAccount(ctx, row.Uid, "wrong-password"); err == nil {
		t.Fatal("bad password accepted")
	}
	var unchanged db.Profile
	db.GetIns().First(&unchanged, row.Id)
	if !bytes.Equal(unchanged.Session, row.Session) {
		t.Fatal("bad password mutated session")
	}
	logged, err := LoginAccount(ctx, row.Uid, "legacy-game-password")
	if err != nil {
		t.Fatal(err)
	}
	var account db.GameAccount
	db.GetIns().First(&account, before.ID)
	if !bytes.HasPrefix(account.PasswordHash, []byte("$argon2id$")) || bytes.Equal(account.PasswordSalt, salt) || account.AuthRevision != before.AuthRevision || account.Revision != before.Revision || logged.Player != nil || logged.Game != nil {
		t.Fatal("credential upgrade changed ownership/save/version")
	}
	registered, err := RegisterGameAccount(ctx, "new-account", "new-password-secret", "测试用户")
	if err != nil {
		t.Fatal(err)
	}
	var registeredRow db.Profile
	db.GetIns().First(&registeredRow, registered.PlayerID)
	if len(registeredRow.PasswordSalt) != 16 || !bytes.HasPrefix(registeredRow.PasswordHash, []byte("$argon2id$")) {
		t.Fatal("new account credential missing")
	}
	now := time.Now().Truncate(time.Millisecond)
	for i := 0; i < 5; i++ {
		if _, err := RecordGameLoginFailure(ctx, row.Uid, now); err != nil {
			t.Fatal(err)
		}
	}
	if delay, err := GameLoginRetryAfter(ctx, row.Uid, now); err != nil || delay != time.Second {
		t.Fatal("retry not persisted", delay, err)
	}
	if err := ClearGameLoginFailures(ctx, row.Uid); err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("legacy-admin-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	admin := db.AdminUser{Username: "security-owner", PasswordHash: string(hash), Role: constUtil.AdminRoleAdmin, Enabled: true}
	if err := db.GetIns().Create(&admin).Error; err != nil {
		t.Fatal(err)
	}
	login, err := AdminLogin(ctx, admin.Username, "legacy-admin-password")
	if err != nil {
		t.Fatal(err)
	}
	var upgraded db.AdminUser
	db.GetIns().First(&upgraded, admin.ID)
	if !strings.HasPrefix(upgraded.PasswordHash, "$argon2id$") {
		t.Fatal("admin legacy not upgraded")
	}
	principal, err := AdminAuthenticate(ctx, login.SessionToken)
	if err != nil {
		t.Fatal(err)
	}
	event := model.SecurityEventInput{Actor: admin.Username, SourceIP: "198.51.100.7", Action: "admin.login", Outcome: "success", ReasonCode: "authenticated"}
	for i := 0; i < 2; i++ {
		if err := adminSecurityEventRecordAt(ctx, event, now.UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}
	events, err := AdminSecurityEvents(ctx, principal, model.AdminSecurityEventsRequest{Page: 1, PageSize: 25})
	if err != nil || len(events.Items) != 1 || events.Items[0].Occurrences != 2 {
		t.Fatal("events not grouped", err)
	}
	if err := db.GetIns().Model(&admin).Update("role", constUtil.AdminRoleViewer).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := AdminSecurityEvents(ctx, principal, model.AdminSecurityEventsRequest{Page: 1, PageSize: 25}); err == nil {
		t.Fatal("stale admin retained log access")
	}
	connection.Close()
	if err := db.InitGameMySQL(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	connection, _ = db.GetIns().DB()
	if _, err := LoginAccount(ctx, row.Uid, "legacy-game-password"); err != nil {
		t.Fatal("cold credential login", err)
	}
}
