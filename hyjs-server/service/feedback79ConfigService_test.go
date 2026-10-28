package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func TestFeedback79ConfigScopeHistoryAndRetry(t *testing.T) {
	f, actor := adminNativeFixture(t, false)
	if err := db.GetIns().AutoMigrate(&db.ServerGameClock{}, &db.ClientUpdatePolicy{}, &db.AdminActivityScheduleRevision{}, &db.AdminActivityDefinition{}, &db.AdminActivityOccurrence{}, &db.AdminCharacterActivityInstance{}, &db.GameplayPool{}, &db.TurntableRules{}, &db.TurntablePersonal{}); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.AdminUser{}).Where("id=?", actor.UserID).Update("role", constUtil.AdminRoleAdmin).Error; err != nil {
		t.Fatal(err)
	}
	actor.Role = constUtil.AdminRoleAdmin
	f.s.ServerClockReady = true
	now := time.Now().UnixMilli()
	raw := []byte(`{"mode":"turntable","templateId":"turntable-original","name":"幸运大转盘","source":"成长投放V12 / 2026-10-26","options":{"scorePerDraw":1,"initialDiamondPool":0,"jackpotPerMillion":0,"poolContributionBps":0,"unknown":{"large":9007199254740993}}}`)
	occ := db.AdminActivityOccurrence{ID: "occ_79", ServerID: 1, TemplateID: "template_79", Name: "幸运大转盘", Mode: "turntable", Timing: "character-day", ConsumerKey: "character_plan_79", ConsumerID: "character-plan:character_plan_79", Enabled: true, FormJSON: raw, Revision: 1}
	tpl := db.AdminActivityDefinition{ID: occ.TemplateID, Name: occ.Name, Mode: "turntable", FormJSON: raw, Enabled: true, Usable: true, Revision: 1}
	key := "admin_turntable_character_" + p6Digest([]byte(fmt.Sprintf("1/%d/%s", f.id, occ.ID)))[:20]
	issued := db.AdminCharacterActivityInstance{PlayerID: f.id, OccurrenceID: occ.ID, ServerID: 1, ConsumerID: "pool:" + key, ConsumerKey: key, Mode: "turntable", StartAt: now - 1000, EndAt: now + 86400000, ClaimEndAt: now + 86400000, SourceVersion: "original", FormJSON: raw}
	pool := db.GameplayPool{ID: key, Source: constUtil.Feedback79GrowthSource, InitialDiamondPool: 0, StartAt: issued.StartAt, EndAt: issued.EndAt, RewardEndAt: issued.ClaimEndAt}
	personal := db.TurntablePersonal{PoolID: key, PlayerID: f.id, DiamondPool: 123, ContributionRemainder: 456, UsedStock: []byte(`{"1":3}`), CreatedAt: now - 1}
	for _, value := range []any{&db.ServerGameClock{ServerID: 1}, &db.ClientUpdatePolicy{ID: 1, Maintenance: false}, &db.AdminActivityScheduleRevision{ServerID: 1, Revision: 7}, &tpl, &occ, &issued, &pool, &personal} {
		if err := db.GetIns().Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.ApplyFeedback79Config(f.ctx, actor); err == nil {
		t.Fatal("non-maintenance apply succeeded")
	}
	if err := db.GetIns().Model(&db.ClientUpdatePolicy{}).Where("id=1").Update("maintenance", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyFeedback79Config(f.ctx, actor); err == nil {
		t.Fatal("missing pool rules accepted")
	}
	var before db.AdminActivityOccurrence
	if err := db.GetIns().First(&before, "id=?", occ.ID).Error; err != nil || !bytes.Equal(before.FormJSON, raw) {
		t.Fatal("failed apply changed schedule", err)
	}
	if err := db.GetIns().Create(&db.TurntableRules{PoolID: key, ScorePerDraw: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyFeedback79Config(f.ctx, actor); err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyFeedback79Config(f.ctx, actor); err != nil {
		t.Fatal(err)
	}
	var updated db.GameplayPool
	if err := db.GetIns().First(&updated, "id=?", key).Error; err != nil || updated.InitialDiamondPool != 0 || updated.JackpotPerMillion != 200 || updated.PoolContributionBps != 200 {
		t.Fatal("live pool", updated, err)
	}
	var updatedPersonal db.TurntablePersonal
	if err := db.GetIns().First(&updatedPersonal, "pool_id=? AND player_id=?", key, f.id).Error; err != nil || !reflect.DeepEqual(personal, updatedPersonal) {
		t.Fatal("personal balance or stock changed", err)
	}
	var afterIssued db.AdminCharacterActivityInstance
	if err := db.GetIns().First(&afterIssued, "player_id=? AND occurrence_id=?", f.id, occ.ID).Error; err != nil || !reflect.DeepEqual(issued, afterIssued) {
		t.Fatal("issued snapshot changed", err)
	}
	var afterOcc db.AdminActivityOccurrence
	var afterTpl db.AdminActivityDefinition
	if err := db.GetIns().First(&afterOcc, "id=?", occ.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&afterTpl, "id=?", tpl.ID).Error; err != nil {
		t.Fatal(err)
	}
	for _, item := range [][]byte{afterOcc.FormJSON, afterTpl.FormJSON} {
		var form model.AdminActivityForm
		if err := json.Unmarshal(item, &form); err != nil || form.Options.JackpotPerMillion != 200 || form.Options.PoolContributionBps != 200 || !bytes.Contains(item, []byte(`9007199254740993`)) {
			t.Fatal("future rule or unknown field changed", err)
		}
	}
	var rev db.AdminActivityScheduleRevision
	if err := db.GetIns().First(&rev, "server_id=?", 1).Error; err != nil || rev.Revision != 8 {
		t.Fatal("schedule revision or retry", rev, err)
	}
	form, _, _, err := f.s.adminNativeVIPPolicy(db.GetIns(), false)
	tier, _ := adminNativeVIPTierFor(form, 2)
	if err != nil || len(tier.DailyRewards) == 0 || tier.DailyRewards[0].Amount != 300000 {
		t.Fatal("membership curve was not applied", err)
	}
}
