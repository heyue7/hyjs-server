package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

func TestP1SettlementCandidateUsesFrozenDeadlineAndServerClock(t *testing.T) {
	real := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	live := db.PeriodInstance{ID: "p1:clock", EndAt: real.Add(time.Hour).UnixMilli()}
	if p1SettlementNeedsLockedCheck(context.Background(), live, real) {
		t.Fatal("future period selected")
	}
	if !p1SettlementNeedsLockedCheck(context.Background(), live, real.Add(time.Hour)) {
		t.Fatal("cutoff must be inclusive")
	}
	server := context.WithValue(context.Background(), constUtil.ServerGameClockBoundKey, true)
	server = context.WithValue(server, constUtil.ServerGameClockOffsetKey, int64(1))
	if !p1SettlementNeedsLockedCheck(server, live, real) {
		t.Fatal("server calendar advance was ignored")
	}
	server = context.WithValue(server, constUtil.ServerGameClockOffsetKey, int64(-1))
	if p1SettlementNeedsLockedCheck(server, live, real.Add(time.Hour)) {
		t.Fatal("server calendar rollback was ignored")
	}

	frozen := live
	frozen.EndAt = real.Add(-time.Hour).UnixMilli()
	raw, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	live.Frozen = raw
	if !p1SettlementNeedsLockedCheck(context.Background(), live, real) {
		t.Fatal("frozen cutoff was ignored")
	}
	live.Frozen = []byte(`{"id":"another-period"}`)
	if !p1SettlementNeedsLockedCheck(context.Background(), live, real) {
		t.Fatal("damaged frozen identity must reach locked validation")
	}
	live.ID = "p1:lp-" + strings.Repeat("a", 26)
	live.Frozen = nil
	if !p1SettlementNeedsLockedCheck(context.Background(), live, real) {
		t.Fatal("private calendar must reach locked validation")
	}
}

func TestP1SettlementCandidatesDataAndBoundedReads(t *testing.T) {
	f := p1Setup(t)
	database := db.GetIns()
	if err := database.AutoMigrate(&db.LocalPlayActivityCycle{}, &db.LocalPlayClockState{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	makePeriod := func(id string, end time.Time) db.PeriodInstance {
		return db.PeriodInstance{
			ID: id, Enabled: true, Title: "settlement test", Kind: "candidate-test", Mode: "cumulative",
			Goal: 100, Rewards: "coin,17", StartAt: now.Add(-7 * 24 * time.Hour).UnixMilli(),
			EndAt: end.UnixMilli(), RewardEndAt: end.Add(48 * time.Hour).UnixMilli(),
			RankRewards: "coin,13", Source: "isolated test",
		}
	}
	periods := make([]db.PeriodInstance, 0, 726)
	for i := 0; i < 520; i++ {
		periods = append(periods, makePeriod(fmt.Sprintf("proof:future:%04d", i), now.Add(24*time.Hour)))
	}
	var oldSettlements []db.PeriodSettlement
	for i := 0; i < 200; i++ {
		period := makePeriod(fmt.Sprintf("proof:history:%04d", i), now.Add(-time.Hour))
		periods = append(periods, period)
		oldSettlements = append(oldSettlements, db.PeriodSettlement{InstanceID: period.ID, SettledAt: now.Add(-time.Minute).UnixMilli(), Snapshot: []byte(`{"original":true}`)})
	}
	frozenDue := makePeriod("proof:y-frozen-due", now.Add(24*time.Hour))
	frozenRules := frozenDue
	frozenRules.EndAt = now.Add(-time.Hour).UnixMilli()
	frozenRules.RewardEndAt = now.Add(24 * time.Hour).UnixMilli()
	frozenDue.RankPlaces = 1
	// Ranking is read from the frozen rules, including the historical cutoff.
	frozenRules.RankPlaces = 1
	frozenDue.Frozen, _ = json.Marshal(frozenRules)
	frozenFuture := makePeriod("proof:y-frozen-future", now.Add(-time.Hour))
	futureRules := frozenFuture
	futureRules.EndAt = now.Add(24 * time.Hour).UnixMilli()
	futureRules.RewardEndAt = now.Add(48 * time.Hour).UnixMilli()
	frozenFuture.Frozen, _ = json.Marshal(futureRules)
	disabled := makePeriod("proof:y-disabled", now.Add(-time.Hour))
	disabled.Enabled, disabled.RankPlaces = false, 1
	due := makePeriod("proof:z-due", now.Add(-time.Hour))
	positiveCycle := "lp-" + strings.Repeat("a", 26)
	negativeCycle := "lp-" + strings.Repeat("b", 26)
	positive := makePeriod("proof:"+positiveCycle, now.Add(24*time.Hour))
	negative := makePeriod("proof:"+negativeCycle, now.Add(-24*time.Hour))
	periods = append(periods, frozenDue, frozenFuture, disabled, due, positive, negative)
	if err := database.CreateInBatches(periods, 50).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.CreateInBatches(oldSettlements, 50).Error; err != nil {
		t.Fatal(err)
	}
	cycles := []db.LocalPlayActivityCycle{
		{ID: positiveCycle, PlayerID: 201, ActivityID: "test", Snapshot: []byte(`{}`), Source: constUtil.LocalPlayActivitiesVersion},
		{ID: negativeCycle, PlayerID: 202, ActivityID: "test", Snapshot: []byte(`{}`), Source: constUtil.LocalPlayActivitiesVersion},
	}
	if err := database.Create(&cycles).Error; err != nil {
		t.Fatal(err)
	}
	clocks := []db.LocalPlayClockState{{PlayerID: 201, OffsetDays: 2}, {PlayerID: 202, OffsetDays: -2}}
	if err := database.Create(&clocks).Error; err != nil {
		t.Fatal(err)
	}
	progress := []db.PeriodProgress{
		{InstanceID: frozenDue.ID, PlayerID: f.id, Value: 3, Snapshot: []byte(`{}`)},
		{InstanceID: disabled.ID, PlayerID: f.id, Value: 3, Snapshot: []byte(`{}`)},
	}
	if err := database.Create(&progress).Error; err != nil {
		t.Fatal(err)
	}

	var lockedReads, pageReads atomic.Int64
	const callback = "test:p1-settlement-candidate-reads"
	if err := database.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		query := tx.Statement.SQL.String()
		if strings.Contains(query, "LEFT JOIN p1_period_settlements AS settled") && strings.Contains(query, "settled.instance_id IS NULL") {
			pageReads.Add(1)
		}
		if tx.Statement.Table == "p1_period_instances" && strings.Contains(query, "id = ?") && len(tx.Statement.Vars) > 0 {
			if id, ok := tx.Statement.Vars[0].(string); ok && strings.HasPrefix(id, "proof:") {
				lockedReads.Add(1)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Callback().Query().Remove(callback) })
	if err := f.s.SettleP1Periods(f.ctx); err != nil {
		t.Fatal(err)
	}
	if got := lockedReads.Load(); got != 5 {
		t.Fatalf("locked reads = %d, want 5 for due and private periods", got)
	}
	if got := pageReads.Load(); got < 3 {
		t.Fatalf("candidate pages = %d, want at least 3", got)
	}
	t.Logf("proof periods=%d; baseline loop predicts %d locked reads; observed candidate locked reads=%d; page queries=%d", len(periods), len(periods), lockedReads.Load(), pageReads.Load())
	for _, id := range []string{frozenDue.ID, disabled.ID, due.ID, positive.ID} {
		if p1Count(t, &db.PeriodSettlement{}, "instance_id=?", id) != 1 {
			t.Fatalf("missing settlement %s", id)
		}
	}
	for _, id := range []string{frozenFuture.ID, negative.ID} {
		if p1Count(t, &db.PeriodSettlement{}, "instance_id=?", id) != 0 {
			t.Fatalf("settled early: %s", id)
		}
	}
	var saved db.PeriodSettlement
	if err := database.First(&saved, "instance_id=?", frozenDue.ID).Error; err != nil {
		t.Fatal(err)
	}
	var snapshot p1Settlement
	if err := json.Unmarshal(saved.Snapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Rules.EndAt != frozenRules.EndAt || len(snapshot.Ranks) != 1 || snapshot.Ranks[0].PlayerID != f.id {
		t.Fatalf("frozen settlement changed: %+v", snapshot)
	}
	if p1Count(t, &db.RewardDelivery{}, "source_key=?", "rank:"+frozenDue.ID) != 1 || p1Count(t, &db.RewardDelivery{}, "source_key=?", "rank:"+disabled.ID) != 0 {
		t.Fatal("enabled and disabled rank delivery behavior changed")
	}
	var historical db.PeriodSettlement
	if err := database.First(&historical, "instance_id=?", oldSettlements[0].InstanceID).Error; err != nil {
		t.Fatal(err)
	}
	if string(historical.Snapshot) != string(oldSettlements[0].Snapshot) {
		t.Fatal("existing settlement was rewritten")
	}

	// A candidate may change after a scan. The authoritative locked read must
	// observe the current deadline rather than the stale candidate's deadline.
	if err := f.s.settleP1Period(f.ctx, frozenFuture.ID); err != nil {
		t.Fatal(err)
	}
	if p1Count(t, &db.PeriodSettlement{}, "instance_id=?", frozenFuture.ID) != 0 {
		t.Fatal("locked recheck settled a future frozen deadline")
	}
}

func TestP1SettlementPrivateClockInvalidThenRecovery(t *testing.T) {
	f := p1Setup(t)
	database := db.GetIns()
	if err := database.AutoMigrate(&db.LocalPlayActivityCycle{}, &db.LocalPlayClockState{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	cycle := "lp-" + strings.Repeat("c", 26)
	period := p1Instance(t, f, "proof:"+cycle, "private-clock", "cumulative", 100, now.Add(24*time.Hour), 0)
	if err := database.Create(&db.LocalPlayActivityCycle{ID: cycle, PlayerID: 303, ActivityID: "test", Snapshot: []byte(`{}`), Source: constUtil.LocalPlayActivitiesVersion}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&db.LocalPlayClockState{PlayerID: 303, OffsetDays: 3651}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.SettleP1Periods(f.ctx); err == nil {
		t.Fatal("invalid private clock was silently accepted")
	}
	if p1Count(t, &db.PeriodSettlement{}, "instance_id=?", period.ID) != 0 {
		t.Fatal("invalid private clock settled a period")
	}
	if err := database.Model(&db.LocalPlayClockState{}).Where("player_id=?", 303).Update("offset_days", 2).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.SettleP1Periods(f.ctx); err != nil {
		t.Fatal(err)
	}
	if p1Count(t, &db.PeriodSettlement{}, "instance_id=?", period.ID) != 1 {
		t.Fatal("corrected private clock did not recover due settlement")
	}
}

func TestP1SettlementRechecksConcurrentDeadlineChange(t *testing.T) {
	f := p1Setup(t)
	database := db.GetIns()
	now := time.Now()
	period := p1Instance(t, f, "proof:concurrent", "candidate-change", "cumulative", 100, now.Add(-time.Hour), 0)
	ctx, cancel := context.WithTimeout(f.ctx, 10*time.Second)
	defer cancel()
	scanned := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	const callback = "test:p1-settlement-concurrent-change"
	if err := database.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "LEFT JOIN p1_period_settlements AS settled") && strings.Contains(tx.Statement.SQL.String(), "settled.instance_id IS NULL") {
			once.Do(func() {
				close(scanned)
				select {
				case <-release:
				case <-ctx.Done():
				}
			})
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Callback().Query().Remove(callback) })
	finished := make(chan error, 1)
	go func() { finished <- f.s.SettleP1Periods(ctx) }()
	select {
	case <-scanned:
	case <-ctx.Done():
		t.Fatal("candidate scan did not complete")
	}
	if err := database.WithContext(ctx).Model(&db.PeriodInstance{}).Where("id=?", period.ID).
		Updates(map[string]any{"end_at": now.Add(time.Hour).UnixMilli(), "reward_end_at": now.Add(2 * time.Hour).UnixMilli()}).Error; err != nil {
		close(release)
		t.Fatal(err)
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if p1Count(t, &db.PeriodSettlement{}, "instance_id=?", period.ID) != 0 {
		t.Fatal("stale candidate settled an extended activity")
	}
	if err := database.Model(&db.PeriodInstance{}).Where("id=?", period.ID).
		Updates(map[string]any{"end_at": period.EndAt, "reward_end_at": period.RewardEndAt}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.SettleP1Periods(f.ctx); err != nil {
		t.Fatal(err)
	}
	if p1Count(t, &db.PeriodSettlement{}, "instance_id=?", period.ID) != 1 {
		t.Fatal("next scan did not recover the due period")
	}
}
