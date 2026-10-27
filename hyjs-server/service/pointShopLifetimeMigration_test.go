package service

import (
	"encoding/json"
	"testing"

	"hyjs-server/db"
)

func TestPointLifetimeImportsAllScheduledHistoryAndRejectsMissingTotal(t *testing.T) {
	f := p4Setup(t)
	var source db.P4CommerceRule
	if err := db.GetIns().Where("kind=?", "point").First(&source).Error; err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"shop_schedule_archived_trial_a", "shop_schedule_archived_trial_b"} {
		cfg := source
		cfg.ID = id
		cfg.Enabled = false
		cfg.EndAt = 1
		cfg.ClaimEndAt = 1
		if err := db.GetIns().Create(&cfg).Error; err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(p4CommerceProgress{Total: map[string]int32{"stableTrialProduct": int32(i + 1)}})
		if err := db.GetIns().Create(&db.P4CommerceState{PlayerID: f.id, RuleID: id, Payload: raw}).Error; err != nil {
			t.Fatal(err)
		}
	}
	_, state, _, err := loadPointShopLifetime(db.GetIns(), f.id)
	if err != nil || state.Total["stableTrialProduct"] != 3 {
		t.Fatal("scheduled history not merged", state, err)
	}
	_, again, _, err := loadPointShopLifetime(db.GetIns(), f.id)
	if err != nil || again.Total["stableTrialProduct"] != 3 {
		t.Fatal("history counted twice", again, err)
	}
	// Importing a legacy positive daily count without Total must fail closed.
	other := f.id + 999
	raw, _ := json.Marshal(p4CommerceProgress{Bought: map[string]int32{"stableTrialProduct": 1}})
	if err := db.GetIns().Create(&db.P4CommerceState{PlayerID: other, RuleID: source.ID, Payload: raw}).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := loadPointShopLifetime(db.GetIns(), other); err == nil {
		t.Fatal("missing historic totals silently became zero")
	}
	var count int64
	if err := db.GetIns().Model(&db.P4CommerceState{}).Where("player_id=? AND rule_id=?", other, "p4:point:lifetime-v1").Count(&count).Error; err != nil || count != 0 {
		t.Fatal("invalid history created fixed lifetime state", count, err)
	}
}
