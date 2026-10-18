package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// TestGameAccountRegisterStrictDuplicateConcurrentAndOriginalLogin preserves the actual original account/session contract.
func TestGameAccountRegisterStrictDuplicateConcurrentAndOriginalLogin(t *testing.T) {
	adminWalletTestEnvironment(t)
	f := p1Setup(t)
	username := "reg-" + uuid.NewString()[:20]
	password := "registration-original-password"
	var wg sync.WaitGroup
	results := make(chan *model.GameAccountRegisterResponse, 4)
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := RegisterGameAccount(f.ctx, username, password)
			results <- out
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	successes, duplicates := 0, 0
	for err := range errs {
		if err == nil {
			successes++
		} else if errors.Is(err, constUtil.ErrGameAccountExists) {
			duplicates++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || duplicates != 3 {
		t.Fatal("concurrent registration did not strictly insert exactly once", successes, duplicates)
	}
	var registered *model.GameAccountRegisterResponse
	for result := range results {
		if result != nil {
			registered = result
		}
	}
	if registered == nil || !registered.Registered || registered.PlayerID <= 1 || registered.Username != username {
		t.Fatal("new original account response", registered)
	}
	var original db.Profile
	if err := db.GetIns().First(&original, registered.PlayerID).Error; err != nil {
		t.Fatal(err)
	}
	if original.Player != nil || original.Game != nil || len(original.PasswordSalt) != 16 || len(original.PasswordHash) != 32 || len(original.Session) != 16 || bytes.Contains(original.PasswordHash, []byte(password)) {
		t.Fatal("registration bypassed original new-player boundary or password storage")
	}
	if _, err := RegisterGameAccount(f.ctx, username, password); !errors.Is(err, constUtil.ErrGameAccountExists) {
		t.Fatal("duplicate registration silently logged in", err)
	}
	if _, err := LoginAccount(f.ctx, username, "wrong-existing-password"); err == nil {
		t.Fatal("original login accepted a wrong registered password")
	}
	var unchanged db.Profile
	if err := db.GetIns().First(&unchanged, original.Id).Error; err != nil || !bytes.Equal(unchanged.Session, original.Session) || !bytes.Equal(unchanged.PasswordHash, original.PasswordHash) {
		t.Fatal("rejected registration/login changed original credentials", err)
	}
	login, err := LoginAccount(f.ctx, username, password)
	if err != nil || login.Id != original.Id || bytes.Equal(login.Session, original.Session) {
		t.Fatal("registered account cannot use original login/session rotation", err)
	}
	if _, err := CreatePlayer(f.ctx, login.Id, "注册原新手", "hero_043"); err != nil {
		t.Fatal("registered account cannot use original starter flow", err)
	}
	f.restart(t)
	cold, err := LoginAccount(f.ctx, username, password)
	if err != nil || cold.Id != original.Id || cold.Player == nil {
		t.Fatal("registered original account cold login", err)
	}
	if count := p1Count(t, &db.Profile{}, "uid=?", username); count != 1 {
		t.Fatal("registration created duplicate profiles", count)
	}
	// Verify the bounded per-client throttle without random timing or a network service.
	client := "fixture-" + uuid.NewString()
	now := time.Now()
	for i := 0; i < constUtil.GameAccountRegisterPerWindow; i++ {
		if !GameAccountRegisterAllowed(client, now) {
			t.Fatal("registration throttled before configured budget")
		}
	}
	if GameAccountRegisterAllowed(client, now) {
		t.Fatal("registration exceeded per-client budget")
	}
	if !GameAccountRegisterAllowed(client, now.Add(time.Minute)) {
		t.Fatal("registration throttle failed to recover after real minute")
	}
}

// TestAdminGamePlayerCreateOriginalStarterRBACAuditAndDuplicate protects the original player creation transaction.
func TestAdminGamePlayerCreateOriginalStarterRBACAuditAndDuplicate(t *testing.T) {
	adminWalletTestEnvironment(t)
	f := p1Setup(t)
	operator := adminWalletTestActor(t, f, constUtil.AdminRoleOperator)
	viewer := adminWalletTestActor(t, f, constUtil.AdminRoleViewer)
	username := "ops-" + uuid.NewString()[:20]
	request := model.AdminGamePlayerCreateRequest{Username: username, Password: "original-starter-password", Name: "后台新用户", HeroID: "hero_043", Reason: "隔离账号创建代表"}
	if _, err := f.s.AdminCreateGamePlayer(f.ctx, viewer, request); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal("viewer created game account", err)
	}
	if count := p1Count(t, &db.Profile{}, "uid=?", username); count != 0 {
		t.Fatal("denied creation left an account")
	}
	out, err := f.s.AdminCreateGamePlayer(f.ctx, operator, request)
	if err != nil || out.ID <= 1 || out.Name != request.Name || out.Level != 1 || out.Vip != 0 || out.Chapter != "1" || out.Mission != "1" || out.Balances["coin"] != 0 || out.Balances["diamond"] != 0 || out.Counts["characters"] != 1 {
		t.Fatal("admin did not create exactly the original starter", out, err)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{request.Password, "passwordHash", "passwordSalt", "sessionHex", "sessionKey", "\"uid\""} {
		if strings.Contains(string(encoded), secret) {
			t.Fatal("account creation DTO leaked credentials", secret)
		}
	}
	var created db.Profile
	if err := db.GetIns().First(&created, out.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.AdminCreateGamePlayer(f.ctx, operator, request); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("admin duplicate overwrote or logged into existing profile", err)
	}
	var unchanged db.Profile
	if err := db.GetIns().First(&unchanged, out.ID).Error; err != nil || !bytes.Equal(unchanged.Session, created.Session) || *unchanged.Player != *created.Player || *unchanged.Game != *created.Game {
		t.Fatal("duplicate admin creation changed existing progress/session", err)
	}
	var audit []db.AdminAudit
	if err := db.GetIns().Where("action=? AND target=?", "player.create", out.ID).Find(&audit).Error; err != nil || len(audit) != 1 || strings.Contains(audit[0].Detail, request.Password) || !strings.Contains(audit[0].Detail, request.Username) {
		t.Fatal("creation audit absent, duplicated or secret-bearing", err)
	}
	login, err := LoginAccount(f.ctx, request.Username, request.Password)
	if err != nil || login.Id != out.ID {
		t.Fatal("admin-created account cannot use original game login", err)
	}
	// An already registered account without a character is also a duplicate, not an invitation to fill it silently.
	halfUsername := "half-" + uuid.NewString()[:20]
	half, err := RegisterGameAccount(f.ctx, halfUsername, "half-registered-password")
	if err != nil {
		t.Fatal(err)
	}
	halfRequest := request
	halfRequest.Username = halfUsername
	if _, err := f.s.AdminCreateGamePlayer(f.ctx, operator, halfRequest); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("admin creation adopted another existing account", err)
	}
	var halfProfile db.Profile
	if err := db.GetIns().First(&halfProfile, half.PlayerID).Error; err != nil || halfProfile.Player != nil {
		t.Fatal("existing uncreated account was modified", err)
	}
}
