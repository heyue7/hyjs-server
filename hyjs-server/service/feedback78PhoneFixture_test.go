package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"hyjs-server/utils"
)

// TestFeedback78PhoneFixture extends the existing finite 77 UI fixture in a separate managed schema.
// It writes credentials exclusively to the explicitly requested private file and does not start a server.
func TestFeedback78PhoneFixture(t *testing.T) {
	output := os.Getenv("HYJS_FEEDBACK78_PHONE_FIXTURE_FILE")
	if output == "" {
		t.Skip("feedback78 phone fixture not requested")
	}
	name := os.Getenv("HYJS_P1_MYSQL_DATABASE")
	prefix := "hyjs_test_p1_activity77_phone_f78_"
	validName := regexp.MustCompile(`^[a-z0-9_]+$`).MatchString(name)
	if !filepath.IsAbs(output) || !strings.HasPrefix(name, prefix) || len(name) <= len(prefix) || len(name) > 64 || !validName {
		t.Fatal("feedback78 fixture requires an absolute output and its dedicated managed MySQL schema")
	}
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatal("feedback78 private output must not already exist")
	}
	// The 77 fixture owns its transient credentials; this wrapper adds only its requested QA settings.
	priorOutput := filepath.Join(t.TempDir(), "activity77-private.json")
	t.Setenv("HYJS_ACTIVITY77_PHONE_FIXTURE_FILE", priorOutput)
	TestActivity77PhoneFixture(t)
	var previous struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		PlayerID  int64  `json:"playerId"`
		AccountID int64  `json:"accountId"`
		ServerID  int64  `json:"serverId"`
		Schema    string `json:"schema"`
		Fixture   string `json:"fixture"`
	}
	raw, err := os.ReadFile(priorOutput)
	if err != nil || json.Unmarshal(raw, &previous) != nil || previous.Schema != name ||
		previous.PlayerID != 2 || previous.AccountID != 2 || previous.ServerID != 1 || previous.Username == "" || previous.Password == "" {
		t.Fatal("feedback78 inherited account or role identity differs")
	}
	ctx := context.Background()
	var profile db.Profile
	if err := db.GetIns().First(&profile, previous.PlayerID).Error; err != nil || profile.AccountID != previous.AccountID || profile.ServerID != 1 {
		t.Fatal("feedback78 actual role ownership differs", err)
	}
	var account db.GameAccount
	if err := db.GetIns().First(&account, previous.AccountID).Error; err != nil || account.Protected || !account.Enabled || account.Username != previous.Username {
		t.Fatal("feedback78 actual account ownership differs", err)
	}
	password, err := adminRandomToken()
	if err != nil {
		t.Fatal("cannot generate isolated admin credential")
	}
	_, hash, err := utils.NewPasswordCredential(ctx, password)
	if err != nil {
		t.Fatal("cannot prepare isolated admin credential")
	}
	user := db.AdminUser{Username: "qa78-fixture-admin", PasswordHash: string(hash), Role: constUtil.AdminRoleAdmin, Enabled: true}
	if err := db.GetIns().Create(&user).Error; err != nil {
		t.Fatal("cannot create isolated admin")
	}
	login, err := AdminLogin(ctx, user.Username, password)
	if err != nil || login == nil || login.User.ID != user.ID || login.User.Role != constUtil.AdminRoleAdmin {
		t.Fatal("isolated admin login failed", err)
	}
	actor := &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
	s := &GameService{}
	if err := s.InitializeAdminSproutSettings(ctx); err != nil {
		t.Fatal(err)
	}
	current, err := s.AdminSproutSettings(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	days, yes, no := int64(15), true, false
	settings, err := s.AdminSaveSproutSettings(ctx, actor, model.AdminSproutSettingsSaveRequest{
		Enabled: true, Days: &days, NewCharacterEnabled: &yes, ReturnEnabled: &no,
		Version: current.Version, Reason: "隔离原机验证新角色小芽",
	})
	if err != nil || !settings.Enabled || settings.Days == nil || *settings.Days != 15 || settings.NewCharacterEnabled == nil || !*settings.NewCharacterEnabled {
		t.Fatal("isolated sprout settings not saved", err)
	}
	active, err := s.SproutActivePlayers(db.GetIns().WithContext(ctx), []int64{previous.PlayerID}, time.Now().UTC())
	if err != nil || !active[previous.PlayerID] {
		t.Fatal("isolated new role is not sprout eligible", err)
	}
	payload := struct {
		Username      string `json:"username"`
		Password      string `json:"password"`
		PlayerID      int64  `json:"playerId"`
		AccountID     int64  `json:"accountId"`
		ServerID      int64  `json:"serverId"`
		Schema        string `json:"schema"`
		Fixture       string `json:"fixture"`
		AdminUsername string `json:"adminUsername"`
		AdminPassword string `json:"adminPassword"`
	}{previous.Username, previous.Password, previous.PlayerID, previous.AccountID, previous.ServerID, name,
		previous.Fixture + "; new-character sprout enabled for 15 days", user.Username, password}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal("cannot encode private fixture")
	}
	file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("cannot create private fixture output")
	}
	written := false
	defer func() {
		_ = file.Close()
		if !written {
			_ = os.Remove(output)
		}
	}()
	if _, err = file.Write(encoded); err != nil {
		t.Fatal("cannot write private fixture output")
	}
	if err = file.Close(); err != nil {
		t.Fatal("cannot finish private fixture output")
	}
	written = true
}
