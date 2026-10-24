package service

import (
	"encoding/json"
	"testing"
	"time"

	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// Frozen kind selects the consumer before its source SQL; matching consumers
// still reject an invalid source rather than silently accepting an event.
func TestActivityHotPath63FrozenKindKeepsSourceValidation(t *testing.T) {
	f := p4Setup(t)
	now := time.Now()
	live := db.PeriodInstance{ID: "p1_hot63", Enabled: true, Kind: constUtil.P4KindPower, Mode: "single", Goal: 1, Rewards: "coin,1", StartAt: now.Add(-time.Hour).UnixMilli(), EndAt: now.Add(time.Hour).UnixMilli(), RewardEndAt: now.Add(time.Hour).UnixMilli(), RewardSourceID: "missing-source63"}
	frozen := live
	frozen.Kind = constUtil.P1EventLoginDay
	raw, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	live.Frozen = raw
	tx := db.GetIns().WithContext(f.ctx)
	if err = tx.Create(&live).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.recordP4PowerEntry(tx, f.id, 100, now); err != nil {
		t.Fatalf("unrelated frozen login source queried by power: %v", err)
	}
	frozen.Kind = constUtil.P4KindPower
	raw, err = json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Model(&live).Update("frozen", raw).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.recordP4PowerEntry(tx, f.id, 100, now); err == nil {
		t.Fatal("matching frozen power bypassed source validation")
	}
}
