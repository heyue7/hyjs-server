package service

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// This test uses an allocated, isolated MySQL database; SQLite does not model
// InnoDB's shared and exclusive row-lock behavior.
func TestP1SharedReadRealMySQLPlayerIsolation(t *testing.T) {
	if os.Getenv("HYJS_P1_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p1_ MySQL database")
	}
	f := p1Setup(t)
	other, err := LoginAccount(f.ctx, fmt.Sprintf("p1-shared-mysql-%d", time.Now().UnixNano()), "isolated-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreatePlayer(f.ctx, other.Id, "独立角色", "hero_043"); err != nil {
		t.Fatal(err)
	}
	period := p1Instance(t, f, "p1_shared_mysql_isolation", "shared_mysql_event", "cumulative", 10, time.Now().Add(time.Hour), 0)
	ids := []int64{f.id, other.Id}
	before := make(map[int64]int64, len(ids))
	for _, id := range ids {
		var row db.Profile
		if err := db.GetIns().First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		player, _, err := decodeProfile(&row)
		if err != nil {
			t.Fatal(err)
		}
		before[id] = player.GetCoin()
	}
	ctx, cancel := context.WithTimeout(f.ctx, 15*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, f.s)
	ctx = context.WithValue(ctx, constUtil.GameRequestContextKey, &pb.CSMove{})
	entered := make(chan int64, len(ids))
	finished := make(chan error, len(ids))
	release := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	for _, id := range ids {
		id := id
		go func() {
			finished <- updateFieldProfileWithTx(ctx, id, func(tx *gorm.DB, player *pb.Player, _ *model.GameState) error {
				if p1SharedRead(tx) {
					entered <- id
					select {
					case <-release:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				player.Coin = proto.Int64(player.GetCoin() + 1)
				return f.s.RecordP1Event(tx, id, fmt.Sprintf("shared-mysql:%d", id), "shared_mysql_event", 1, time.Now())
			})
		}()
	}
	for range ids {
		select {
		case <-entered:
		case err := <-finished:
			t.Fatalf("a player failed before both shared transactions entered: %v", err)
		case <-ctx.Done():
			t.Fatal("distinct players did not hold activity reads concurrently")
		}
	}

	exclusiveAcquired := make(chan struct{})
	exclusiveFinished := make(chan error, 1)
	go func() {
		exclusiveFinished <- db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if _, err := p1LockPeriods(tx); err != nil {
				return err
			}
			close(exclusiveAcquired)
			return nil
		})
	}()
	select {
	case <-exclusiveAcquired:
		t.Fatal("an exclusive activity writer passed held shared locks")
	case err := <-exclusiveFinished:
		t.Fatalf("exclusive activity writer failed unexpectedly: %v", err)
	case <-time.After(250 * time.Millisecond):
	}
	releaseOnce.Do(func() { close(release) })
	for range ids {
		select {
		case err := <-finished:
			if err != nil {
				t.Fatalf("player transaction or exclusive retry failed: %v", err)
			}
		case <-ctx.Done():
			t.Fatal("player retry did not finish")
		}
	}
	select {
	case err := <-exclusiveFinished:
		if err != nil {
			t.Fatalf("exclusive activity writer failed after release: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("exclusive activity writer remained blocked")
	}
	for _, id := range ids {
		var row db.Profile
		if err := db.GetIns().First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		player, _, err := decodeProfile(&row)
		if err != nil || player.GetCoin() != before[id]+1 {
			t.Fatalf("rollback or retry changed player %d more than once: %v", id, err)
		}
		if p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", id, fmt.Sprintf("shared-mysql:%d", id)) != 1 {
			t.Fatalf("player %d did not retain exactly one event", id)
		}
		var progress db.PeriodProgress
		if err := db.GetIns().First(&progress, "instance_id=? AND player_id=?", period.ID, id).Error; err != nil || progress.Value != 1 {
			t.Fatalf("player %d progress mismatch: %+v, %v", id, progress, err)
		}
	}
}

// Compare the original exclusive profile transaction and the movement path on
// the same isolated MySQL, players and activity rows. This is a bounded local
// workload and does not claim production capacity or production CPU savings.
func TestP1SharedReadRealMySQLMovementLoad(t *testing.T) {
	if os.Getenv("HYJS_P4_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p4_ MySQL database")
	}
	f := p4Setup(t)
	database := db.GetIns()
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(8)
	sqlDB.SetMaxIdleConns(8)
	now := time.Now()
	periods := make([]db.PeriodInstance, 700)
	for i := range periods {
		periods[i] = db.PeriodInstance{
			ID: fmt.Sprintf("cpuopt-load-%04d", i), Enabled: false, Title: "isolated load",
			Kind: "cpuopt-load", Mode: "cumulative", Goal: 1, Rewards: "coin,1",
			StartAt: now.Add(time.Hour).UnixMilli(), EndAt: now.Add(2 * time.Hour).UnixMilli(),
			RewardEndAt: now.Add(3 * time.Hour).UnixMilli(), Source: "isolated load",
		}
	}
	if err := database.CreateInBatches(periods, 50).Error; err != nil {
		t.Fatal(err)
	}
	ids := []int64{f.id}
	for i := 1; i < 8; i++ {
		account, err := LoginAccount(f.ctx, fmt.Sprintf("cpuopt-load-%d-%d", now.UnixNano(), i), "isolated-test-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := CreatePlayer(f.ctx, account.Id, "并发角色", "hero_043"); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, account.Id)
	}
	base := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	for _, id := range ids {
		if err := updateProfileWithTx(base, id, func(*gorm.DB, *pb.Player, *model.GameState) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	run := func(shared bool) (time.Duration, []time.Duration) {
		start := time.Now()
		latencies := make(chan time.Duration, len(ids)*5)
		errs := make(chan error, len(ids))
		var wg sync.WaitGroup
		for _, id := range ids {
			id := id
			wg.Add(1)
			go func() {
				defer wg.Done()
				for step := 1; step <= 5; step++ {
					move := &pb.CSMove{PointNew: &pb.PointNew{X: proto.Float32(float32(step)), Z: proto.Float32(float32(id))}}
					ctx := context.WithValue(base, constUtil.GameRequestContextKey, move)
					started := time.Now()
					var err error
					if shared {
						_, err = f.s.fieldMove(ctx, id, move)
					} else {
						err = updateProfileWithTx(ctx, id, func(_ *gorm.DB, _ *pb.Player, game *model.GameState) error {
							game.Position.Current = move.GetPointNew()
							return nil
						})
					}
					latencies <- time.Since(started)
					if err != nil {
						errs <- err
						return
					}
				}
			}()
		}
		wg.Wait()
		close(errs)
		close(latencies)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		var times []time.Duration
		for latency := range latencies {
			times = append(times, latency)
		}
		if len(times) != len(ids)*5 {
			t.Fatalf("incomplete movement workload: %d", len(times))
		}
		sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
		return time.Since(start), times
	}
	exclusiveWall, exclusive := run(false)
	sharedWall, shared := run(true)
	t.Logf("8 players, 5 town moves each, 700 extra periods: exclusive wall=%s p50=%s p95=%s; shared wall=%s p50=%s p95=%s",
		exclusiveWall, exclusive[len(exclusive)/2], exclusive[len(exclusive)*95/100-1],
		sharedWall, shared[len(shared)/2], shared[len(shared)*95/100-1])
}
