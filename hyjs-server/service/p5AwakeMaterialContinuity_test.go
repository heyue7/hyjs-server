package service

import (
	"encoding/json"
	"testing"
	"time"

	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// 真实发布可持续到缺失的后月，冻结报价、人工停运和旧个人目录保持。
func TestP5AwakeMaterialContinuityFrozenMonthStopAndPrivateDirectory(t *testing.T) {
	f, id := localPlayActivitiesFixture(t)
	tx := db.GetIns().WithContext(f.ctx)
	zone := time.FixedZone("UTC+8", 8*3600)
	january := time.Date(2030, 1, 1, 0, 0, 0, 0, zone)
	load := func(at time.Time) db.P4CommerceRule {
		t.Helper()
		if err := f.s.seedP5AwakeMaterialSupply(tx, at); err != nil {
			t.Fatal(err)
		}
		var cfg db.P4CommerceRule
		if err := tx.First(&cfg, "id=?", constUtil.P5AwakeMaterialSupplyPrefix+at.Format("200601")).Error; err != nil {
			t.Fatal(err)
		}
		return cfg
	}
	prior := load(january)
	if prior.Enabled || len(prior.Frozen) != 0 {
		t.Fatal("unpublished330 must stay closed")
	}
	prior.Enabled = true
	wanted := append([]byte(nil), prior.Rules...)
	frozen, err := json.Marshal(prior)
	if err != nil {
		t.Fatal(err)
	}
	var edited p4CommerceRules
	if err = json.Unmarshal(prior.Rules, &edited); err != nil {
		t.Fatal(err)
	}
	edited.Products[0].Cost = "coin,2000000"
	changed, err := json.Marshal(edited)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Model(&prior).Updates(map[string]any{"enabled": true, "frozen": frozen, "rules": changed}).Error; err != nil {
		t.Fatal(err)
	}
	clone := prior
	clone.ID += "_lp-not-a-global-month"
	clone.StartAt = january.AddDate(0, 0, 1).UnixMilli()
	clone.Enabled = false
	clone.Frozen = nil
	if err = tx.Create(&clone).Error; err != nil {
		t.Fatal(err)
	}
	february := load(january.AddDate(0, 1, 0))
	if february.Enabled || string(february.Rules) != string(wanted) || len(february.Frozen) != 0 {
		t.Fatal("next month must retain frozen330 terms but stay closed")
	}
	if err = tx.Model(&february).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if load(january.AddDate(0, 1, 0)).Enabled || load(january.AddDate(0, 2, 0)).Enabled {
		t.Fatal("restart or next month reopened an operator stop")
	}
	december := load(january.AddDate(0, 11, 0))
	if err = tx.Model(&december).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if next := load(january.AddDate(1, 0, 0)); next.Enabled || next.ID != constUtil.P5AwakeMaterialSupplyPrefix+"203101" {
		t.Fatal("year boundary silently re-enabled330 publication")
	}
	// 当前真实个人目录在330发布后仍保持地址及快照，采用新源须显式刷新。
	if _, err = f.s.LocalPlayActivitiesApply(f.ctx, id, "open", "TimeLimitExchange", "33000000-0000-4000-8000-000000000001"); err != nil {
		t.Fatal(err)
	}
	now := LocalPlayNow(f.ctx)
	key, err := localPlayExchangeKey(tx, now)
	if err != nil {
		t.Fatal(err)
	}
	directory, bindings, err := f.s.p4ExchangeDirectory(tx, key, now)
	if err != nil {
		t.Fatal(err)
	}
	hash, raw := directory.InputHash, string(directory.Bindings)
	if err = tx.Model(&db.P4CommerceRule{}).Where("id=?", constUtil.P5AwakeMaterialSupplyPrefix+now.In(zone).Format("200601")).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.p5AppendAwakeMaterialBinding(tx, directory, &bindings, now); err != nil || directory.InputHash != hash || string(directory.Bindings) != raw {
		t.Fatal("late330 publication changed a frozen personal directory", err)
	}
}
