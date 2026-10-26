package service

import (
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"strings"
	"testing"
	"time"
)

func TestP13ItemBudgetCampQuotaBombFamilyRollbackAndLegacy(t *testing.T) {
	f, _ := p13GuildSetup(t)
	if err := f.s.InitializeP13UnionFightItemBudget(f.ctx); err != nil {
		t.Fatal(err)
	}
	match := p6Digest([]byte(t.Name()))
	t.Cleanup(func() {
		db.GetIns().Where("match_id=?", match).Delete(&db.P13UnionFightItemTeamUse{})
		db.GetIns().Where("match_id=?", match).Delete(&db.P13UnionFightItemTargetCooldown{})
	})
	small := p13UFItemBudgetConfig{constUtil.P13UFItemBudgetRule, 2, 10000, constUtil.P13UFItemBudgetBomb}
	large := small
	large.Limit = 1
	smallHash, largeHash := strings.Repeat("a", 64), strings.Repeat("b", 64)
	use := func(camp int64, item string, target int64, hash string, cfg p13UFItemBudgetConfig, at int64, rollback bool) error {
		return db.GetIns().Transaction(func(tx *gorm.DB) error {
			var gate db.P13GuildPolicy
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
				return err
			}
			if err := p13UFItemBudgetConsume(tx, match, camp, item, target, hash, cfg, time.UnixMilli(at)); err != nil {
				return err
			}
			if rollback {
				return errors.New("explicit later stock/effect failure")
			}
			return nil
		})
	}
	if err := use(0, "ufitem_017", 101, smallHash, small, 1000, false); err != nil {
		t.Fatal(err)
	}
	if err := use(0, "ufitem_018", 101, largeHash, large, 6000, false); err == nil {
		t.Fatal("large bomb bypassed small bomb target cooldown")
	}
	var count int64
	db.GetIns().Model(&db.P13UnionFightItemTeamUse{}).Where("match_id=? AND item_id=?", match, "ufitem_018").Count(&count)
	if count != 0 {
		t.Fatal("failed target use spent large-bomb quota")
	}
	if err := use(0, "ufitem_018", 101, largeHash, large, 11000, true); err == nil {
		t.Fatal("rollback fixture")
	}
	if err := use(0, "ufitem_018", 101, largeHash, large, 11000, false); err != nil {
		t.Fatal("rollback retained budget/CD", err)
	}
	if err := use(0, "ufitem_017", 102, smallHash, small, 11000, false); err != nil {
		t.Fatal("second camp0 use", err)
	}
	if err := use(0, "ufitem_017", 103, smallHash, small, 21000, false); err == nil {
		t.Fatal("camp0 quota exceeded")
	}
	if err := use(1, "ufitem_017", 104, smallHash, small, 21000, false); err != nil {
		t.Fatal("camp1 quota contaminated by camp0", err)
	}
	var used db.P13UnionFightItemTeamUse
	if err := db.GetIns().First(&used, "match_id=? AND camp=? AND item_id=?", match, 0, "ufitem_017").Error; err != nil || used.Used != 2 || used.SourceHash != p13UFItemTeamUseHash(&used) {
		t.Fatal("camp0 counter/hash", used.Used, err)
	}
	db.GetIns().Model(&db.P13UnionFightItemTargetCooldown{}).Where("match_id=? AND target_guid=?", match, 103).Count(&count)
	if count != 0 {
		t.Fatal("exhausted request reserved target clock")
	}
	if err := p13UFItemBudgetConsume(nil, "", 0, "ufitem_017", 0, "", p13UFItemBudgetConfig{}, time.Time{}); err != nil {
		t.Fatal("legacy zero fields acquired new behavior", err)
	}
	bad := small
	bad.Rule = "unknown-budget"
	if err := use(0, "ufitem_017", 105, smallHash, bad, 31000, false); err == nil {
		t.Fatal("unknown rule accepted")
	}
}
func TestP13ItemBudgetRepairHammerFamiliesAndActualGenerationGUID(t *testing.T) {
	f, _ := p13GuildSetup(t)
	_ = f
	match := p6Digest([]byte(t.Name()))
	t.Cleanup(func() {
		db.GetIns().Where("match_id=?", match).Delete(&db.P13UnionFightItemTeamUse{})
		db.GetIns().Where("match_id=?", match).Delete(&db.P13UnionFightItemTargetCooldown{})
	})
	hash := strings.Repeat("c", 64)
	run := func(camp int64, item string, target int64, cfg p13UFItemBudgetConfig) error {
		return db.GetIns().Transaction(func(tx *gorm.DB) error {
			var gate db.P13GuildPolicy
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
				return err
			}
			return p13UFItemBudgetConsume(tx, match, camp, item, target, hash, cfg, time.UnixMilli(1000))
		})
	}
	if err := run(0, "ufitem_019", 91, p13UFItemBudgetConfig{constUtil.P13UFItemBudgetRule, 2, 10000, constUtil.P13UFItemBudgetRepair}); err != nil {
		t.Fatal(err)
	}
	if err := run(1, "ufitem_020", 91, p13UFItemBudgetConfig{constUtil.P13UFItemBudgetRule, 4, 10000, constUtil.P13UFItemBudgetHammer}); err != nil {
		t.Fatal("repair and hammer must have distinct clocks", err)
	}
	factory := &db.P13UnionFightFactoryState{MatchID: match, GUID: p13UFFieldGUID(match, "factory", "1")}
	first := p13UFCarInstanceGUID(factory)
	factory.CarGeneration = 2
	second := p13UFCarInstanceGUID(factory)
	if first == second {
		t.Fatal("actual generation source must have independent GUID")
	}
	bomb := p13UFItemBudgetConfig{constUtil.P13UFItemBudgetRule, 4, 10000, constUtil.P13UFItemBudgetBomb}
	if err := run(0, "ufitem_017", first, bomb); err != nil {
		t.Fatal(err)
	}
	if err := run(0, "ufitem_017", second, bomb); err != nil {
		t.Fatal("new real generation inherited predecessor CD", err)
	}
}

func TestP13ItemBudgetLegacyOfferHashAndStrictRule(t *testing.T) {
	car := db.P13UnionFightCarItemOffer{MatchID: strings.Repeat("a", 64), ItemID: "ufitem_017", HPPercent: 20, Radius: 5, CooldownMillis: 5000, Source: "legacy-budget-fixture"}
	gate := db.P13UnionFightGateItemOffer{MatchID: car.MatchID, ItemID: "ufitem_020", HPPercent: 20, Enemy: true, Radius: 5, CooldownMillis: 5000, Source: car.Source}
	if p13UFCarItemOfferHash(&car) != "4fdb6cf9ab3264ccae4eb4b2628b8637ee41b14820f7d7f02db268c99387d41c" || p13UFGateItemOfferHash(&gate) != "86389f20d49bdbbabd2ab2dd4004d0ebc588a9d47e957be3bc1f31209b210059" {
		t.Fatal("zero optional budget fields changed legacy quote hashes")
	}
	for _, cfg := range []p13UFItemBudgetConfig{{Limit: 4}, {Rule: "unknown", Limit: 4, TargetCooldownMillis: 10000, Family: "bomb"}, {Rule: constUtil.P13UFItemBudgetRule, Limit: 4, TargetCooldownMillis: 9999, Family: "bomb"}, {Rule: constUtil.P13UFItemBudgetRule, Limit: 4, TargetCooldownMillis: 10000, Family: "repair"}} {
		if p13UFItemBudgetValidate("ufitem_017", cfg) == nil {
			t.Fatal("unpublished/mismatched rule accepted", cfg)
		}
	}
}
