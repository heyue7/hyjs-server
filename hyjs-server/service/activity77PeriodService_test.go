package service

import (
	"bytes"
	"encoding/json"
	"os"
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

func TestActivity77PeriodMigratesClaimedTrial(t *testing.T) {
	f, actor := reuseFixture(t, false)
	pairs, err := activity77PeriodPairs()
	if err != nil {
		t.Fatal(err)
	}
	pair := pairs[2]
	f.s.InstallGrowth74Resources()
	for i, other := range pairs {
		if i == 2 {
			continue
		}
		if _, err = f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: other.old.Name, Form: other.old, DefaultDays: 1, Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	def, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: pair.old.Name, Form: pair.old, DefaultDays: 7, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	cal, err := f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil {
		t.Fatal(err)
	}
	cal, err = f.s.AdminSaveActivitySchedule(f.ctx, actor, 1, model.AdminActivityScheduleSaveRequest{Version: cal.Version, Items: append(cal.Items, model.AdminActivityOccurrenceDTO{TemplateID: def.ID, TemplateVersion: def.Version, Timing: "calendar", StartAt: now - 1000, EndAt: now + 86400000, ClaimEndAt: now + 86400000, Enabled: true})})
	if err != nil {
		t.Fatal(err)
	}
	var row db.AdminActivityOccurrence
	if err = db.GetIns().First(&row, "id=?", cal.Items[len(cal.Items)-1].ID).Error; err != nil {
		t.Fatal(err)
	}
	var cfg db.PeriodInstance
	if err = db.GetIns().First(&cfg, "id=?", row.ConsumerKey).Error; err != nil {
		t.Fatal(err)
	}
	cfg.Frozen = adminJSON(cfg)
	if err = db.GetIns().Save(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	p := db.PeriodProgress{PlayerID: f.id, InstanceID: cfg.ID, Value: 27, Snapshot: adminJSON(cfg)}
	if err = db.GetIns().Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	old := db.RewardDelivery{PlayerID: f.id, SourceKey: p1PeriodSources(cfg)[0], Title: cfg.Title, Content: cfg.Source, Rewards: cfg.Rewards, CreatedAt: now - 2000, ExpiresAt: cfg.RewardEndAt, ClaimedAt: now - 1000}
	if err = db.GetIns().Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&p).Update("reward_id", old.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.InitializeClientUpdates(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.ClientUpdatePolicy{}).Where("id=?", 1).Update("maintenance", true).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.ApplyActivity77Periods(f.ctx, actor); err != nil {
		t.Fatal(err)
	}
	var migrated db.PeriodInstance
	if err = db.GetIns().First(&migrated, "id=?", cfg.ID).Error; err != nil {
		t.Fatal(err)
	}
	var periodFrozen db.PeriodInstance
	if json.Unmarshal(migrated.Frozen, &periodFrozen) != nil || len(p1PeriodSources(periodFrozen)) != 5 {
		t.Fatal("period frozen not migrated")
	}
	if len(p1PeriodSources(migrated)) != 5 {
		t.Fatal("wrong tier count")
	}
	for _, key := range p1PeriodSources(migrated) {
		var receipt db.RewardDelivery
		if err = db.GetIns().First(&receipt, "player_id=? AND source_key=?", f.id, key).Error; err != nil || receipt.ClaimedAt != old.ClaimedAt {
			t.Fatal("claim coverage", key, err)
		}
	}
	if err = db.GetIns().First(&p, "instance_id=? AND player_id=?", cfg.ID, f.id).Error; err != nil {
		t.Fatal(err)
	}
	var firstMapped db.RewardDelivery
	if err = db.GetIns().First(&firstMapped, "player_id=? AND source_key=?", f.id, p1PeriodSources(migrated)[0]).Error; err != nil || p.RewardID != firstMapped.ID {
		t.Fatal("progress reward reference", err)
	}
	var snap db.PeriodInstance
	if err = json.Unmarshal(p.Snapshot, &snap); err != nil || p.Value != 27 || len(p1PeriodSources(snap)) != 5 {
		t.Fatal("progress", err)
	}
	err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		items, deliveries, e := f.s.p1PeriodItems(tx, f.id, &p, snap, time.Now().UnixMilli())
		if e != nil {
			return e
		}
		if len(items) != 5 || len(deliveries) != 5 {
			t.Fatal("missing tiers")
		}
		for _, item := range items {
			if item.GetStatus() != 2 {
				t.Fatal("claimed tier became available", item)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.ApplyActivity77Periods(f.ctx, actor); err != nil {
		t.Fatal("retry", err)
	}
}

func TestActivity77PeriodUnclaimedDailyReissued(t *testing.T) {
	f, actor := reuseFixture(t, false)
	pairs, err := activity77PeriodPairs()
	if err != nil {
		t.Fatal(err)
	}
	pair := pairs[1]
	def, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: pair.old.Name, Form: pair.old, DefaultDays: 1, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	cal, err := f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil {
		t.Fatal(err)
	}
	cal, err = f.s.AdminSaveActivitySchedule(f.ctx, actor, 1, model.AdminActivityScheduleSaveRequest{Version: cal.Version, Items: append(cal.Items, model.AdminActivityOccurrenceDTO{TemplateID: def.ID, TemplateVersion: def.Version, Timing: "calendar", StartAt: now - 1000, EndAt: now + 86400000, ClaimEndAt: now + 86400000, Enabled: true})})
	if err != nil {
		t.Fatal(err)
	}
	var row db.AdminActivityOccurrence
	if err = db.GetIns().First(&row, "id=?", cal.Items[len(cal.Items)-1].ID).Error; err != nil {
		t.Fatal(err)
	}
	var cfg db.PeriodInstance
	if err = db.GetIns().First(&cfg, "id=?", row.ConsumerKey).Error; err != nil {
		t.Fatal(err)
	}
	progress := db.PeriodProgress{PlayerID: f.id, InstanceID: cfg.ID, Value: 6, Snapshot: adminJSON(cfg)}
	if err = db.GetIns().Create(&progress).Error; err != nil {
		t.Fatal(err)
	}
	original := db.RewardDelivery{PlayerID: f.id, SourceKey: p1PeriodSources(cfg)[0], Title: cfg.Title, Content: cfg.Source, Rewards: cfg.Rewards, CreatedAt: now - 1000, ExpiresAt: cfg.RewardEndAt}
	if err = db.GetIns().Create(&original).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&progress).Update("snapshot", []byte(`{}`)).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.activity77PeriodOccurrence(f.ctx, actor, row, pair); err == nil {
		t.Fatal("corrupt snapshot accepted")
	}
	var rollback db.RewardDelivery
	if err = db.GetIns().First(&rollback, original.ID).Error; err != nil || rollback.ExpiresAt != original.ExpiresAt {
		t.Fatal("failed migration changed old receipt", err)
	}
	if err = db.GetIns().Model(&progress).Update("snapshot", adminJSON(cfg)).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.activity77PeriodOccurrence(f.ctx, actor, row, pair); err != nil {
		t.Fatal(err)
	}
	var saved db.RewardDelivery
	if err = db.GetIns().First(&saved, original.ID).Error; err != nil || saved.ExpiresAt > time.Now().UnixMilli() || saved.Rewards != original.Rewards || saved.Content != original.Content {
		t.Fatal("old notice", err, saved)
	}
	if _, err = f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{original.ID}}); err == nil {
		t.Fatal("retired notice still claimable")
	}
	if err = db.GetIns().First(&progress, "instance_id=? AND player_id=?", cfg.ID, f.id).Error; err != nil {
		t.Fatal(err)
	}
	var snapshot db.PeriodInstance
	if err = json.Unmarshal(progress.Snapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	var newID int64
	var firstResources []*pb.BaseResource
	err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		items, deliveries, e := f.s.p1PeriodItems(tx, f.id, &progress, snapshot, time.Now().UnixMilli())
		if e != nil {
			return e
		}
		if len(items) != 3 {
			t.Fatal("tiers")
		}
		newID = deliveries[0].ID
		firstResources = items[0].GetRewardList()
		for _, r := range deliveries {
			if r.ID == 0 || r.ClaimedAt != 0 {
				t.Fatal("new entitlement", r)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := f.profile(t)
	if _, err = f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{newID}}); err == nil || err.Error() != constUtil.P1ActivityRewardPageOnly {
		t.Fatal("new activity notice accepted by center", err)
	}
	claim := &pb.CSActivityReward{ActivityId: proto.String(cfg.ID), Index: proto.Int32(1), GetResources: firstResources}
	if _, err = f.s.P1Activity(f.ctx, f.id, claim); err != nil {
		t.Fatal("new activity claim", err)
	}
	after, _ := f.profile(t)
	if after.GetCoin()-before.GetCoin() != 3000000 {
		t.Fatal("wrong new coin reward")
	}
	if _, err = f.s.P1Activity(f.ctx, f.id, claim); err != nil {
		t.Fatal("activity claim retry", err)
	}
	if out, err := f.s.RewardCenter(f.ctx, f.id, &pb.CSAddRewardsToPlayer{RewardIds: []int64{newID}}); err != nil || len(out[0].(*pb.SCAddRewardsToPlayer).GetResult().GetSimps()) != 0 {
		t.Fatal("claimed center retry changed", err)
	}
	retried, _ := f.profile(t)
	if retried.GetCoin() != after.GetCoin() {
		t.Fatal("claim retry granted twice")
	}
	f.restart(t)
	var count int64
	if err = db.GetIns().Model(&db.RewardDelivery{}).Where("player_id=? AND source_key IN ?", f.id, p1PeriodSources(snapshot)).Count(&count).Error; err != nil || count != 3 {
		t.Fatal("cold recovery", count, err)
	}
}

func TestActivity77PeriodHistoricalCoverage(t *testing.T) {
	for _, spec := range []struct {
		name      string
		pair, old int
		covered   []int
	}{
		{"opening", 0, 0, []int{1, 2}}, {"daily-2", 1, 0, []int{0}}, {"daily-4", 1, 1, []int{1}}, {"daily-6", 1, 2, []int{2}}, {"weekly-20", 3, 0, []int{0, 1, 2, 3}}, {"weekly-40", 3, 1, []int{4}},
	} {
		t.Run(spec.name, func(t *testing.T) {
			f, actor := reuseFixture(t, false)
			f.s.InstallGrowth74Resources()
			pairs, err := activity77PeriodPairs()
			if err != nil {
				t.Fatal(err)
			}
			pair := pairs[spec.pair]
			def, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: pair.old.Name, Form: pair.old, DefaultDays: 1, Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now().UnixMilli()
			cal, err := f.s.AdminActivitySchedule(f.ctx, actor, 1)
			if err != nil {
				t.Fatal(err)
			}
			cal, err = f.s.AdminSaveActivitySchedule(f.ctx, actor, 1, model.AdminActivityScheduleSaveRequest{Version: cal.Version, Items: append(cal.Items, model.AdminActivityOccurrenceDTO{TemplateID: def.ID, TemplateVersion: def.Version, Timing: "calendar", StartAt: now - 1000, EndAt: now + 86400000, ClaimEndAt: now + 86400000, Enabled: true})})
			if err != nil {
				t.Fatal(err)
			}
			var occ db.AdminActivityOccurrence
			if err = db.GetIns().First(&occ, "id=?", cal.Items[len(cal.Items)-1].ID).Error; err != nil {
				t.Fatal(err)
			}
			var cfg db.PeriodInstance
			if err = db.GetIns().First(&cfg, "id=?", occ.ConsumerKey).Error; err != nil {
				t.Fatal(err)
			}
			state := db.PeriodProgress{InstanceID: cfg.ID, PlayerID: f.id, Value: pair.old.Milestones[spec.old].Goal, Snapshot: adminJSON(cfg)}
			if err = db.GetIns().Create(&state).Error; err != nil {
				t.Fatal(err)
			}
			receipt := db.RewardDelivery{PlayerID: f.id, SourceKey: p1PeriodSources(cfg)[spec.old], Rewards: p1PeriodMilestones(cfg)[spec.old].Rewards, Title: cfg.Title, Content: cfg.Source, CreatedAt: now - 10, ExpiresAt: cfg.RewardEndAt, ClaimedAt: now - 5}
			if err = db.GetIns().Create(&receipt).Error; err != nil {
				t.Fatal(err)
			}
			before, gameBefore := f.profile(t)
			if err = f.s.activity77PeriodOccurrence(f.ctx, actor, occ, pair); err != nil {
				t.Fatal(err)
			}
			after, gameAfter := f.profile(t)
			if !bytes.Equal(adminJSON(before), adminJSON(after)) || !bytes.Equal(adminJSON(gameBefore), adminJSON(gameAfter)) {
				t.Fatal("migration changed player assets")
			}
			var next db.PeriodInstance
			if err = db.GetIns().First(&next, "id=?", cfg.ID).Error; err != nil {
				t.Fatal(err)
			}
			want := map[int]bool{}
			for _, i := range spec.covered {
				want[i] = true
			}
			for i, key := range p1PeriodSources(next) {
				var count int64
				if err = db.GetIns().Model(&db.RewardDelivery{}).Where("player_id=? AND source_key=? AND claimed_at=?", f.id, key, receipt.ClaimedAt).Count(&count).Error; err != nil || (count == 1) != want[i] {
					t.Fatal("coverage", i, count, err)
				}
			}
		})
	}
}

func TestActivity77PeriodForeignRealmPreflight(t *testing.T) {
	f, actor := reuseFixture(t, false)
	pairs, err := activity77PeriodPairs()
	if err != nil {
		t.Fatal(err)
	}
	pair := pairs[2]
	f.s.InstallGrowth74Resources()
	for i, other := range pairs {
		if i == 2 {
			continue
		}
		if _, err = f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: other.old.Name, Form: other.old, DefaultDays: 1, Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	def, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: pair.old.Name, Form: pair.old, DefaultDays: 7, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	occ := db.AdminActivityOccurrence{ID: "foreign77", ServerID: 2, TemplateID: def.ID, TemplateVersion: def.Version, Name: def.Name, Mode: def.Mode, ConsumerID: "period:foreign77", ConsumerKey: "foreign77", FormJSON: adminJSON(pair.old), StartAt: 1, EndAt: 2, ClaimEndAt: 2, Enabled: true, Revision: 1}
	if err = db.GetIns().Create(&occ).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.InitializeClientUpdates(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.ClientUpdatePolicy{}).Where("id=1").Update("maintenance", true).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.ApplyActivity77Periods(f.ctx, actor); err == nil || !strings.Contains(err.Error(), "其他区服") {
		t.Fatal("foreign realm was not rejected", err)
	}
	var template db.AdminActivityDefinition
	if err = db.GetIns().First(&template, "id=?", def.ID).Error; err != nil {
		t.Fatal(err)
	}
	var form model.AdminActivityForm
	if json.Unmarshal(template.FormJSON, &form) != nil || !activity77FormMatch(form, pair.old) {
		t.Fatal("preflight modified template")
	}
}

func TestActivity77LiveFormSnapshot(t *testing.T) {
	path := os.Getenv("HYJS_ACTIVITY77_FORM_SNAPSHOT")
	if path == "" {
		t.Skip("HYJS_ACTIVITY77_FORM_SNAPSHOT unset")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var forms []model.AdminActivityForm
	if err = json.Unmarshal(raw, &forms); err != nil {
		t.Fatal(err)
	}
	f, _ := reuseFixture(t, false)
	f.s.InstallGrowth74Resources()
	pairs, err := activity77PeriodPairs()
	if err != nil {
		t.Fatal(err)
	}
	counts := make([]int, len(pairs))
	total := 0
	for _, form := range forms {
		for i, pair := range pairs {
			if activity77FormMatch(form, pair.old) {
				counts[i]++
				total++
				t.Logf("matched group=%d mode=%s source=%s", i, form.Mode, form.Source)
			}
		}
	}
	if total != 4 {
		t.Fatalf("matched=%d groups=%v", total, counts)
	}
	for i, count := range counts {
		if count != 1 {
			t.Fatalf("group=%d matched=%d", i, count)
		}
	}
}

func TestActivity77PeriodMissingGroupRejected(t *testing.T) {
	f, actor := reuseFixture(t, false)
	if err := f.s.InitializeClientUpdates(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.ClientUpdatePolicy{}).Where("id=1").Update("maintenance", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyActivity77Periods(f.ctx, actor); err == nil || !strings.Contains(err.Error(), "未找到目标活动模板") {
		t.Fatal("missing groups accepted", err)
	}
	var audits int64
	if err := db.GetIns().Model(&db.AdminAudit{}).Where("action LIKE ?", "activity77.period.%").Count(&audits).Error; err != nil || audits != 0 {
		t.Fatal("missing group wrote changes", audits, err)
	}
}

func TestActivity77PeriodNewFormRequiresAudit(t *testing.T) {
	f, actor := reuseFixture(t, false)
	f.s.InstallGrowth74Resources()
	pairs, err := activity77PeriodPairs()
	if err != nil {
		t.Fatal(err)
	}
	for i, pair := range pairs {
		form := pair.old
		if i == 2 {
			form = pair.next
		}
		if _, err = f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: form.Name, Form: form, DefaultDays: 1, Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	if err = f.s.InitializeClientUpdates(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.ClientUpdatePolicy{}).Where("id=1").Update("maintenance", true).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.ApplyActivity77Periods(f.ctx, actor); err == nil || !strings.Contains(err.Error(), "缺少迁移审计") {
		t.Fatal("unaudited new form accepted", err)
	}
}
