package service

import (
	"testing"
	"time"

	"hyjs-server/db"
)

// TestTurntableConfigurationClosedDraftAndInvalidRanking validates closed drafts without weakening original draw guards.
func TestTurntableConfigurationClosedDraftAndInvalidRanking(t *testing.T) {
	s := &GameService{}
	now := time.Now().UnixMilli()
	cfg := db.GameplayPool{ID: "closed-configuration", Enabled: false, MinLevel: 1, SingleCost: "diamond,50", TenCost: "diamond,488", CoinPerDraw: 10000, FreeSeconds: 86400, StartAt: now - 1000, EndAt: now + 3600000, RewardEndAt: now + 7200000}
	rewards := make([]db.GameplayPoolReward, 9)
	for i := range rewards {
		rewards[i] = db.GameplayPoolReward{PoolID: cfg.ID, ID: int32(i), ResourceID: "coin", Amount: 1, Weight: 1}
	}
	rules := db.TurntableRules{PoolID: cfg.ID, ScorePerDraw: 1, TopPrize: "diamond,10", AdvancePrize: "diamond,1", AdvancePlaces: 10}
	if err := s.validateTurntableConfiguration(&cfg, rewards, &rules); err != nil {
		t.Fatal("valid closed original configuration was marked unusable", err)
	}
	invalid := rules
	invalid.TopPrize = "equip_unknown,1"
	if err := s.validateTurntableConfiguration(&cfg, rewards, &invalid); err == nil {
		t.Fatal("closed invalid ranking reward was marked usable")
	}
	invalidWeights := append([]db.GameplayPoolReward(nil), rewards...)
	invalidWeights[0].Weight = 0
	if err := s.validateTurntableConfiguration(&cfg, invalidWeights, &rules); err == nil {
		t.Fatal("pure draft validation omitted original pool weight guards")
	}
	invalidPrice := cfg
	invalidPrice.SingleCost = "diamond,51"
	if err := s.validateTurntableConfiguration(&invalidPrice, rewards, &rules); err == nil {
		t.Fatal("closed draft bypassed original client price guard")
	}
}
