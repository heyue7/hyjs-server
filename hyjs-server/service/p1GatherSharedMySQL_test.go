package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p1GatherSecondPlayer(t *testing.T, f *p1Fixture) int64 {
	t.Helper()
	account, err := LoginAccount(f.ctx, fmt.Sprintf("p1-gather-peer-%d", time.Now().UnixNano()), "isolated-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, account.Id, "采集活动并发", "hero_043"); err != nil {
		t.Fatal(err)
	}
	return account.Id
}

// InnoDB must allow another player's already-frozen projection to commit while
// the first transaction holds directory shares and uncommitted progress.
func TestP1GatherFrozenMySQLConcurrentIsolation(t *testing.T) {
	if os.Getenv("HYJS_P1_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p1_ MySQL database")
	}
	f := p1GatherFixture(t)
	peer := p1GatherSecondPlayer(t, f)
	period := p1Instance(t, f, "gather-shared-mysql", constUtil.P8EventMineGather, "cumulative", 1, time.Now().Add(time.Hour), 0)
	p1GatherFreeze(t, f, period.ID)
	ctx, cancel := context.WithTimeout(p1GatherContext(f), 15*time.Second)
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	releaseHeld := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseHeld()
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- p1GatherTestSharedProfile(f.s, ctx, f.id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
			if !p1SharedRead(tx) {
				return errors.New("frozen first gather retried exclusively")
			}
			attempt := &db.P8GatherAttempt{PlayerID: f.id, Token: "concurrent-first"}
			if err := f.s.recordP1GatherEvent(tx, f.id, attempt, constUtil.P8EventMineGather, false, time.Now()); err != nil {
				return err
			}
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	select {
	case <-entered:
	case err := <-firstDone:
		t.Fatalf("first gather failed before holding the transaction: %v", err)
	case <-ctx.Done():
		t.Fatal("first gather timed out")
	}
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- p1GatherTestSharedProfile(f.s, ctx, peer, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
			if !p1SharedRead(tx) {
				return errors.New("frozen peer gather retried exclusively")
			}
			return f.s.recordP1GatherEvent(tx, peer, &db.P8GatherAttempt{PlayerID: peer, Token: "concurrent-peer"}, constUtil.P8EventMineGather, true, time.Now())
		})
	}()
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("independent gather failed: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("another player's gather waited for the first transaction")
	case <-ctx.Done():
		t.Fatal("independent gather timed out")
	}

	// The same player still owns a single profile lock; an activity writer must
	// also wait until the already-admitted shared projection commits.
	sameDone := make(chan error, 1)
	go func() {
		sameDone <- p1GatherTestSharedProfile(f.s, ctx, f.id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
			return f.s.recordP1GatherEvent(tx, f.id, &db.P8GatherAttempt{PlayerID: f.id, Token: "concurrent-same"}, constUtil.P8EventMineGather, false, time.Now())
		})
	}()
	exclusiveAcquired, exclusiveDone := make(chan struct{}), make(chan error, 1)
	go func() {
		exclusiveDone <- db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			guarded, err := p1AcquireGatherCatalogWriter(tx)
			if err != nil {
				return err
			}
			var current db.PeriodInstance
			if err := guarded.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, "id=?", period.ID).Error; err != nil {
				return err
			}
			close(exclusiveAcquired)
			return nil
		})
	}()
	select {
	case err := <-sameDone:
		t.Fatalf("same player passed its held profile lock: %v", err)
	case <-exclusiveAcquired:
		t.Fatal("exclusive period writer passed a held gather")
	case err := <-exclusiveDone:
		t.Fatalf("exclusive writer failed unexpectedly: %v", err)
	case <-time.After(250 * time.Millisecond):
	}
	releaseHeld()
	for _, result := range []<-chan error{firstDone, sameDone, exclusiveDone} {
		select {
		case err := <-result:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("blocked transaction did not finish after release")
		}
	}
	for _, expected := range []struct {
		id    int64
		value int64
	}{{f.id, 2}, {peer, 1}} {
		var progress db.PeriodProgress
		if err := db.GetIns().First(&progress, "instance_id=? AND player_id=?", period.ID, expected.id).Error; err != nil || progress.Value != expected.value {
			t.Fatalf("per-player progress mismatch: %+v %v", progress, err)
		}
		if p1Count(t, &db.RewardDelivery{}, "player_id=? AND source_key=?", expected.id, "activity:"+period.ID) != 1 {
			t.Fatalf("player %d did not receive exactly one activity attachment", expected.id)
		}
	}
}

// Both initial shared attempts must roll back before either upgrades to the
// original exclusive freeze path; one frozen policy governs both outcomes.
func TestP1GatherFirstFreezeMySQLConcurrentRetry(t *testing.T) {
	if os.Getenv("HYJS_P1_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p1_ MySQL database")
	}
	f := p1GatherFixture(t)
	peer := p1GatherSecondPlayer(t, f)
	period := p1Instance(t, f, "gather-first-mysql", constUtil.P8EventMineGather, "cumulative", 1, time.Now().Add(time.Hour), 0)
	ctx, cancel := context.WithTimeout(p1GatherContext(f), 15*time.Second)
	defer cancel()
	entered := make(chan int64, 2)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseHeld := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseHeld()
	type result struct {
		id    int64
		modes []bool
		err   error
	}
	finished := make(chan result, 2)
	for _, id := range []int64{f.id, peer} {
		id := id
		go func() {
			var modes []bool
			err := p1GatherTestSharedProfile(f.s, ctx, id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
				shared := p1SharedRead(tx)
				modes = append(modes, shared)
				if shared {
					entered <- id
					select {
					case <-release:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				attempt := &db.P8GatherAttempt{PlayerID: id, Token: fmt.Sprintf("first-freeze-%d", id)}
				return f.s.recordP1GatherEvent(tx, id, attempt, constUtil.P8EventMineGather, false, time.Now())
			})
			finished <- result{id: id, modes: modes, err: err}
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case outcome := <-finished:
			t.Fatalf("gather exited before both shared attempts: %+v", outcome)
		case <-ctx.Done():
			t.Fatal("initial shared attempts did not both enter")
		}
	}
	releaseHeld()
	for i := 0; i < 2; i++ {
		select {
		case outcome := <-finished:
			if outcome.err != nil || len(outcome.modes) != 2 || !outcome.modes[0] || outcome.modes[1] {
				t.Fatalf("player %d did not retry after rollback: %v %v", outcome.id, outcome.modes, outcome.err)
			}
			var progress db.PeriodProgress
			if err := db.GetIns().First(&progress, "instance_id=? AND player_id=?", period.ID, outcome.id).Error; err != nil || progress.Value != 1 {
				t.Fatalf("first freeze progress: %+v %v", progress, err)
			}
			if p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", outcome.id, fmt.Sprintf("p8:gather:first-freeze-%d", outcome.id)) != 1 {
				t.Fatal("first freeze fact was not exactly once")
			}
		case <-ctx.Done():
			t.Fatal("first freeze retries timed out")
		}
	}
	var saved db.PeriodInstance
	if err := db.GetIns().First(&saved, "id=?", period.ID).Error; err != nil || len(saved.Frozen) == 0 {
		t.Fatalf("period was not frozen: %v", err)
	}
}

func TestP1GatherDisableMySQLWaitsForAdmittedFact(t *testing.T) {
	if os.Getenv("HYJS_P1_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p1_ MySQL database")
	}
	f := p1GatherFixture(t)
	peer := p1GatherSecondPlayer(t, f)
	period := p1Instance(t, f, "gather-disable-mysql", constUtil.P8EventMineGather, "cumulative", 1, time.Now().Add(time.Hour), 0)
	p1GatherFreeze(t, f, period.ID)
	ctx, cancel := context.WithTimeout(p1GatherContext(f), 15*time.Second)
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	releaseHeld := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseHeld()
	writeDone := make(chan error, 1)
	go func() {
		writeDone <- p1GatherTestSharedProfile(f.s, ctx, f.id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
			if err := f.s.recordP1GatherEvent(tx, f.id, &db.P8GatherAttempt{PlayerID: f.id, Token: "disable-before"}, constUtil.P8EventMineGather, false, time.Now()); err != nil {
				return err
			}
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	select {
	case <-entered:
	case err := <-writeDone:
		t.Fatalf("gather exited before the disable test: %v", err)
	case <-ctx.Done():
		t.Fatal("gather did not reach the disable test")
	}
	started, disabled := make(chan struct{}), make(chan error, 1)
	go func() {
		close(started)
		disabled <- db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			guarded, err := p1AcquireGatherCatalogWriter(tx)
			if err != nil {
				return err
			}
			return guarded.Model(&db.PeriodInstance{}).Where("id=?", period.ID).Update("enabled", false).Error
		})
	}()
	<-started
	select {
	case err := <-disabled:
		t.Fatalf("disable passed an admitted gather: %v", err)
	case <-time.After(250 * time.Millisecond):
	}
	releaseHeld()
	for _, result := range []<-chan error{writeDone, disabled} {
		select {
		case err := <-result:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("disable transaction did not finish")
		}
	}
	if err := p1GatherTestSharedProfile(f.s, ctx, peer, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
		return f.s.recordP1GatherEvent(tx, peer, &db.P8GatherAttempt{PlayerID: peer, Token: "disable-after"}, constUtil.P8EventMineGather, true, time.Now())
	}); err != nil {
		t.Fatal(err)
	}
	if p1ProgressValue(t, f, period.ID).Value != 1 || p1Count(t, &db.PeriodProgress{}, "instance_id=? AND player_id=?", period.ID, peer) != 0 ||
		p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", peer, "ggo:gather:disable-after") != 1 {
		t.Fatal("disable changed the admitted fact or counted the later fact")
	}
}

// A transaction admitted before the deadline must be included even if its
// commit occurs after the deadline and settlement is already waiting.
func TestP1GatherCutoffMySQLWaitsForAdmittedFact(t *testing.T) {
	if os.Getenv("HYJS_P1_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p1_ MySQL database")
	}
	f := p1GatherFixture(t)
	peer := p1GatherSecondPlayer(t, f)
	deadline := time.Now().Add(4 * time.Second)
	period := p1Instance(t, f, "gather-cutoff-mysql", constUtil.P8EventMineGather, "cumulative", 1, deadline, 2)
	p1GatherFreeze(t, f, period.ID)
	ctx, cancel := context.WithTimeout(p1GatherContext(f), 15*time.Second)
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	releaseHeld := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseHeld()
	writeDone := make(chan error, 1)
	go func() {
		writeDone <- p1GatherTestSharedProfile(f.s, ctx, f.id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
			if err := f.s.recordP1GatherEvent(tx, f.id, &db.P8GatherAttempt{PlayerID: f.id, Token: "cutoff-before"}, constUtil.P8EventMineGather, false, time.Now()); err != nil {
				return err
			}
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	select {
	case <-entered:
		if !time.Now().Before(deadline) {
			t.Fatal("fixture did not record the event before the deadline")
		}
	case err := <-writeDone:
		t.Fatalf("gather exited before the deadline test: %v", err)
	case <-ctx.Done():
		t.Fatal("gather did not reach the deadline test")
	}
	if err := p1GatherTestSharedProfile(f.s, ctx, peer, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
		return f.s.recordP1GatherEvent(tx, peer, &db.P8GatherAttempt{PlayerID: peer, Token: "cutoff-peer"}, constUtil.P8EventMineGather, true, time.Now())
	}); err != nil || !time.Now().Before(deadline) {
		t.Fatalf("peer fact was not committed before cutoff: %v", err)
	}
	timer := time.NewTimer(time.Until(deadline) + 20*time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		t.Fatal("deadline wait timed out")
	}
	settleDone := make(chan error, 1)
	go func() { settleDone <- f.s.settleP1Period(ctx, period.ID) }()
	select {
	case err := <-settleDone:
		t.Fatalf("settlement passed an uncommitted admitted gather: %v", err)
	case <-time.After(250 * time.Millisecond):
	}
	releaseHeld()
	for _, result := range []<-chan error{writeDone, settleDone} {
		select {
		case err := <-result:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("cutoff transaction did not finish")
		}
	}
	var stored db.PeriodSettlement
	if err := db.GetIns().First(&stored, "instance_id=?", period.ID).Error; err != nil {
		t.Fatal(err)
	}
	var snapshot p1Settlement
	first, second := f.id, peer
	if first > second {
		first, second = second, first
	}
	if err := json.Unmarshal(stored.Snapshot, &snapshot); err != nil || len(snapshot.Ranks) != 2 ||
		snapshot.Ranks[0].PlayerID != first || snapshot.Ranks[1].PlayerID != second || snapshot.Ranks[0].Value != 1 || snapshot.Ranks[1].Value != 1 {
		t.Fatalf("deadline snapshot missed the committed gather: %+v %v", snapshot.Ranks, err)
	}
	for _, id := range []int64{f.id, peer} {
		if p1Count(t, &db.RewardDelivery{}, "player_id=? AND source_key=?", id, "rank:"+period.ID) != 1 {
			t.Fatalf("player %d did not receive its stable rank attachment", id)
		}
	}
	if err := p1GatherTestSharedProfile(f.s, ctx, f.id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
		return f.s.recordP1GatherEvent(tx, f.id, &db.P8GatherAttempt{PlayerID: f.id, Token: "cutoff-after"}, constUtil.P8EventMineGather, false, time.Now())
	}); err != nil {
		t.Fatal(err)
	}
	if p1ProgressValue(t, f, period.ID).Value != 1 || p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.id, "p8:gather:cutoff-after") != 1 {
		t.Fatal("late fact changed the closed leaderboard or was lost")
	}
}
