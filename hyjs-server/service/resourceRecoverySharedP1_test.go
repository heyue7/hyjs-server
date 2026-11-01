package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// recoverySharedTrace records lock mode through the transaction context even
// on SQLite, which does not emit MySQL locking clauses.
type recoverySharedTrace struct {
	logger.Interface
	mu        sync.Mutex
	shared    int
	exclusive int
}

func (r *recoverySharedTrace) Trace(ctx context.Context, _ time.Time, statement func() (string, int64), _ error) {
	query, _ := statement()
	if !strings.Contains(strings.ToLower(query), "p1_period_instances") {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if ctx.Value(constUtil.P1SharedReadContextKey{}) == true {
		r.shared++
	} else {
		r.exclusive++
	}
}

func (r *recoverySharedTrace) counts() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.shared, r.exclusive
}

// The isolated fixture sets one due energy cycle without invoking save hooks.
func recoverySharedSetDue(t *testing.T, id int64) {
	t.Helper()
	var row db.Profile
	if err := db.GetIns().First(&row, id).Error; err != nil {
		t.Fatal(err)
	}
	player, _, err := decodeProfile(&row)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	player.Energy = proto.Int32(10)
	player.EnergyLastRefreshTime = proto.Int64(now - constUtil.EnergyRecoveryIntervalMS - 10000)
	player.Stamina = proto.Int32(constUtil.StaminaRecoveryLimit)
	player.StaminaLastRefreshTime = proto.Int64(now)
	raw, err := json.Marshal(player)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.Profile{}).Where("id=?", id).UpdateColumn("player", string(raw)).Error; err != nil {
		t.Fatal(err)
	}
}

// A first P4 collection fact forces a complete rollback and the original
// exclusive retry; resource recovery and activity progress still commit once.
func TestResourceProfileSharedP1Retry(t *testing.T) {
	f := p4Setup(t)
	ctx := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	period := p1Instance(t, f, "recovery_shared_collection", constUtil.P4KindCollect, "cumulative", 99, time.Now().Add(time.Hour), 0)
	recoverySharedSetDue(t, f.id)
	database := db.GetIns()
	previous := database.Config.Logger
	trace := &recoverySharedTrace{Interface: previous}
	database.Config.Logger = trace
	defer func() { database.Config.Logger = previous }()

	player, _, err := f.s.resourceProfile(ctx, f.id)
	if err != nil || player == nil || player.GetEnergy() != 11 {
		t.Fatalf("recovery failed or applied twice: %v, %v", player, err)
	}
	shared, exclusive := trace.counts()
	if shared == 0 || exclusive == 0 {
		t.Fatalf("expected shared attempt and exclusive retry: shared=%d exclusive=%d", shared, exclusive)
	}
	facts := p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P4KindCollect)
	if facts != 1 || p1ProgressValue(t, f, period.ID).Value != 1 {
		t.Fatalf("collection fact or progress was lost or duplicated: facts=%d", facts)
	}
	stored, _ := f.profile(t)
	if stored.GetEnergy() != player.GetEnergy() || stored.GetEnergyLastRefreshTime() != player.GetEnergyLastRefreshTime() {
		t.Fatal("returned recovery differs from the committed profile")
	}
}

// InnoDB is required to verify that a recovery with no new facts can finish
// while another transaction holds shared P1 window locks.
func TestResourceProfileSharedP1RealMySQL(t *testing.T) {
	if os.Getenv("HYJS_P4_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p4_ MySQL database")
	}
	f := p4Setup(t)
	ctx, cancel := context.WithTimeout(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), 20*time.Second)
	defer cancel()
	// Establish the normal P4 facts before testing the unchanged save hook.
	recoverySharedSetDue(t, f.id)
	if _, _, err := f.s.resourceProfile(ctx, f.id); err != nil {
		t.Fatal(err)
	}
	recoverySharedSetDue(t, f.id)
	database := db.GetIns()
	previous := database.Config.Logger
	trace := &loginPeriodLockTrace{Interface: previous}
	database.Config.Logger = trace
	defer func() { database.Config.Logger = previous }()

	acquired := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	holder := make(chan error, 1)
	go func() {
		shared := context.WithValue(ctx, constUtil.P1SharedReadContextKey{}, true)
		holder <- database.WithContext(shared).Transaction(func(tx *gorm.DB) error {
			if _, err := p1LockPeriods(tx); err != nil {
				return err
			}
			close(acquired)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	select {
	case <-acquired:
	case err := <-holder:
		t.Fatalf("shared holder failed: %v", err)
	case <-ctx.Done():
		t.Fatal("shared holder timed out")
	}
	trace.shared.Store(0)
	trace.exclusive.Store(0)
	done := make(chan error, 1)
	go func() {
		player, _, err := f.s.resourceProfile(ctx, f.id)
		if err == nil && (player == nil || player.GetEnergy() != 11) {
			err = constUtil.ErrAdminConfiguration
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("recovery failed while shared lock was held: %v", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("no-fact recovery blocked behind an unrelated shared reader")
	}
	if trace.shared.Load() == 0 || trace.exclusive.Load() != 0 {
		t.Fatalf("expected only shared activity reads: shared=%d exclusive=%d", trace.shared.Load(), trace.exclusive.Load())
	}
	releaseOnce.Do(func() { close(release) })
	if err := <-holder; err != nil {
		t.Fatal(err)
	}
}

type recoveryComparisonTrace struct {
	logger.Interface
	total     atomic.Int64
	shared    atomic.Int64
	exclusive atomic.Int64
}

func (r *recoveryComparisonTrace) Trace(_ context.Context, _ time.Time, statement func() (string, int64), _ error) {
	query, _ := statement()
	r.total.Add(1)
	query = strings.ToLower(query)
	if !strings.Contains(query, "p1_period_instances") {
		return
	}
	if strings.Contains(query, "for share") {
		r.shared.Add(1)
	}
	if strings.Contains(query, "for update") {
		r.exclusive.Add(1)
	}
}

// Run unchanged against baseline and candidate with an identical fixed holder,
// due recovery and 700 unrelated future windows. It reports SQL and latency;
// the separate shared-reader test establishes the candidate's expected mode.
func TestResourceProfileContentionComparisonMySQL(t *testing.T) {
	if os.Getenv("HYJS_P4_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p4_ MySQL database")
	}
	f := p4Setup(t)
	ctx, cancel := context.WithTimeout(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), 25*time.Second)
	defer cancel()
	recoverySharedSetDue(t, f.id)
	if _, _, err := f.s.resourceProfile(ctx, f.id); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	rows := make([]db.PeriodInstance, 700)
	for i := range rows {
		rows[i] = db.PeriodInstance{ID: fmt.Sprintf("recovery_comparison_%04d", i), Enabled: true, Title: "isolated future window",
			Kind: "isolated_future", Mode: "cumulative", Goal: 1, Rewards: "coin,1", Source: "isolated comparison",
			StartAt: now.Add(time.Hour).UnixMilli(), EndAt: now.Add(2 * time.Hour).UnixMilli(), RewardEndAt: now.Add(3 * time.Hour).UnixMilli()}
	}
	if err := db.GetIns().CreateInBatches(rows, 100).Error; err != nil {
		t.Fatal(err)
	}
	recoverySharedSetDue(t, f.id)
	database := db.GetIns()
	previous := database.Config.Logger
	trace := &recoveryComparisonTrace{Interface: previous}
	database.Config.Logger = trace
	defer func() { database.Config.Logger = previous }()
	finish := holdGatherSharedP1(t, ctx)
	trace.total.Store(0)
	trace.shared.Store(0)
	trace.exclusive.Store(0)
	released := make(chan struct{})
	go func() {
		select {
		case <-time.After(800 * time.Millisecond):
		case <-ctx.Done():
		}
		finish()
		close(released)
	}()
	started := time.Now()
	player, _, err := f.s.resourceProfile(ctx, f.id)
	elapsed := time.Since(started)
	total, shared, exclusive := trace.total.Load(), trace.shared.Load(), trace.exclusive.Load()
	<-released
	if err != nil || player == nil || player.GetEnergy() != 11 {
		t.Fatalf("comparison recovery failed: %v, %v", player, err)
	}
	t.Logf("recovery_comparison hold_ms=800 future_windows=700 elapsed_ms=%.1f sql=%d p1_share=%d p1_update=%d", float64(elapsed.Microseconds())/1000, total, shared, exclusive)
}
