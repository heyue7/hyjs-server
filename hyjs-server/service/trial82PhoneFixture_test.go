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

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestTrial82PhoneFixture builds only a dedicated, managed, finite local QA
// role. The caller owns the schema, private credentials and service lifetime.
func TestTrial82PhoneFixture(t *testing.T) {
	output := os.Getenv("HYJS_TRIAL82_PHONE_FIXTURE_FILE")
	if output == "" {
		t.Skip("trial82 phone fixture not requested")
	}
	name := os.Getenv("HYJS_P1_MYSQL_DATABASE")
	prefix := "hyjs_test_p1_activity77_phone_82_"
	if !filepath.IsAbs(output) || !strings.HasPrefix(name, prefix) || len(name) <= len(prefix) || len(name) > 64 || !regexp.MustCompile(`^[a-z0-9_]+$`).MatchString(name) {
		t.Fatal("trial82 fixture requires its dedicated managed MySQL schema and absolute private output")
	}
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatal("trial82 private output must not already exist")
	}
	priorOutput := filepath.Join(t.TempDir(), "activity77-private.json")
	defer os.Remove(priorOutput)
	t.Setenv("HYJS_ACTIVITY77_PHONE_FIXTURE_FILE", priorOutput)
	TestActivity77PhoneFixture(t)
	var prior struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		PlayerID  int64  `json:"playerId"`
		AccountID int64  `json:"accountId"`
		ServerID  int64  `json:"serverId"`
		Schema    string `json:"schema"`
		Fixture   string `json:"fixture"`
	}
	raw, err := os.ReadFile(priorOutput)
	if err != nil || json.Unmarshal(raw, &prior) != nil || prior.Schema != name || prior.PlayerID != 2 || prior.AccountID != 2 || prior.ServerID != 1 || prior.Username == "" || prior.Password == "" {
		t.Fatal("inherited local fixture identity mismatch")
	}
	var actual string
	if err = db.GetIns().Raw("SELECT DATABASE()").Scan(&actual).Error; err != nil || actual != name {
		t.Fatal("fixture database identity mismatch", err)
	}
	var profile db.Profile
	if err = db.GetIns().First(&profile, prior.PlayerID).Error; err != nil || profile.AccountID != prior.AccountID || profile.ServerID != 1 {
		t.Fatal("isolated player identity mismatch", err)
	}
	var account db.GameAccount
	if err = db.GetIns().First(&account, prior.AccountID).Error; err != nil || account.Protected || !account.Enabled || account.Username != prior.Username {
		t.Fatal("isolated account identity mismatch", err)
	}
	var user db.AdminUser
	if err = db.GetIns().Where("username=?", "qa77-fixture-admin").First(&user).Error; err != nil || !user.Enabled || user.Role != constUtil.AdminRoleAdmin {
		t.Fatal("isolated administrative fixture mismatch", err)
	}
	actor := &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
	s := &GameService{ServerID: 1}
	ctx := context.WithValue(context.Background(), constUtil.GameServiceContextKey, s)
	ctx = context.WithValue(ctx, constUtil.AdminServerScopeContextKey, s.OriginalServerID())
	if err = s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err = s.LoadP7Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	s.InstallGrowth74Resources()
	if err = s.InstallGGOItems(); err != nil {
		t.Fatal(err)
	}
	if err = s.LoadAdminCatalogue("../../analysis/tables", "../../admin-web/public/media/content-manifest.json"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{s.InitializeServerGameClock, s.InitializeGameplayPools, s.InitializeShop, s.InitializeP1, s.InitializeP4, s.InitializeP6, s.InitCooking, s.InitializeDailyCopy, s.InitializeP7} {
		if err = initialize(ctx); err != nil {
			t.Fatal("trial82 isolated initialization", err)
		}
	}
	maintenance := db.GetIns().Model(&db.ClientUpdatePolicy{}).Where("id=? AND maintenance=?", 1, false).Update("maintenance", true)
	if maintenance.Error != nil || maintenance.RowsAffected != 1 {
		t.Fatal("isolated maintenance transition failed", maintenance.Error)
	}
	if err = s.PublishP7ReRush(ctx); err != nil {
		t.Fatal("isolated Re catalogue publication", err)
	}
	var plan growth74Plan
	if err = json.Unmarshal(growth74Parameters, &plan); err != nil {
		t.Fatal(err)
	}
	trial, ok := plan.Stores["trial"]
	if !ok || len(trial.Products) == 0 {
		t.Fatal("complete trial source missing")
	}
	if err = s.growth74Shop(ctx, actor, "point", trial.Level, trial.Products); err != nil {
		t.Fatal("isolated trial source", err)
	}
	if err = s.ApplyActivity77Shops(ctx, actor); err != nil {
		t.Fatal("isolated 77 star gates", err)
	}
	if err = s.ApplyTrial82Config(ctx, actor); err != nil {
		t.Fatal("isolated trial82 migration", err)
	}
	source, err := s.AdminOrdinaryShopSource(db.GetIns().WithContext(ctx), "point")
	if err != nil {
		t.Fatal(err)
	}
	rules, err := s.validateP4Commerce(source)
	if err != nil || !trialRefresh82Enabled(source, rules) {
		t.Fatal("trial82 consumer rules missing", err)
	}
	var weight, sacredWeight int64
	for _, product := range rules.Products {
		if !product.isEnabled() {
			continue
		}
		group, groupErr := s.trialRefresh82Group(product)
		if groupErr != nil {
			t.Fatal(groupErr)
		}
		if group == 1 {
			weight += product.Weight
		}
		if strings.HasPrefix(product.Rewards, "frag_equip_037,") || strings.HasPrefix(product.Rewards, "frag_equip_038,") || strings.HasPrefix(product.Rewards, "frag_equip_039,") || strings.HasPrefix(product.Rewards, "frag_equip_040,") {
			if group != 1 || product.Cost != "point,6000" || !strings.HasSuffix(product.Rewards, ",5") {
				t.Fatal("sacred trial offer differs")
			}
			sacredWeight += product.Weight
		}
	}
	if sacredWeight != 44 || weight != 220 {
		t.Fatal("isolated trial weights differ", sacredWeight, weight)
	}
	state, err := p7State(db.GetIns().WithContext(ctx), prior.PlayerID, time.Now())
	if err != nil || state.PlayerID != prior.PlayerID || state.RunID != 1 || state.Floor != 1 {
		t.Fatal("isolated initial trial state differs", err)
	}
	if err = db.GetIns().Model(&db.P7RushState{}).Where("player_id=?", prior.PlayerID).Update("record_stars", 306).Error; err != nil {
		t.Fatal(err)
	}
	if err = updateProfileWithTx(ctx, prior.PlayerID, func(_ *gorm.DB, player *pb.Player, game *model.GameState) error {
		if simpleOwned(player, game, "item_013") != 0 {
			return constUtil.ErrAdminConfiguration
		}
		player.Point = proto.Int32(200000)
		_, err := s.GrantFixedReward(player, game, "item_013,2")
		return err
	}); err != nil {
		t.Fatal("isolated test assets", err)
	}
	if err = db.GetIns().First(&profile, prior.PlayerID).Error; err != nil {
		t.Fatal(err)
	}
	player, game, err := decodeProfile(&profile)
	if err != nil || player.GetPoint() != 200000 || simpleOwned(player, game, "item_013") != 2 {
		t.Fatal("isolated test asset verification", err)
	}
	reopened := db.GetIns().Model(&db.ClientUpdatePolicy{}).Where("id=? AND maintenance=?", 1, true).Update("maintenance", false)
	if reopened.Error != nil || reopened.RowsAffected != 1 {
		t.Fatal("isolated service reopen failed", reopened.Error)
	}
	prior.Fixture += "; trial82 QA-only: first floor, 306 synthetic record stars, 200000 points and 2 refresh tickets"
	encoded, err := json.Marshal(prior)
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
