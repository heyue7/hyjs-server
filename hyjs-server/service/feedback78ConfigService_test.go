package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type feedback78Fixture struct {
	s       *GameService
	ctx     context.Context
	actor   *model.AdminPrincipal
	periods []db.PeriodInstance
	profile db.Profile
	vip     db.VIPShopProduct
}

func feedback78FixtureIDs() ([]string, []string, []string) {
	periods := []string{"totalLogin_p1_admin_schedule_foreign"}
	occurrences := []string{"occ_foreign"}
	sources := []string{"activity:unrelated-login", "unrelated"}
	for i := 0; i < constUtil.Feedback78ScheduleCount; i++ {
		id := fmt.Sprintf("totalLogin_p1_admin_schedule_feedback%02d", i)
		periods = append(periods, id)
		occurrences = append(occurrences, fmt.Sprintf("occ_feedback%02d", i))
		sources = append(sources, "activity:"+id)
	}
	return periods, occurrences, sources
}

func feedback78CleanupMySQL(t *testing.T) {
	t.Helper()
	periods, occurrences, sources := feedback78FixtureIDs()
	targets := []struct {
		table any
		where string
		args  []any
	}{
		{&db.RewardDelivery{}, "player_id IN ? AND source_key IN ?", []any{[]int64{1, 42, 43, 44, 99}, sources}},
		{&db.MembershipClaim{}, "player_id=? AND kind=?", []any{42, "vip-shop:" + constUtil.P3VIP10GrowthProduct}},
		{&db.PeriodProgress{}, "instance_id IN ? AND player_id=?", []any{periods, 42}},
		{&db.AdminActivityOccurrence{}, "id IN ?", []any{occurrences}},
		{&db.PeriodInstance{}, "id IN ?", []any{periods}},
		{&db.AdminActivityDefinition{}, "id=?", []any{constUtil.Feedback78TemplateID}},
		{&db.VIPShopProduct{}, "id=?", []any{constUtil.P3VIP10GrowthProduct}},
		{&db.AdminActivityScheduleRevision{}, "server_id=?", []any{1}},
		{&db.ClientUpdatePolicy{}, "id=?", []any{1}},
		{&db.AdminAudit{}, "actor=? AND action=?", []any{"feedback78-isolated", constUtil.Feedback78AuditAction}},
		{&db.AdminUser{}, "username=?", []any{"feedback78-isolated"}},
		{&db.Profile{}, "id=? AND uid=?", []any{42, "feedback78-player"}},
	}
	for _, target := range targets {
		if err := db.GetIns().Where(target.where, target.args...).Delete(target.table).Error; err != nil {
			t.Errorf("feedback78 fixture cleanup failed: %v", err)
		}
	}
}

func feedback78Setup(t *testing.T) *feedback78Fixture {
	t.Helper()
	ctx := context.Background()
	name := os.Getenv("HYJS_P1_MYSQL_DATABASE")
	if name == "" {
		if err := db.InitGame(ctx, filepath.Join(t.TempDir(), "feedback78.db")); err != nil {
			t.Fatal(err)
		}
	} else {
		if !strings.HasPrefix(name, "hyjs_test_p1_feedback78_") {
			t.Fatal("feedback78 tests require a dedicated managed MySQL test database")
		}
		cfg := db.MySQLConfig{Host: os.Getenv("HYJS_P1_MYSQL_HOST"), User: os.Getenv("HYJS_P1_MYSQL_USER"),
			Password: os.Getenv("HYJS_P1_MYSQL_PASSWORD"), Database: name}
		if err := db.InitGameMySQL(ctx, cfg); err != nil {
			t.Fatal(err)
		}
		var actual string
		if err := db.GetIns().Raw("SELECT DATABASE()").Scan(&actual).Error; err != nil || actual != name {
			t.Fatal("feedback78 database identity mismatch")
		}
	}
	t.Cleanup(func() { pool, _ := db.GetIns().DB(); pool.Close() })
	if err := db.GetIns().AutoMigrate(&db.AdminUser{}, &db.AdminAudit{}, &db.ClientUpdatePolicy{},
		&db.AdminActivityDefinition{}, &db.AdminActivityOccurrence{}, &db.AdminActivityScheduleRevision{},
		&db.PeriodInstance{}, &db.PeriodProgress{}, &db.RewardDelivery{}, &db.VIPShopProduct{}, &db.MembershipClaim{}); err != nil {
		t.Fatal(err)
	}
	if name != "" {
		periods, occurrences, sources := feedback78FixtureIDs()
		// Fixed identities are required for the production guard; never replace an existing fixture or business row.
		for _, check := range []struct {
			table any
			where string
			args  []any
		}{
			{&db.Profile{}, "id=? OR uid=?", []any{42, "feedback78-player"}},
			{&db.AdminActivityDefinition{}, "id=?", []any{constUtil.Feedback78TemplateID}},
			{&db.VIPShopProduct{}, "id=?", []any{constUtil.P3VIP10GrowthProduct}},
			{&db.ClientUpdatePolicy{}, "id=?", []any{1}},
			{&db.AdminActivityScheduleRevision{}, "server_id=?", []any{1}},
			{&db.AdminUser{}, "username=?", []any{"feedback78-isolated"}},
			{&db.PeriodInstance{}, "id IN ?", []any{periods}},
			{&db.PeriodProgress{}, "instance_id IN ?", []any{periods}},
			{&db.AdminActivityOccurrence{}, "id IN ?", []any{occurrences}},
			{&db.RewardDelivery{}, "player_id IN ? AND source_key IN ?", []any{[]int64{1, 42, 43, 44, 99}, sources}},
			{&db.MembershipClaim{}, "player_id=? AND kind=?", []any{42, "vip-shop:" + constUtil.P3VIP10GrowthProduct}},
			{&db.AdminAudit{}, "actor=? AND action=?", []any{"feedback78-isolated", constUtil.Feedback78AuditAction}},
		} {
			var count int64
			if err := db.GetIns().Model(check.table).Where(check.where, check.args...).Count(&count).Error; err != nil || count != 0 {
				t.Fatal("feedback78 fixture identity is not empty", err)
			}
		}
		t.Cleanup(func() { feedback78CleanupMySQL(t) })
	}
	user := db.AdminUser{Username: "feedback78-isolated", PasswordHash: "fixture", Role: constUtil.AdminRoleAdmin, Enabled: true}
	vip := db.VIPShopProduct{ID: constUtil.P3VIP10GrowthProduct, MinVIP: 10, CostResource: "diamond", Cost: 1100, Reward: "item_044,1", Source: constUtil.P3VIP10GrowthSource, Enabled: true}
	player, game := `{"fixture":"player unchanged"}`, `{"fixture":"game unchanged"}`
	profile := db.Profile{Id: 42, Uid: "feedback78-player", Session: []byte("1234567890123456"), Player: &player, Game: &game, ServerID: 1}
	form := model.AdminActivityForm{Name: constUtil.Feedback78ActivityName, Mode: constUtil.Feedback78ActivityMode,
		TemplateID: "login-total-original", Source: constUtil.Feedback78ActivitySource,
		Milestones: []model.AdminActivityMilestone{{Goal: 3, Rewards: []model.AdminResourceAmount{{ResourceID: "item_9711", Amount: 288}}}}}
	template := db.AdminActivityDefinition{ID: constUtil.Feedback78TemplateID, Name: form.Name, Mode: form.Mode, FormJSON: adminJSON(form), Enabled: true, Usable: true, Revision: 2}
	for _, row := range []any{&user, &vip, &profile, &template, &db.ClientUpdatePolicy{ID: 1, Maintenance: true}, &db.AdminActivityScheduleRevision{ServerID: 1, Revision: 5}} {
		if err := db.GetIns().Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	f := &feedback78Fixture{s: &GameService{}, ctx: ctx, actor: &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}, profile: profile, vip: vip}
	start := time.Now().UTC().Add(-time.Hour).UnixMilli()
	week := int64(7 * 24 * time.Hour / time.Millisecond)
	for i := 0; i < constUtil.Feedback78ScheduleCount; i++ {
		key := fmt.Sprintf("totalLogin_p1_admin_schedule_feedback%02d", i)
		at := start + int64(i)*week
		period := db.PeriodInstance{ID: key, Enabled: true, Title: form.Name, Kind: constUtil.P1EventLoginDay, Mode: "cumulative", Goal: 3,
			Rewards: constUtil.Feedback78ActivityReward, StartAt: at, EndAt: at + week, RewardEndAt: at + week, Source: form.Source}
		period.Frozen = adminJSON(period)
		occ := db.AdminActivityOccurrence{ID: fmt.Sprintf("occ_feedback%02d", i), ServerID: 1, TemplateID: template.ID,
			Name: form.Name, Mode: form.Mode, Timing: "calendar", ConsumerKey: key, ConsumerID: "period:" + key,
			StartAt: at, EndAt: at + week, ClaimEndAt: at + week, Lane: 6, Enabled: true, FormJSON: adminJSON(form), Revision: 1}
		for _, row := range []any{&period, &occ} {
			if err := db.GetIns().Create(row).Error; err != nil {
				t.Fatal(err)
			}
		}
		f.periods = append(f.periods, period)
	}
	progress := db.PeriodProgress{InstanceID: f.periods[0].ID, PlayerID: profile.Id, Snapshot: f.periods[0].Frozen, Value: 3}
	if err := db.GetIns().Create(&progress).Error; err != nil {
		t.Fatal(err)
	}
	return f
}

func TestFeedback78ConfigPreservesHistoryAndIsIdempotent(t *testing.T) {
	f := feedback78Setup(t)
	now := time.Now().UnixMilli()
	first, second := p1PeriodSources(f.periods[0])[0], p1PeriodSources(f.periods[1])[0]
	pending := db.RewardDelivery{PlayerID: 42, SourceKey: first, Title: "目标", Rewards: "item_9711,288", ExpiresAt: now + 86400000}
	orphan := db.RewardDelivery{PlayerID: 99, SourceKey: second, Title: "孤儿附件", Rewards: "item_9711,288", ExpiresAt: 0}
	claimed := db.RewardDelivery{PlayerID: 43, SourceKey: first, Title: "已领取", Rewards: "item_9711,288", ExpiresAt: now + 86400000, ClaimedAt: now - 1000, Result: []byte{1, 2, 3}}
	expired := db.RewardDelivery{PlayerID: 44, SourceKey: first, Title: "已过期", Rewards: "item_9711,288", ExpiresAt: now - 1000}
	other := db.RewardDelivery{PlayerID: 42, SourceKey: "activity:unrelated-login", Title: "保留", Rewards: "item_9711,288", ExpiresAt: now + 86400000}
	claim := db.MembershipClaim{PlayerID: 42, Kind: "vip-shop:" + f.vip.ID, Period: "2026-10-27", Reward: f.vip.Reward, At: now - 500}
	foreignPeriod := f.periods[0]
	foreignPeriod.ID = "totalLogin_p1_admin_schedule_foreign"
	foreignPeriod.Frozen = nil
	foreignOccurrence := db.AdminActivityOccurrence{ID: "occ_foreign", ServerID: 2, TemplateID: constUtil.Feedback78TemplateID,
		ConsumerKey: foreignPeriod.ID, ConsumerID: "period:" + foreignPeriod.ID, Enabled: true, FormJSON: []byte(`{}`), Revision: 1}
	for _, row := range []any{&pending, &orphan, &claimed, &expired, &other, &claim, &foreignPeriod, &foreignOccurrence} {
		if err := db.GetIns().Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	before := time.Now().UnixMilli()
	if err := f.s.ApplyFeedback78Config(f.ctx, f.actor); err != nil {
		t.Fatal(err)
	}
	after := time.Now().UnixMilli()
	for _, original := range []db.RewardDelivery{pending, orphan} {
		var got db.RewardDelivery
		if err := db.GetIns().First(&got, original.ID).Error; err != nil {
			t.Fatal(err)
		}
		if got.ExpiresAt < before || got.ExpiresAt > after || got.ClaimedAt != 0 || got.SourceKey != original.SourceKey || got.Rewards != original.Rewards || !reflect.DeepEqual(got.Result, original.Result) {
			t.Fatalf("pending receipt not retired safely: %+v", got)
		}
	}
	for _, original := range []db.RewardDelivery{claimed, expired, other} {
		var got db.RewardDelivery
		if err := db.GetIns().First(&got, original.ID).Error; err != nil || !reflect.DeepEqual(got, original) {
			t.Fatalf("claimed, expired or other receipt changed: %+v %v", got, err)
		}
	}
	var profile db.Profile
	if err := db.GetIns().First(&profile, f.profile.Id).Error; err != nil || !reflect.DeepEqual(profile, f.profile) {
		t.Fatal("profile assets changed", err)
	}
	var savedClaim db.MembershipClaim
	if err := db.GetIns().First(&savedClaim, "player_id=? AND kind=? AND period=?", claim.PlayerID, claim.Kind, claim.Period).Error; err != nil || !reflect.DeepEqual(savedClaim, claim) {
		t.Fatal("VIP purchase history changed", err)
	}
	var foreign db.AdminActivityOccurrence
	if err := db.GetIns().First(&foreign, "id=?", foreignOccurrence.ID).Error; err != nil || !foreign.Enabled || foreign.Archived || foreign.Revision != 1 {
		t.Fatal("other realm occurrence changed", err)
	}
	var foreignAfter db.PeriodInstance
	if err := db.GetIns().First(&foreignAfter, "id=?", foreignPeriod.ID).Error; err != nil || !reflect.DeepEqual(foreignAfter, foreignPeriod) {
		t.Fatal("other realm period changed", err)
	}
	var vip db.VIPShopProduct
	if err := db.GetIns().First(&vip, "id=?", f.vip.ID).Error; err != nil {
		t.Fatal(err)
	}
	want := f.vip
	want.Cost, want.Source = 2888, constUtil.Feedback78VIPCurrentSource
	if !reflect.DeepEqual(vip, want) {
		t.Fatalf("VIP terms changed unexpectedly: %+v", vip)
	}
	var template db.AdminActivityDefinition
	if err := db.GetIns().First(&template, "id=?", constUtil.Feedback78TemplateID).Error; err != nil || !template.Archived || template.Enabled || template.Revision != 3 {
		t.Fatal("template not retired", err)
	}
	var revision db.AdminActivityScheduleRevision
	if err := db.GetIns().First(&revision, "server_id=1").Error; err != nil || revision.Revision != 6 {
		t.Fatal("schedule revision", revision, err)
	}
	var occurrences []db.AdminActivityOccurrence
	if err := db.GetIns().Where("server_id=1 AND template_id=?", template.ID).Find(&occurrences).Error; err != nil || len(occurrences) != 26 {
		t.Fatal("occurrence inventory", err)
	}
	for _, row := range occurrences {
		if !row.Archived || row.Enabled || row.Revision != 2 {
			t.Fatal("occurrence not retired", row.ID)
		}
	}
	for _, original := range f.periods {
		var got db.PeriodInstance
		if err := db.GetIns().First(&got, "id=?", original.ID).Error; err != nil {
			t.Fatal(err)
		}
		original.Enabled = false
		if !reflect.DeepEqual(got, original) {
			t.Fatal("period/frozen history changed", got.ID)
		}
	}
	bound := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	if _, err := f.s.p1ActivityApply(db.GetIns().WithContext(bound), f.profile.Id, &pb.Player{}, &model.GameState{}, f.periods[0].ID, &pb.CSActivityReward{}, after); err == nil {
		t.Fatal("retired activity accepted a direct claim")
	}
	var progress db.PeriodProgress
	if err := db.GetIns().First(&progress, "instance_id=? AND player_id=?", f.periods[0].ID, f.profile.Id).Error; err != nil || !reflect.DeepEqual(progress.Snapshot, f.periods[0].Frozen) || progress.Value != 3 {
		t.Fatal("progress snapshot changed", err)
	}
	if err := f.s.ApplyFeedback78Config(f.ctx, f.actor); err != nil {
		t.Fatal("idempotent retry", err)
	}
	if err := db.GetIns().First(&revision, "server_id=1").Error; err != nil || revision.Revision != 6 || p1Count(t, &db.AdminAudit{}, "action=?", constUtil.Feedback78AuditAction) != 1 {
		t.Fatal("retry changed revision or audit", err)
	}
}

func TestFeedback78ConfigRejectsDriftWithoutPartialWrites(t *testing.T) {
	for _, scenario := range []string{"maintenance", "vip-price", "vip-source", "vip-reward", "vip-limit", "operator", "template", "snapshot", "schedule-count", "audit-failure"} {
		t.Run(scenario, func(t *testing.T) {
			f := feedback78Setup(t)
			pending := db.RewardDelivery{PlayerID: f.profile.Id, SourceKey: p1PeriodSources(f.periods[0])[0], Title: "隔离附件", Rewards: constUtil.Feedback78ActivityReward, ExpiresAt: time.Now().Add(time.Hour).UnixMilli()}
			if err := db.GetIns().Create(&pending).Error; err != nil {
				t.Fatal(err)
			}
			var err error
			switch scenario {
			case "maintenance":
				err = db.GetIns().Model(&db.ClientUpdatePolicy{}).Where("id=1").Update("maintenance", false).Error
			case "vip-price":
				err = db.GetIns().Model(&db.VIPShopProduct{}).Where("id=?", f.vip.ID).Update("cost", 1200).Error
			case "vip-source":
				err = db.GetIns().Model(&db.VIPShopProduct{}).Where("id=?", f.vip.ID).Update("source", "operator changed source").Error
			case "vip-reward":
				err = db.GetIns().Model(&db.VIPShopProduct{}).Where("id=?", f.vip.ID).Update("reward", "item_064,1").Error
			case "vip-limit":
				err = db.GetIns().Model(&db.VIPShopProduct{}).Where("id=?", f.vip.ID).Update("total_limit", 1).Error
			case "operator":
				err = db.GetIns().Model(&db.AdminUser{}).Where("id=?", f.actor.UserID).Update("role", constUtil.AdminRoleOperator).Error
			case "template":
				err = db.GetIns().Model(&db.AdminActivityDefinition{}).Where("id=?", constUtil.Feedback78TemplateID).Update("name", "其他活动").Error
			case "snapshot":
				bad := f.periods[0]
				bad.RewardSourceID = "unrelated"
				err = db.GetIns().Model(&db.PeriodProgress{}).Where("instance_id=? AND player_id=?", bad.ID, f.profile.Id).Update("snapshot", adminJSON(bad)).Error
			case "schedule-count":
				err = db.GetIns().Where("id=?", "occ_feedback25").Delete(&db.AdminActivityOccurrence{}).Error
			case "audit-failure":
				statement := "CREATE TRIGGER reject_feedback78_audit BEFORE INSERT ON admin_audits BEGIN SELECT RAISE(ABORT, 'isolated audit failure'); END"
				if db.GetIns().Dialector.Name() == "mysql" {
					statement = "CREATE TRIGGER reject_feedback78_audit BEFORE INSERT ON admin_audits FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'isolated audit failure'"
				}
				err = db.GetIns().Exec(statement).Error
				if err == nil {
					t.Cleanup(func() {
						if e := db.GetIns().Exec("DROP TRIGGER IF EXISTS reject_feedback78_audit").Error; e != nil {
							t.Errorf("remove isolated audit trigger: %v", e)
						}
					})
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = f.s.ApplyFeedback78Config(f.ctx, f.actor); err == nil {
				t.Fatal("guard accepted", scenario)
			}
			var template db.AdminActivityDefinition
			var period db.PeriodInstance
			var revision db.AdminActivityScheduleRevision
			if err = db.GetIns().First(&template, "id=?", constUtil.Feedback78TemplateID).Error; err != nil || template.Archived || !template.Enabled {
				t.Fatal("failed guard changed template", err)
			}
			if err = db.GetIns().First(&period, "id=?", f.periods[0].ID).Error; err != nil || !period.Enabled {
				t.Fatal("failed guard changed period", err)
			}
			if err = db.GetIns().First(&revision, "server_id=1").Error; err != nil || revision.Revision != 5 || p1Count(t, &db.AdminAudit{}, "action=?", constUtil.Feedback78AuditAction) != 0 {
				t.Fatal("failed guard changed revision or audit", err)
			}
			var after db.RewardDelivery
			if err = db.GetIns().First(&after, pending.ID).Error; err != nil || !reflect.DeepEqual(after, pending) {
				t.Fatal("failed transaction changed pending receipt", err)
			}
			if scenario == "audit-failure" {
				var vip db.VIPShopProduct
				if err = db.GetIns().First(&vip, "id=?", f.vip.ID).Error; err != nil || !reflect.DeepEqual(vip, f.vip) {
					t.Fatal("audit failure did not roll back VIP price", err)
				}
			}
		})
	}
}

func TestFeedback78ExpiryUsesRealmClock(t *testing.T) {
	f := feedback78Setup(t)
	if err := db.GetIns().AutoMigrate(&db.ServerGameClock{}); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&db.ServerGameClock{ServerID: 1, OffsetDays: 3, HighDays: 3, Revision: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if db.GetIns().Dialector.Name() == "mysql" {
		t.Cleanup(func() {
			if err := db.GetIns().Where("server_id=? AND offset_days=? AND high_days=? AND revision=?", 1, 3, 3, 1).Delete(&db.ServerGameClock{}).Error; err != nil {
				t.Errorf("remove isolated realm clock: %v", err)
			}
		})
	}
	f.s.ServerClockReady = true
	pending := db.RewardDelivery{PlayerID: f.profile.Id, SourceKey: p1PeriodSources(f.periods[0])[0], Title: "时钟附件", Rewards: constUtil.Feedback78ActivityReward}
	if err := db.GetIns().Create(&pending).Error; err != nil {
		t.Fatal(err)
	}
	before := time.Now().UTC().AddDate(0, 0, 3).UnixMilli()
	if err := f.s.ApplyFeedback78Config(f.ctx, f.actor); err != nil {
		t.Fatal(err)
	}
	after := time.Now().UTC().AddDate(0, 0, 3).UnixMilli()
	if err := db.GetIns().First(&pending, pending.ID).Error; err != nil || pending.ExpiresAt < before || pending.ExpiresAt > after {
		t.Fatal("retirement did not use realm game time", pending.ExpiresAt, err)
	}
}

func TestFeedback78PreflightRejectsOversize(t *testing.T) {
	f := feedback78Setup(t)
	if err := db.GetIns().Create(&db.RewardDelivery{PlayerID: 1, SourceKey: "unrelated", Title: "test", Rewards: "coin,1"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := feedback78Preflight(db.GetIns(), []string{f.periods[0].ID}, 0, 100); err == nil {
		t.Fatal("delivery scan cap ignored")
	}
	if _, err := feedback78Preflight(db.GetIns(), []string{f.periods[0].ID}, 100, 0); err == nil {
		t.Fatal("progress cap ignored")
	}
}

func TestFeedback78PublishedInputOmitsWeeklyBonus(t *testing.T) {
	var activities []growth74Activity
	if err := json.Unmarshal(growth74ActivitiesJSON, &activities); err != nil {
		t.Fatal(err)
	}
	var starter *growth74Activity
	for _, activity := range activities {
		if activity.Form.Mode == constUtil.Feedback78ActivityMode && activity.Form.Source == constUtil.Feedback78ActivitySource &&
			len(activity.Form.Milestones) == 1 && activity.Form.Milestones[0].Goal == 3 &&
			len(activity.Form.Milestones[0].Rewards) == 1 && activity.Form.Milestones[0].Rewards[0].ResourceID == "item_9711" && activity.Form.Milestones[0].Rewards[0].Amount == 288 {
			t.Fatal("unapproved weekly activity remains")
		}
		if activity.Timing == "character-day" && activity.Form.Name == "启程装备补给" {
			if starter != nil {
				t.Fatal("duplicate starter login rewards")
			}
			item := activity
			starter = &item
		}
	}
	if starter == nil || len(starter.Form.Milestones) != 3 {
		t.Fatal("starter D1-D3 login rewards missing")
	}
	want := []model.AdminActivityMilestone{
		{Goal: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "item_9760", Amount: 1}}},
		{Goal: 2, Rewards: []model.AdminResourceAmount{{ResourceID: "equip_025", Amount: 1}, {ResourceID: "equip_026", Amount: 1}}},
		{Goal: 3, Rewards: []model.AdminResourceAmount{{ResourceID: "equip_027", Amount: 1}, {ResourceID: "equip_028", Amount: 1}}},
	}
	if !reflect.DeepEqual(starter.Form.Milestones, want) {
		t.Fatal("starter D1-D3 login rewards changed")
	}
}
