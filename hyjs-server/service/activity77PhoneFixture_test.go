package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

func activity77PhoneRollout(t *testing.T, f *p1Fixture, actor *model.AdminPrincipal) {
	t.Helper()
	f.s.InstallGrowth74Resources()
	for _, init := range []func(context.Context) error{f.s.InitializeClientUpdates, f.s.InitializeKingRecruitWorlds, f.s.InitializeReRestaurant, f.s.InitializeAdminShopAvailability} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := InitializeAdminNativeActivitySettings(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Save(&db.ClientUpdatePolicy{ID: 1, Maintenance: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().AutoMigrate(&db.GameAccountRole{}); err != nil {
		t.Fatal(err)
	}
	if err := initializeGGOAccountWeekly(db.GetIns()); err != nil {
		t.Fatal(err)
	}
	var point db.P4CommerceRule
	if err := db.GetIns().Where("kind=?", "point").First(&point).Error; err != nil {
		t.Fatal(err)
	}
	resources := []string{"item_007", "item_013", "item_010", "item_005", "item_009", "item_008", "item_012", "item_006", "item_017", "frag_equip_013", "frag_equip_025", "frag_equip_037", "frag_equip_038", "frag_equip_039", "frag_equip_040"}
	var source p4CommerceRules
	for i, id := range resources {
		source.Products = append(source.Products, p4BaseProduct(fmt.Sprintf("fixture77_%d", i), id+",1", "point,150", int32(i+1), 1))
	}
	point.Rules = adminJSON(source)
	if err := db.GetIns().Save(&point).Error; err != nil {
		t.Fatal(err)
	}
	zone := time.FixedZone("UTC+8", 8*3600)
	now := time.Now().In(zone)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, zone)
	if err := db.GetIns().Model(&db.AdminGameServer{}).Where("id=?", 1).Update("open_at", start.UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	pairs, err := activity77PeriodPairs()
	if err != nil {
		t.Fatal(err)
	}
	calendar, err := f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil {
		t.Fatal(err)
	}
	items := calendar.Items
	for i, pair := range pairs {
		def, e := f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: pair.old.Name, Form: pair.old, DefaultDays: 7, Enabled: true})
		if e != nil {
			t.Fatal(e)
		}
		at := start
		end := start.AddDate(0, 0, 1)
		if i == 3 {
			at = start.AddDate(0, 0, 2)
			end = at.AddDate(0, 0, 7)
		}
		item := model.AdminActivityOccurrenceDTO{TemplateID: def.ID, TemplateVersion: def.Version, Timing: "calendar", StartAt: at.UnixMilli(), EndAt: end.UnixMilli(), ClaimEndAt: end.UnixMilli(), Enabled: true}
		if i == 3 {
			item.Timing = "character-day"
			item.StartDay = 1
			item.DurationDays = 7
			item.StartAt = start.UnixMilli()
			item.EndAt = start.AddDate(0, 0, 7).UnixMilli()
			item.ClaimEndAt = item.EndAt
		}
		items = append(items, item)
	}
	var pool db.GameplayPool
	if err = db.GetIns().First(&pool, "id=?", "recruit_3").Error; err != nil {
		t.Fatal(err)
	}
	var prizes []db.GameplayPoolReward
	if err = db.GetIns().Where("pool_id=?", "recruit_3").Order("id").Find(&prizes).Error; err != nil {
		t.Fatal(err)
	}
	oldForm, err := f.s.adminKingRecruitActivityForm(db.GetIns(), pool, prizes)
	if err != nil {
		t.Fatal(err)
	}
	oldForm.Name = "王者招募"
	oldForm.Source = "成长投放V12 · 原SAO/ALO/GGO轮转"
	oldForm.Alias = "成长投放V12"
	def, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: oldForm.Name, Form: oldForm, DefaultDays: 183, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	items = append(items, model.AdminActivityOccurrenceDTO{TemplateID: def.ID, TemplateVersion: def.Version, Timing: "calendar", StartAt: start.UnixMilli(), EndAt: start.AddDate(0, 0, 183).UnixMilli(), ClaimEndAt: start.AddDate(0, 0, 183).UnixMilli(), Enabled: true})
	calendar, err = f.s.AdminSaveActivitySchedule(f.ctx, actor, 1, model.AdminActivityScheduleSaveRequest{Version: calendar.Version, Items: items})
	if err != nil {
		t.Fatal(err)
	}
	var oldKey string
	for _, item := range calendar.Items {
		if item.TemplateID == def.ID {
			_, oldKey, _ = strings.Cut(item.ConsumerID, ":")
		}
	}
	if oldKey == "" {
		t.Fatal("old pool absent")
	}
	if err = db.GetIns().Model(&db.KingRecruitActivitySnapshot{}).Where("pool_id=?", oldKey).Update("frozen", true).Error; err != nil {
		t.Fatal(err)
	}
	session := []byte("activity77-test1")
	if err = db.GetIns().Model(&db.Profile{}).Where("id=?", f.id).Update("session", session).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Save(&db.KingRecruitSelection{PlayerID: f.id, PoolID: oldKey}).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Save(&db.KingRecruitState{PlayerID: f.id, Day: p1Day(now), Draws: 9, Luck: 70}).Error; err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error { v := int32(70); g.KingRecruitSharedLuck = &v; return nil }); err != nil {
		t.Fatal(err)
	}
	if err = f.s.ApplyActivity77(f.ctx); err != nil {
		t.Fatal(err)
	}
}

func TestActivity77PhoneFixture(t *testing.T) {
	output := os.Getenv("HYJS_ACTIVITY77_PHONE_FIXTURE_FILE")
	if output == "" {
		t.Skip("phone fixture not requested")
	}
	name := os.Getenv("HYJS_P1_MYSQL_DATABASE")
	if !strings.HasPrefix(name, "hyjs_test_p1_activity77_phone_") || !filepath.IsAbs(output) {
		t.Fatal("fixture requires an absolute output path and the dedicated isolated database prefix")
	}
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatal("fixture output must not already exist")
	}
	f := &p1Fixture{ctx: context.Background(), mysql: db.MySQLConfig{Host: os.Getenv("HYJS_P1_MYSQL_HOST"), User: os.Getenv("HYJS_P1_MYSQL_USER"), Password: os.Getenv("HYJS_P1_MYSQL_PASSWORD"), Database: name}}
	f.open(t)
	t.Cleanup(func() {
		sqlDB, _ := db.GetIns().DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})
	var actual string
	if err := db.GetIns().Raw("SELECT DATABASE()").Scan(&actual).Error; err != nil || actual != name {
		t.Fatal("database identity mismatch")
	}
	if db.AccountsTableName("game_accounts") != "game_accounts" {
		t.Fatal("shared account schema is prohibited for this fixture")
	}
	var existing int64
	if err := db.GetIns().Model(&db.Profile{}).Count(&existing).Error; err != nil || existing != 0 {
		t.Fatal("fixture requires an empty profile table")
	}
	if err := db.GetIns().AutoMigrate(&db.P7RushState{}); err != nil {
		t.Fatal(err)
	}
	f.s = &GameService{}
	if err := f.s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	f.s.InstallGrowth74Resources()
	if err := f.s.InstallGGOItems(); err != nil {
		t.Fatal(err)
	}
	if err := f.s.LoadAdminCatalogue("../../analysis/tables", "../../admin-web/public/media/content-manifest.json"); err != nil {
		t.Fatal(err)
	}
	for _, init := range []func(context.Context) error{f.s.InitializeGameplayPools, f.s.InitializeShop, f.s.InitializeP1, f.s.InitializeP4, f.s.InitializeGGO, AdminInitializeFixture77, InitializeAdminOperations, InitializeAdminActivityModes, f.s.InitializePlayerOpeningCalendar, f.s.InitializeAdminGameServers, f.s.InitializeAdminActivitySchedules, f.s.InitializeServerGameClock, f.s.InitializeLocalPlayActivities, f.s.InitializeP2} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	// The isolated rehearsal explicitly opens the native point and score stores.
	if err := db.GetIns().Model(&db.P4CommerceRule{}).Where("kind IN ?", []string{"point", "score"}).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	user := db.AdminUser{Username: "qa77-fixture-admin", PasswordHash: "unused-isolated-fixture", Role: constUtil.AdminRoleAdmin, Enabled: true}
	if err := db.GetIns().Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
	placeholder, err := LoginAccount(f.ctx, "qa77-protected-placeholder", "isolated-placeholder-password")
	if err != nil {
		t.Fatal(err)
	}
	if placeholder.Id != 1 {
		t.Fatal("unexpected protected identity")
	}
	username := "king-" + p6Digest([]byte(t.Name()))[:24]
	password := "isolated-password"
	account, err := LoginAccount(f.ctx, username, password)
	if err != nil {
		t.Fatal(err)
	}
	if account.Id <= 1 {
		t.Fatal("protected identity rejected")
	}
	if _, err = CreatePlayer(f.ctx, account.Id, "王者原入口", "hero_043"); err != nil {
		t.Fatal(err)
	}
	f.id = account.Id
	activity77PhoneRollout(t, f, actor)
	if err = f.s.InitializeGlobalGameAccounts(f.ctx); err != nil {
		t.Fatal(err)
	}
	var profile db.Profile
	if err = db.GetIns().First(&profile, f.id).Error; err != nil {
		t.Fatal(err)
	}
	var global db.GameAccount
	if err = db.GetIns().First(&global, profile.AccountID).Error; err != nil {
		t.Fatal(err)
	}
	if profile.Id <= 1 || global.Protected || !global.Enabled || global.Username != username || global.ID <= 0 {
		t.Fatal("fixture account ownership invalid")
	}
	bound := serverClockBindTest(t, f, f.id)
	if err = updateProfileWithTx(bound, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		p.Vip = proto.Int32(10)
		p.NoviceGuideSchedule = proto.Int32(8)
		_, e := f.s.GrantFixedReward(p, g, "item_9710,1000000;score,100000;diamond,100000")
		return e
	}); err != nil {
		t.Fatal(err)
	}
	if err = f.s.PrepareAdminCharacterActivitySchedules(bound, f.id); err != nil {
		t.Fatal(err)
	}
	if err = updateProfileWithTx(bound, f.id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
		return f.s.RecordP1Event(tx, f.id, "qa77-ui-wanted-fixture", constUtil.ReActivityWantedAttemptKind, 40, LocalPlayNow(bound))
	}); err != nil {
		t.Fatal(err)
	}
	var progressRows []db.PeriodProgress
	if err = db.GetIns().Where("player_id=?", f.id).Find(&progressRows).Error; err != nil {
		t.Fatal(err)
	}
	qualified := 0
	for _, progress := range progressRows {
		var cfg db.PeriodInstance
		if err = db.GetIns().First(&cfg, "id=?", progress.InstanceID).Error; err != nil {
			t.Fatal(err)
		}
		if cfg.Kind == constUtil.ReActivityWantedAttemptKind && progress.Value >= 40 {
			qualified++
		}
	}
	if qualified != 2 {
		t.Fatalf("wanted fixture requires two qualified activities, got %d", qualified)
	}
	if err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		var role db.Profile
		if e := tx.First(&role, f.id).Error; e != nil {
			return e
		}
		return syncGlobalAccountRole(tx, &role)
	}); err != nil {
		t.Fatal(err)
	}
	var plan growth74Plan
	if err = json.Unmarshal(growth74Parameters, &plan); err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(f.ctx, constUtil.AdminServerScopeContextKey, f.s.OriginalServerID())
	if err = f.s.growth74Shop(ctx, actor, "ggo", 1, plan.CreditShop.Products); err != nil {
		t.Fatal(err)
	}
	shop, err := f.s.AdminShop(ctx, "ggo")
	if err != nil {
		t.Fatal(err)
	}
	if len(shop.Items) < 6 || shop.Settings.Refresh == nil || !shop.Settings.Refresh.Enabled {
		t.Fatal("ggo catalogue or refresh missing")
	}
	var ggo db.P4CommerceRule
	if err = db.GetIns().First(&ggo, "id=?", shop.Settings.PeriodID).Error; err != nil {
		t.Fatal(err)
	}
	var rules p4CommerceRules
	if json.Unmarshal(ggo.Rules, &rules) != nil {
		t.Fatal("ggo rules invalid")
	}
	var quote ggoQuoteState
	if err = ggoDrawQuoteSlots(rules, &quote); err != nil || len(quote.Slots) != 6 {
		t.Fatal("ggo six-slot quote invalid", err)
	}
	weekly := 0
	for _, item := range shop.Items {
		if item.Enabled && item.AccountWeeklyLimit > 0 {
			weekly++
		}
	}
	if weekly == 0 {
		t.Fatal("ggo weekly limits missing")
	}
	// The isolated fixture is intended for direct phone login after the repair.
	if err = db.GetIns().Model(&db.ClientUpdatePolicy{}).Where("id=? AND maintenance=?", 1, true).Update("maintenance", false).Error; err != nil {
		t.Fatal(err)
	}
	payload := struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		PlayerID  int64  `json:"playerId"`
		AccountID int64  `json:"accountId"`
		ServerID  int64  `json:"serverId"`
		Schema    string `json:"schema"`
		Fixture   string `json:"fixture"`
	}{username, password, f.id, global.ID, f.s.OriginalServerID(), name, "qa77-ui-wanted-fixture: 40 synthetic wanted attack events"}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("cannot create private fixture file")
	}
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(output)
		}
	}()
	if _, err = file.Write(raw); err != nil {
		t.Fatal("cannot write private fixture file")
	}
	if err = file.Close(); err != nil {
		t.Fatal("cannot finish private fixture file")
	}
	ok = true
}

func AdminInitializeFixture77(ctx context.Context) error { return AdminInitialize(ctx, "") }
