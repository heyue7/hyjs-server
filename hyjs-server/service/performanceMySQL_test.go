package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// TestPerformanceMySQL uses one owned schema for actual joins, batch queries,
// concurrent map reads, committed publications and a cold connection reopen.
func TestPerformanceMySQL(t *testing.T) {
	name := os.Getenv("HYJS_MYSQL_TEST_DATABASE")
	if !strings.HasPrefix(name, "hyjs_test_") {
		t.Skip("owned MySQL test allocation required")
	}
	ctx := context.Background()
	cfg := db.MySQLConfig{Host: os.Getenv("HYJS_MYSQL_HOST"), User: os.Getenv("HYJS_MYSQL_USER"), Password: os.Getenv("HYJS_MYSQL_PASSWORD"), Database: name}
	if err := db.InitGameMySQL(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	connection, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection.Close() })
	if err := AdminInitialize(ctx, ""); err != nil {
		t.Fatal(err)
	}
	user := db.AdminUser{Username: "performance-mysql-owner", PasswordHash: "session-fixture-only", Role: constUtil.AdminRoleAdmin, Enabled: true}
	if err := db.GetIns().Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("c", 64)
	if err := db.GetIns().Create(&db.AdminSession{TokenHash: adminDigest(token), UserID: user.ID, CSRFHash: adminDigest(strings.Repeat("d", 64)), ExpiresAtUnix: time.Now().Unix() + 3600}).Error; err != nil {
		t.Fatal(err)
	}
	recorder := &adminActivityQueryRecorder{Interface: db.GetIns().Logger}
	db.GetIns().Logger = recorder
	if _, err := adminAuthenticateTwoQueries(ctx, token); err != nil || recorder.countContaining("") != 2 {
		t.Fatal("MySQL previous authentication queries", recorder.countContaining(""), err)
	}
	recorder.reset()
	actor, err := AdminAuthenticate(ctx, token)
	if err != nil || actor.UserID != user.ID || recorder.countContaining("") != 1 {
		t.Fatal("actual MySQL authentication join", err)
	}
	if err := db.GetIns().Model(&user).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := AdminAuthenticate(ctx, token); !errors.Is(err, constUtil.ErrAdminSession) {
		t.Fatal("committed disable was not observed", err)
	}
	if err := db.GetIns().Model(&user).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	svc := &GameService{}
	for _, load := range []func(string) error{svc.LoadTables, svc.LoadP8Tables, svc.LoadP12WorldBossTables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.InitializeAdminFields(ctx); err != nil {
		t.Fatal(err)
	}
	catalog, err := svc.AdminFields(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	var template model.AdminFieldTemplateDTO
	for _, row := range catalog.Items {
		if row.Configuration.Scene == "field_001" {
			template = row
		}
	}
	if template.ID == "" {
		t.Fatal("original forest template missing")
	}
	if _, err := svc.AdminDeployField(ctx, actor, model.AdminFieldDeployRequest{ServerID: 1, Scene: "field_001", TemplateID: template.ID, TemplateVersion: template.Version, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	original, _ := json.Marshal(svc.Tables)
	recorder.reset()
	warm, err := svc.AdminFieldRuntimeTables(ctx, 1)
	if err != nil || recorder.countContaining("") != 1 || recorder.countContaining("information_schema") != 0 {
		t.Fatal(err)
	}
	// Real concurrent readers traverse shared immutable data under the race run.
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 4; n++ {
				rows, e := svc.AdminFieldRuntimeTables(ctx, 1)
				if e != nil || !reflect.DeepEqual(rows, warm) {
					t.Errorf("concurrent committed field snapshot: %v", e)
					return
				}
			}
		}()
	}
	wg.Wait()
	var deployed db.AdminFieldDeployment
	if err := db.GetIns().First(&deployed, "server_id=? AND scene=?", 1, "field_001").Error; err != nil {
		t.Fatal(err)
	}
	// The exact content key must observe external writes even without a revision bump.
	var field model.AdminFieldConfiguration
	if err := json.Unmarshal([]byte(deployed.Configuration), &field); err != nil {
		t.Fatal(err)
	}
	field.MinLevel = 29
	changed, _ := json.Marshal(field)
	where := db.GetIns().Model(&db.AdminFieldDeployment{}).Where("server_id=? AND scene=?", 1, "field_001")
	if err := where.Update("configuration", string(changed)).Error; err != nil {
		t.Fatal(err)
	}
	next, err := svc.AdminFieldRuntimeTables(ctx, 1)
	if err != nil || adminFieldRuntimeTestRow(next, "Field", "FieldId", "field_001")["levelLimit"] != "29" || adminFieldRuntimeTestRow(warm, "Field", "FieldId", "field_001")["levelLimit"] == "29" {
		t.Fatal("publication changed old snapshot or failed to refresh", err)
	}
	if err := where.Update("configuration", "{").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AdminFieldRuntimeTables(ctx, 1); err == nil {
		t.Fatal("invalid committed configuration silently reused old cache")
	}
	if err := where.Update("configuration", string(changed)).Error; err != nil {
		t.Fatal(err)
	}
	other, err := svc.AdminFieldRuntimeTables(ctx, 2)
	if err != nil || !reflect.DeepEqual(other, svc.Tables) {
		t.Fatal("realm snapshots leaked", err)
	}
	after, _ := json.Marshal(svc.Tables)
	if string(after) != string(original) {
		t.Fatal("shared original tables mutated")
	}
	// Batch reads retain the exact old projection and version for every pool.
	if err := db.GetIns().AutoMigrate(&db.GameplayPool{}, &db.GameplayPoolReward{}, &db.P4CommerceRule{}, &db.P4WelfareRule{}, &db.PeriodInstance{}, &db.P4ActivityRegistry{}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 60; i++ {
		id := fmt.Sprintf("perf-pool-%03d", i)
		if err := db.GetIns().Create(&db.GameplayPool{ID: id, Name: id, Source: "performance-fixture"}).Error; err != nil {
			t.Fatal(err)
		}
		if i%3 != 0 {
			if err := db.GetIns().Create(&db.GameplayPoolReward{PoolID: id, ID: 1, ResourceID: "coin", Amount: 1, Weight: 1}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	recorder.reset()
	previous := legacyAdminActivities(t, ctx)
	previousQueries := recorder.countContaining("")
	recorder.reset()
	current, err := AdminActivities(ctx)
	currentQueries := recorder.countContaining("")
	if err != nil || !reflect.DeepEqual(previous, current) || previousQueries != 65 || currentQueries != 6 {
		t.Fatal("MySQL batch projection differs", err)
	}
	t.Logf("actual MySQL SQL counts: authentication 2 -> 1; runtime field read=1 with no metadata query; 60-pool activities %d -> %d", previousQueries, currentQueries)
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.InitGameMySQL(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	connection, _ = db.GetIns().DB()
	cold, err := svc.AdminFieldRuntimeTables(ctx, 1)
	if err != nil || adminFieldRuntimeTestRow(cold, "Field", "FieldId", "field_001")["levelLimit"] != "29" {
		t.Fatal("cold connection reused wrong field state", err)
	}
	if _, err := AdminAuthenticate(ctx, token); err != nil {
		t.Fatal("cold session join", err)
	}
	t.Log("PASS MySQL: session join/revocation; 24 concurrent field readers; next-read content refresh; invalid-config refusal; realm isolation; 60-pool exact projection; cold connection recovery")
}
