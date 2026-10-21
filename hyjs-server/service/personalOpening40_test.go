package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestPersonalOpening40MigrationSharesHistoricalClaimsAndRetainsFrozenRules
// consumes actual typed period messages against an isolated SQLite fixture.
func TestPersonalOpening40MigrationSharesHistoricalClaimsAndRetainsFrozenRules(t *testing.T) {
	f, actor := adminModesFixture(t)
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	for _, initialize := range []func(context.Context) error{f.s.InitializePlayerOpeningCalendar, f.s.InitializeAdminGameServers, f.s.InitializeAdminActivitySchedules, f.s.InitializeServerGameClock} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	// 角色处于个人七日窗口的最后一天，后续用获准的一天日历变化验证到期。
	created := now.AddDate(0, 0, -6)
	zone := time.FixedZone("UTC+8", 8*3600)
	localCreated := created.In(zone)
	anchor := time.Date(localCreated.Year(), localCreated.Month(), localCreated.Day(), 0, 0, 0, 0, zone).UnixMilli()
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", f.id).Update("created_at", created.UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Save(&db.PlayerOpeningCalendar{PlayerID: f.id, StartAt: anchor, CreatedAt: now.UnixMilli(), Source: constUtil.PlayerOpeningCalendarSource}).Error; err != nil {
		t.Fatal(err)
	}
	start := now.Add(-time.Hour)
	end := now.AddDate(0, 0, 7)
	form := adminModeForm("copy-win")
	form.Milestones = []model.AdminActivityMilestone{{Goal: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 7}}}, {Goal: 3, Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 11}}}}
	if err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error { return f.s.adminRestoreReActivity(tx, 1, form, start, end, now) }); err != nil {
		t.Fatal(err)
	}
	source := personalOpening40Source(form.Mode, 1)
	var old db.PeriodInstance
	if err := db.GetIns().First(&old, "id=?", source).Error; err != nil {
		t.Fatal(err)
	}
	snapshot, _ := json.Marshal(old)
	previous := db.PeriodProgress{PlayerID: f.id, InstanceID: source, Snapshot: snapshot, Value: 3, LastDay: p1Day(now)}
	if err := db.GetIns().Create(&previous).Error; err != nil {
		t.Fatal(err)
	}
	sources := p1PeriodSources(old)
	claimed := db.RewardDelivery{PlayerID: f.id, SourceKey: sources[0], Title: old.Title, Content: "history", Rewards: "coin,7", CreatedAt: now.UnixMilli() - 2000, ExpiresAt: now.UnixMilli() - 1000, ClaimedAt: now.UnixMilli() - 1500}
	pending := db.RewardDelivery{PlayerID: f.id, SourceKey: sources[1], Title: old.Title, Content: "pending", Rewards: "coin,11", CreatedAt: now.UnixMilli() - 2000, ExpiresAt: now.UnixMilli() - 1000}
	for _, row := range []*db.RewardDelivery{&claimed, &pending} {
		if err := db.GetIns().Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.InitializePersonalOpening40(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializePersonalOpening40(f.ctx); err != nil {
		t.Fatal("migration replay", err)
	}
	if got := p1Count(t, &db.AdminAudit{}, "action=?", "personal-opening40.migrate"); got != 1 {
		t.Fatal("migration duplicated", got)
	}
	var profile db.Profile
	if err := db.GetIns().First(&profile, f.id).Error; err != nil {
		t.Fatal(err)
	}
	bound, err := f.s.BindServerGameClockContext(f.ctx, &profile)
	if err != nil {
		t.Fatal(err)
	}
	f.ctx = bound
	if err = f.s.PrepareAdminCharacterActivitySchedules(bound, f.id); err != nil {
		t.Fatal(err)
	}
	var issued db.AdminCharacterActivityInstance
	if err = db.GetIns().First(&issued, "player_id=? AND occurrence_id=?", f.id, personalOpening40ID(form.Mode, 1)).Error; err != nil {
		t.Fatal(err)
	}
	var current db.PeriodInstance
	if err = db.GetIns().First(&current, "id=?", issued.ConsumerKey).Error; err != nil {
		t.Fatal(err)
	}
	if current.RewardSourceID != source {
		t.Fatal("historical source lost", current.RewardSourceID)
	}
	forged := current
	forged.RewardSourceID = source + "_foreign"
	if err = personalOpening40ValidateSource(db.GetIns().WithContext(bound), forged, f.id); err == nil {
		t.Fatal("unverified historical reward alias accepted")
	}
	if err = personalOpening40ValidateSource(db.GetIns().WithContext(bound), current, f.id+999); err == nil {
		t.Fatal("foreign player borrowed historical reward source")
	}
	before, _ := f.profile(t)
	replies, err := f.s.P1Activity(bound, f.id, &pb.CSActivityInfo{ActivityId: proto.String(current.ID)})
	if err != nil {
		t.Fatal("personal info", err)
	}
	info := replies[0].(*pb.SCActivityInfo)
	if len(info.GetItemList()) != 2 || info.GetItemList()[0].GetStatus() != 2 || info.GetItemList()[1].GetStatus() != 1 || info.GetItemList()[1].GetLeft() != 3 {
		t.Fatal("history projected incorrectly", info)
	}
	if err = db.GetIns().First(&pending, pending.ID).Error; err != nil || pending.ExpiresAt != current.RewardEndAt {
		t.Fatal("pending window not renewed", pending, err)
	}
	request := &pb.CSActivityReward{ActivityId: proto.String(current.ID), Index: proto.Int32(2), GetResources: info.GetItemList()[1].RewardList}
	if _, err = f.s.P1Activity(bound, f.id, request); err != nil {
		t.Fatal("old pending personal claim", err)
	}
	if _, err = f.s.P1Activity(bound, f.id, request); err != nil {
		t.Fatal("claim retry", err)
	}
	after, _ := f.profile(t)
	if after.GetCoin() != before.GetCoin()+11 {
		t.Fatal("historical reward duplicated", before.GetCoin(), after.GetCoin())
	}
	if got := p1Count(t, &db.RewardDelivery{}, "player_id=? AND source_key LIKE ?", f.id, "activity:"+current.ID+"%"); got != 0 {
		t.Fatal("new personal source double granted", got)
	}
	var saved db.PeriodProgress
	if err = db.GetIns().First(&saved, "player_id=? AND instance_id=?", f.id, current.ID).Error; err != nil {
		t.Fatal(err)
	}
	var frozen db.PeriodInstance
	if json.Unmarshal(saved.Snapshot, &frozen) != nil || frozen.StartAt != current.StartAt || frozen.RewardSourceID != source {
		t.Fatal("personal snapshot not frozen correctly")
	}
	rows, err := f.s.AdminPersonalOpening40(bound, actor, 1)
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	change := model.AdminPersonalOpening40SaveRequest{Version: rows[0].Version, StartDay: 2, DurationDays: 9, ClaimDays: 2, Enabled: true, Form: rows[0].Form}
	change.Form.Milestones[0].Rewards[0].Amount = 99
	if _, err = f.s.AdminSavePersonalOpening40(bound, actor, 1, rows[0].ID, change); err != nil {
		t.Fatal("future edit", err)
	}
	if _, err = f.s.AdminSavePersonalOpening40(bound, actor, 1, rows[0].ID, change); err == nil {
		t.Fatal("stale version accepted")
	}
	if err = f.s.PrepareAdminCharacterActivitySchedules(bound, f.id); err != nil {
		t.Fatal(err)
	}
	var retained db.PeriodInstance
	if err = db.GetIns().First(&retained, "id=?", current.ID).Error; err != nil || retained.EndAt != current.EndAt || retained.Rewards != current.Rewards {
		t.Fatal("issued rules changed", err)
	}
	// Pause/re-enable changes access only; it never replaces an issued snapshot.
	rows, err = f.s.AdminPersonalOpening40(bound, actor, 1)
	if err != nil {
		t.Fatal(err)
	}
	toggle := model.AdminPersonalOpening40SaveRequest{Version: rows[0].Version, StartDay: rows[0].StartDay, DurationDays: rows[0].DurationDays, ClaimDays: rows[0].ClaimDays, Enabled: false, Form: rows[0].Form}
	paused, err := f.s.AdminSavePersonalOpening40(bound, actor, 1, rows[0].ID, toggle)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.P1Activity(bound, f.id, &pb.CSActivityInfo{ActivityId: proto.String(current.ID)}); err == nil {
		t.Fatal("disabled personal activity remained accessible")
	}
	toggle.Version, toggle.Enabled = paused.Version, true
	if _, err = f.s.AdminSavePersonalOpening40(bound, actor, 1, rows[0].ID, toggle); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.P1Activity(bound, f.id, &pb.CSActivityInfo{ActivityId: proto.String(current.ID)}); err != nil {
		t.Fatal("re-enabled instance lost access", err)
	}
	f.restart(t)
	if _, err = f.s.P1Activity(bound, f.id, request); err != nil {
		t.Fatal("cold claim retry", err)
	}
	// The edited source covers day eight, but the original issued seven-day window has ended.
	// Advancing this isolated fixture's clock cannot issue the same occurrence a second time.
	if err = db.GetIns().Model(&db.ServerGameClock{}).Where("server_id=?", f.s.serverGameClockID()).Updates(map[string]any{"offset_days": int64(1), "high_days": int64(1), "revision": gorm.Expr("revision + 1")}).Error; err != nil {
		t.Fatal(err)
	}
	later, err := f.s.BindServerGameClockContext(bound, &profile)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.PrepareAdminCharacterActivitySchedules(later, f.id); err != nil {
		t.Fatal(err)
	}
	if got := p1Count(t, &db.AdminCharacterActivityInstance{}, "player_id=? AND occurrence_id=?", f.id, issued.OccurrenceID); got != 1 {
		t.Fatal("edited source reissued expired instance", got)
	}
	if _, err = f.s.P1Activity(later, f.id, &pb.CSActivityInfo{ActivityId: proto.String(current.ID)}); err == nil {
		t.Fatal("edited source reopened original issued window")
	}
}

func TestPersonalOpening40ExpiredCharacterDoesNotReopen(t *testing.T) {
	f, _ := adminModesFixture(t)
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	for _, initialize := range []func(context.Context) error{f.s.InitializePlayerOpeningCalendar, f.s.InitializeAdminGameServers, f.s.InitializeAdminActivitySchedules, f.s.InitializeServerGameClock} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	form := adminModeForm("arena-wins")
	form.Milestones = []model.AdminActivityMilestone{{Goal: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 7}}}}
	if err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
		return f.s.adminRestoreReActivity(tx, 1, form, now.Add(-time.Hour), now.AddDate(0, 0, 7), now)
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializePersonalOpening40(f.ctx); err != nil {
		t.Fatal(err)
	}
	old := now.AddDate(0, 0, -30).UnixMilli()
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", f.id).Update("created_at", old).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Save(&db.PlayerOpeningCalendar{PlayerID: f.id, StartAt: old, CreatedAt: old, Source: constUtil.PlayerOpeningCalendarSource}).Error; err != nil {
		t.Fatal(err)
	}
	var profile db.Profile
	if err := db.GetIns().First(&profile, f.id).Error; err != nil {
		t.Fatal(err)
	}
	bound, err := f.s.BindServerGameClockContext(f.ctx, &profile)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.PrepareAdminCharacterActivitySchedules(bound, f.id); err != nil {
		t.Fatal(err)
	}
	if got := p1Count(t, &db.AdminCharacterActivityInstance{}, "player_id=?", f.id); got != 0 {
		t.Fatal("old character reopened", got)
	}
	if !adminActivityWindowsOverlap(model.AdminActivityOccurrenceDTO{Timing: "calendar"}, model.AdminActivityOccurrenceDTO{Timing: "character-day"}, "limited-sign") {
		t.Fatal("no-ID welfare conflict removed")
	}
	if adminActivityWindowsOverlap(model.AdminActivityOccurrenceDTO{Timing: "calendar"}, model.AdminActivityOccurrenceDTO{Timing: "character-day"}, "plunder-count") {
		t.Fatal("supported personal/calendar blocked")
	}
}
