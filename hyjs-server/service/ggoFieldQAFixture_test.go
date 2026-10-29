package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/config"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// This opt-in fixture only touches the named managed QA database. Coordinates
// and short patrol segments are unapproved rehearsal inputs, never release data.
func TestGGOIsolatedQAFixture(t *testing.T) {
	output := os.Getenv("HYJS_DESERT_QA_FIXTURE")
	if output == "" {
		t.Skip("isolated desert QA fixture not requested")
	}
	if output != "/private/tmp/hyjs-desert-v4-qa/fixture.json" || os.Getenv("HYJS_MYSQL_DATABASE") != "hyjs_test_p1_desert_v4_qa_28b" || os.Getenv("HYJS_MYSQL_HOST") != "127.0.0.1:3306" || os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" {
		t.Fatal("fixture requires the designated isolated database and private path")
	}
	if _, err := os.Lstat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("fixture output already exists")
	}
	adminPath := filepath.Join(filepath.Dir(output), "admin.json")
	if _, err := os.Lstat(adminPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("fixture admin path already exists")
	}
	if err := config.Init("test"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	mc := config.AppConfigIns.MysqlConfig
	if mc.Host != "127.0.0.1:3306" || mc.DbName != "hyjs_test_p1_desert_v4_qa_28b" || config.AppConfigIns.Game.AccountsDatabase != "" {
		t.Fatal("database config mismatch")
	}
	if err := db.InitExistingGameMySQL(ctx, db.MySQLConfig{Host: mc.Host, User: mc.User, Password: mc.Pass, Database: mc.DbName}); err != nil {
		t.Fatal(err)
	}
	pool, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(3)
	defer pool.Close()
	var actual string
	if err = db.GetIns().Raw("SELECT DATABASE()").Scan(&actual).Error; err != nil || actual != mc.DbName {
		t.Fatal("database identity mismatch")
	}
	var count int64
	if err = db.GetIns().Model(&db.Profile{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("QA fixture requires empty profiles")
	}
	if err = db.GetIns().Model(&db.AdminUser{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("QA fixture requires no preexisting admin")
	}

	randomSecret := func() string {
		var b [24]byte
		if _, e := rand.Read(b[:]); e != nil {
			t.Fatal(e)
		}
		return hex.EncodeToString(b[:])
	}
	writePrivate := func(path string, value any) {
		raw, e := json.Marshal(value)
		if e != nil {
			t.Fatal(e)
		}
		f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			t.Fatal("cannot create private fixture file")
		}
		if _, e = f.Write(raw); e != nil {
			_ = f.Close()
			t.Fatal("cannot write private fixture file")
		}
		if e = f.Close(); e != nil {
			t.Fatal("cannot close private fixture file")
		}
	}
	admin := struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{"qa-desert-admin", randomSecret()}
	writePrivate(adminPath, admin)
	if err = AdminInitialize(ctx, adminPath); err != nil {
		t.Fatal(err)
	}
	var adminRow db.AdminUser
	if err = db.GetIns().Where("username=? AND role=? AND enabled=?", admin.Username, constUtil.AdminRoleAdmin, true).First(&adminRow).Error; err != nil {
		t.Fatal(err)
	}
	s := &GameService{ServerID: 1, GGOIntegrationReady: true}
	if err = s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	s.InstallGrowth74Resources()
	if err = s.InstallGGOItems(); err != nil {
		t.Fatal(err)
	}
	if err = s.LoadAdminCatalogue("../../analysis/tables", "../../admin-web/public/media/content-manifest.json"); err != nil {
		t.Fatal(err)
	}
	if err = s.InitializeServerGameClock(ctx); err != nil {
		t.Fatal(err)
	}
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, s)
	if err = s.InitializeClientUpdates(ctx); err != nil {
		t.Fatal(err)
	}

	var input struct {
		EligibleForRelease bool `json:"eligibleForRelease"`
		Points             []struct {
			ID       string                       `json:"id"`
			Kind     string                       `json:"kind"`
			Position *model.AdminFieldGGOPosition `json:"modelPosition"`
			Stand    *model.AdminFieldGGOPosition `json:"standPosition"`
		} `json:"points"`
	}
	raw, err := os.ReadFile("../../analysis/desert-extraction-v4-unity/point-selection-r5.json")
	if err != nil || json.Unmarshal(raw, &input) != nil || input.EligibleForRelease || len(input.Points) != 59 {
		t.Fatal("QA point input invalid")
	}
	ggo := defaultGGOFieldConfiguration()
	ggo.Runtime.Enabled, ggo.Runtime.MinClientBuild = true, 2026102803
	var safe *model.AdminFieldGGOPosition
	var exits []*model.AdminFieldGGOPosition
	monsterIndex := 0
	for _, item := range input.Points {
		if item.Position == nil || item.Position.X == nil || item.Position.Y == nil || item.Position.Z == nil {
			t.Fatal("missing QA position")
		}
		if item.Kind == "safe" {
			safe = item.Position
			continue
		}
		kind := item.Kind
		if kind == "mob" {
			kind = "monster"
		}
		point := model.AdminFieldGGOPoint{ID: item.ID, Kind: kind, Position: item.Position, StandPosition: item.Stand, Radius: 3}
		if kind == "monster" {
			monsterIndex++
			switch {
			case monsterIndex <= 10:
				point.Level = 35
			case monsterIndex <= 18:
				point.Level = 45
			default:
				point.Level = 60
			}
			if monsterIndex <= 4 {
				end := &model.AdminFieldGGOPosition{X: ggoFloat64(*item.Position.X), Y: ggoFloat64(*item.Position.Y), Z: ggoFloat64(*item.Position.Z + .1)}
				point.Patrol = &model.AdminFieldGGOPatrol{Waypoints: []*model.AdminFieldGGOPosition{item.Position, end}, Speed: 3.2}
			}
			if monsterIndex == 12 {
				ggo.Runtime.EntryPoints = []*model.AdminFieldGGOPosition{item.Position}
			}
		}
		if kind == "exit" {
			exits = append(exits, item.Position)
		}
		ggo.Runtime.Points = append(ggo.Runtime.Points, point)
	}
	if safe == nil || len(exits) != 3 || monsterIndex != 24 {
		t.Fatal("QA map candidate incomplete")
	}
	ggo.Runtime.SafeArea = &model.AdminFieldGGOSafeArea{MinX: *safe.X - 4, MaxX: *safe.X + 4, MinZ: *safe.Z - 4, MaxZ: *safe.Z + 4}
	ggo.Runtime.SafeReturn = safe
	exitIndex := 0
	for i := range ggo.NPCs {
		npc := &ggo.NPCs[i]
		switch npc.Role {
		case "teleport":
			npc.Position = &model.AdminFieldGGOPosition{X: ggoFloat64(*safe.X - 1), Y: ggoFloat64(*safe.Y), Z: ggoFloat64(*safe.Z)}
		case "settlement":
			npc.Position = &model.AdminFieldGGOPosition{X: ggoFloat64(*safe.X + 1), Y: ggoFloat64(*safe.Y), Z: ggoFloat64(*safe.Z)}
		case "extraction":
			npc.Position = exits[exitIndex]
			exitIndex++
		}
	}
	if !model.ValidAdminFieldGGORuntime(ggo) || !s.AdminGGOFieldBossResourceCatalogValid(ggo.Runtime.Boss) {
		t.Fatal("QA runtime configuration invalid")
	}
	var template db.AdminFieldTemplate
	if err = db.GetIns().First(&template, "id=?", "original-field_003").Error; err != nil {
		t.Fatal(err)
	}
	var field model.AdminFieldConfiguration
	if err = json.Unmarshal([]byte(template.Configuration), &field); err != nil {
		t.Fatal(err)
	}
	field.GGO = ggo
	raw, err = json.Marshal(field)
	if err != nil {
		t.Fatal(err)
	}
	template.Configuration, template.Revision, template.UpdatedAt = string(raw), template.Revision+1, time.Now().UnixMilli()
	if err = db.GetIns().Save(&template).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: adminRow.ID, Username: adminRow.Username, Role: adminRow.Role}
	adminCtx := context.WithValue(ctx, constUtil.AdminServerScopeContextKey, int64(1))
	var previous db.AdminFieldDeployment
	version := ""
	if e := db.GetIns().First(&previous, "server_id=? AND scene=?", 1, constUtil.GGOFieldScene).Error; e == nil {
		version = adminVersion(previous)
	} else if !errors.Is(e, gorm.ErrRecordNotFound) {
		t.Fatal(e)
	}
	if _, err = s.AdminDeployField(adminCtx, actor, model.AdminFieldDeployRequest{ServerID: 1, Scene: constUtil.GGOFieldScene, TemplateID: template.ID, TemplateVersion: adminVersion(template), Version: version, Enabled: true}); err != nil {
		t.Fatal("QA field deploy", err)
	}
	if err = db.GetIns().Model(&db.P10ScenePolicy{}).Where("id=?", constUtil.P10ScenePolicyID).Updates(map[string]any{"enabled": true, "line_count": 2, "max_players": 100}).Error; err != nil {
		t.Fatal(err)
	}

	// ID 1 is a protected placeholder and never enters gameplay.
	placeholderUser, placeholderPassword := "qa-desert-placeholder", randomSecret()
	if _, err = RegisterGameAccount(ctx, placeholderUser, placeholderPassword, "QA占位"); err != nil {
		t.Fatal(err)
	}
	placeholder, err := LoginAccount(ctx, placeholderUser, placeholderPassword)
	if err != nil || placeholder.Id != 1 {
		t.Fatal("unexpected protected placeholder identity", err)
	}
	if err = db.GetIns().Model(&db.GameAccount{}).Where("id=?", placeholder.AccountID).Update("protected", true).Error; err != nil {
		t.Fatal(err)
	}
	values, err := ggoChipValues(ggo)
	if err != nil {
		t.Fatal(err)
	}
	var policy db.ClientUpdatePolicy
	if err = db.GetIns().First(&policy, 1).Error; err != nil || policy.Maintenance {
		t.Fatal("QA client policy unavailable", err)
	}
	type playerFixture struct {
		PlayerID   int64  `json:"playerId"`
		SessionHex string `json:"sessionHex"`
		Username   string `json:"username"`
		Password   string `json:"password"`
	}
	players := make([]playerFixture, 0, 2)
	for i := 0; i < 2; i++ {
		username, password := fmt.Sprintf("qa-desert-player-%d", i+1), randomSecret()
		if _, err = RegisterGameAccount(ctx, username, password, fmt.Sprintf("沙域测试%d", i+1)); err != nil {
			t.Fatal(err)
		}
		profile, e := LoginAccount(ctx, username, password)
		if e != nil {
			t.Fatal(e)
		}
		if profile.Id <= 1 || profile.AccountID <= 0 {
			t.Fatal("protected role cannot be fixture")
		}
		if _, e = CreatePlayer(ctx, profile.Id, fmt.Sprintf("沙域测试%d", i+1), "hero_043"); e != nil {
			t.Fatal(e)
		}
		var account db.GameAccount
		if e = db.GetIns().First(&account, profile.AccountID).Error; e != nil || account.Protected || !account.Enabled {
			t.Fatal("fixture account is not eligible")
		}
		bound, e := s.BindLocalPlayContext(ctx, profile)
		if e != nil {
			t.Fatal(e)
		}
		e = updateProfileWithTx(bound, profile.Id, func(_ *gorm.DB, p *pb.Player, g *model.GameState) error {
			p.Level, p.NoviceGuideSchedule = proto.Int32(60), proto.Int32(8)
			if len(g.Warriors) > 0 {
				g.Warriors[0].Level = proto.Int32(60)
			}
			g.Position = &pb.Position{PlayerId: proto.Int64(profile.Id), Scene: proto.String(constUtil.GGOFieldScene), Current: &pb.PointNew{X: proto.Float32(float32(*safe.X)), Y: proto.Float32(float32(*safe.Y)), Z: proto.Float32(float32(*safe.Z))}}
			g.PositionLoading = false
			g.Field = &model.FieldState{Scene: constUtil.GGOFieldScene, Side: &pb.PlayerSide{PlayerId: proto.Int64(profile.Id), State: proto.Int32(constUtil.P8FieldIdle), CurHp: proto.Float64(1), MaxHp: proto.Float64(1), Tiredness: proto.Int32(200), MaxTiredness: proto.Int32(200)}}
			chips := [6]int64{10}
			if i == 1 {
				chips = [6]int64{0, 0, 6}
			}
			g.GGOField = &model.GGOFieldState{EconomyVersion: ggo.Runtime.EconomyVersion, ExchangeValues: values, Chips: chips, PositionRevision: 1}
			return nil
		})
		if e != nil {
			t.Fatal(e)
		}
		// Finite synthetic skill levels let both QA roles interact with all
		// configured tiers; real accounts are never modified by this fixture.
		if e = db.GetIns().Save(&db.GemCraftState{PlayerID: profile.Id, Level: 6}).Error; e != nil {
			t.Fatal(e)
		}
		if e = db.GetIns().Save(&db.CookingCraftState{PlayerID: profile.Id, Level: 6}).Error; e != nil {
			t.Fatal(e)
		}
		if e = db.GetIns().First(profile, profile.Id).Error; e != nil {
			t.Fatal(e)
		}
		now := time.Now().UnixMilli()
		binding := db.ClientUpdateSession{PlayerID: profile.Id, SessionHash: clientUpdateSessionHash(profile.Session), BuildCode: ggo.Runtime.MinClientBuild, StartupID: randomSecret(), LoginAt: now, LastSeenAt: now, MaintenanceEpoch: policy.MaintenanceEpoch}
		if e = db.GetIns().Save(&binding).Error; e != nil {
			t.Fatal(e)
		}
		players = append(players, playerFixture{PlayerID: profile.Id, SessionHex: hex.EncodeToString(profile.Session), Username: username, Password: password})
	}
	if len(players) != 2 || players[0].PlayerID == players[1].PlayerID || strings.TrimSpace(players[0].SessionHex) == "" {
		t.Fatal("QA identities invalid")
	}
	writePrivate(output, map[string]any{"http": "http://127.0.0.1:18861", "tcpHost": "127.0.0.1", "tcpPort": 18862, "schema": mc.DbName, "players": players, "fixture": "r5 candidate and short synthetic patrol paths; QA only, not eligible for release"})
}
