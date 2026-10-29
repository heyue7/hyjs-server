package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func trial82EconomyFixture(t *testing.T) (*p1Fixture, *model.AdminPrincipal, db.P4CommerceRule) {
	t.Helper()
	f, actor := reuseFixture(t, false)
	if err := f.s.InitializeClientUpdates(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Save(&db.ClientUpdatePolicy{ID: 1, Maintenance: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.LoadP7Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().AutoMigrate(&db.P7RushStageRule{}); err != nil {
		t.Fatal(err)
	}
	stages, err := f.s.p7ReRushCatalogue(true)
	if err != nil {
		t.Fatal(err)
	}
	for i := range stages {
		p := &stages[i]
		if p.Kind == constUtil.P7RushTrial {
			p.Reward = fmt.Sprintf("coin,%d;point,%d", (2000+p.Floor*500)*p.Star, (50+p.Floor*10)*p.Star)
		}
	}
	if err = db.GetIns().CreateInBatches(&stages, 100).Error; err != nil {
		t.Fatal(err)
	}
	var source db.P4CommerceRule
	if err = db.GetIns().Where("kind=? AND id NOT LIKE ?", "point", "shop_schedule_%").First(&source).Error; err != nil {
		t.Fatal(err)
	}
	var plan growth74Plan
	if err = json.Unmarshal(growth74Parameters, &plan); err != nil {
		t.Fatal(err)
	}
	rules := p4CommerceRules{LiveEditable: true}
	for i, spec := range plan.Stores["trial"].Products {
		p := p4BaseProduct(fmt.Sprintf("trial82_fixture_%d", i), fmt.Sprintf("%s,%d", spec.Reward, spec.Quantity), fmt.Sprintf("point,%d", spec.Price), int32(i+1), max(22, int32(spec.MinimumPlayerLevel)))
		p.Priority, p.MinTrialStars = int32(spec.Menu*1000)+int32(i), 30
		if spec.Menu == 4 {
			p.TotalLimit = 1
		}
		rules.Products = append(rules.Products, p)
	}
	source.Rules = adminJSON(rules)
	source.Frozen = adminJSON(source)
	if err = db.GetIns().Save(&source).Error; err != nil {
		t.Fatal(err)
	}
	return f, actor, source
}

func TestTrial82EconomyMigrationPreservesAssetsHistoryAndRetries(t *testing.T) {
	f, actor, source := trial82EconomyFixture(t)
	var beforeProfile db.Profile
	if err := db.GetIns().First(&beforeProfile, f.id).Error; err != nil {
		t.Fatal(err)
	}
	history := db.P4CommerceState{PlayerID: f.id, RuleID: "p4:point:lifetime-v1", Payload: []byte(`{"total":{"trial82_fixture_8":1,"trial82_fixture_12":1,"trial82_fixture_17":1}}`)}
	if err := db.GetIns().Create(&history).Error; err != nil {
		t.Fatal(err)
	}
	var otherBefore []db.P4CommerceRule
	if err := db.GetIns().Where("kind<>?", "point").Order("id").Find(&otherBefore).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyTrial82Config(f.ctx, actor); err != nil {
		t.Fatal(err)
	}
	var after db.P4CommerceRule
	if err := db.GetIns().First(&after, "id=?", source.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after.Frozen, source.Frozen) {
		t.Fatal("historical source frozen bytes changed")
	}
	var rules p4CommerceRules
	if err := json.Unmarshal(after.Rules, &rules); err != nil {
		t.Fatal(err)
	}
	if rules.TrialRefreshVersion != 1 || rules.ResourceRefreshCap != 15 || rules.Slots != 6 {
		t.Fatal("refresh settings", rules)
	}
	var weight, sacredWeight int64
	for _, p := range rules.Products {
		r, err := p1RewardResources(p.Rewards)
		if err != nil {
			t.Fatal(err)
		}
		group := f.s.trialRefresh82Priority(p.Priority, r) / 1000
		if group == 1 {
			weight += p.Weight
		}
		if p.Rewards == "frag_equip_037,5" || p.Rewards == "frag_equip_038,5" || p.Rewards == "frag_equip_039,5" || p.Rewards == "frag_equip_040,5" {
			if group != 1 || p.Cost != "point,6000" || p.Weight != 11 || p.TotalLimit != 0 || p.DailyLimit != 1 {
				t.Fatal("sacred offer", p)
			}
			sacredWeight += p.Weight
		}
	}
	if sacredWeight != 44 || weight != 220 {
		t.Fatal("weighted availability", sacredWeight, weight)
	}
	if err := f.s.ApplyTrial82Config(f.ctx, actor); err != nil {
		t.Fatal("retry", err)
	}
	var again db.P4CommerceRule
	var afterProfile db.Profile
	var afterHistory db.P4CommerceState
	var otherAfter []db.P4CommerceRule
	if err := db.GetIns().First(&again, "id=?", source.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&afterProfile, f.id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&afterHistory, "player_id=? AND rule_id=?", f.id, history.RuleID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Where("kind<>?", "point").Order("id").Find(&otherAfter).Error; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(again.Rules, after.Rules) || !reflect.DeepEqual(beforeProfile, afterProfile) || !bytes.Equal(history.Payload, afterHistory.Payload) || !reflect.DeepEqual(otherBefore, otherAfter) {
		t.Fatal("retry or unrelated history changed")
	}
	var audits int64
	if err := db.GetIns().Model(&db.AdminAudit{}).Where("action=?", constUtil.Trial82AuditAction).Count(&audits).Error; err != nil || audits != 1 {
		t.Fatal("audit retry", audits, err)
	}
}

func TestTrial82EconomyRejectsDriftAndNonMaintenanceAtomically(t *testing.T) {
	f, actor, source := trial82EconomyFixture(t)
	if err := db.GetIns().Model(&db.ClientUpdatePolicy{}).Where("id=?", 1).Update("maintenance", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyTrial82Config(f.ctx, actor); err == nil {
		t.Fatal("open server accepted")
	}
	if err := db.GetIns().Model(&db.ClientUpdatePolicy{}).Where("id=?", 1).Update("maintenance", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P7RushStageRule{}).Where("id=?", "re-rush:99:3").Update("reward", "point,99999").Error; err != nil {
		t.Fatal(err)
	}
	var before []db.P7RushStageRule
	if err := db.GetIns().Order("id").Find(&before).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyTrial82Config(f.ctx, actor); err == nil {
		t.Fatal("unreviewed reward overwritten")
	}
	var after db.P4CommerceRule
	var afterStages []db.P7RushStageRule
	if err := db.GetIns().First(&after, "id=?", source.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Order("id").Find(&afterStages).Error; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after.Rules, source.Rules) || !reflect.DeepEqual(before, afterStages) {
		t.Fatal("partial migration persisted")
	}
}

func TestTrial82PointBudgetAndFrozenProjection(t *testing.T) {
	var total int32
	for floor := int32(1); floor <= 102; floor++ {
		total += trial82FloorPoints(floor, 3)
	}
	total += 34 * 9 * constUtil.Trial82SectionStarPoints
	if total != 21879 {
		t.Fatal("full run point budget", total)
	}
	for _, tc := range []struct{ floor, star, want int32 }{{1, 1, 11}, {1, 3, 33}, {21, 3, 93}, {60, 3, 210}, {102, 3, 336}} {
		if got := trial82FloorPoints(tc.floor, tc.star); got != tc.want {
			t.Fatal(tc, got)
		}
	}
	old := db.P7RushStageRule{Floor: 1, Star: 3, Reward: "coin,7500;point,180"}
	if p7ReRushTaskProjection(old).GetPoint() != 180 {
		t.Fatal("frozen old rule projection repriced")
	}
	old.Reward = "coin,7500;point,33"
	if p7ReRushTaskProjection(old).GetPoint() != 33 {
		t.Fatal("published point projection wrong")
	}
}
