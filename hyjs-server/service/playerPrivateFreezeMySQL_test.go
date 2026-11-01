package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestPlayerConcurrencyMySQLPrivateFirstFreeze(t *testing.T) {
	name := os.Getenv("HYJS_P1_MYSQL_DATABASE")
	if name == "" {
		t.Skip("requires a coordinated isolated hyjs_test_p1_ MySQL database")
	}
	if !strings.HasPrefix(name, "hyjs_test_p1_") || os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" {
		t.Fatal("requires an isolated game and accounts fixture")
	}

	f := p1GatherFixture(t)
	peer := p1GatherSecondPlayer(t, f)
	database := db.GetIns()
	pool, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(6)
	pool.SetMaxIdleConns(6)

	players := [2]int64{f.id, peer}
	tokens := [2]string{"private-A-first", "private-B-first"}
	end := time.Now().Add(time.Hour)
	periods := [2]db.PeriodInstance{
		p1Instance(t, f, "private-first-freeze-A", constUtil.P8EventMineGather, "cumulative", 1, end, 0),
		p1Instance(t, f, "private-first-freeze-B", constUtil.P8EventMineGather, "cumulative", 1, end, 0),
	}
	for i := range periods {
		periods[i].OwnerPlayerID = players[i]
		periods[i].RealmID = f.s.OriginalServerID()
		result := database.Model(&db.PeriodInstance{}).Where("id=?", periods[i].ID).
			Updates(map[string]any{"owner_player_id": periods[i].OwnerPlayerID, "realm_id": periods[i].RealmID})
		if result.Error != nil || result.RowsAffected != 1 {
			t.Fatalf("assign private period %d: rows=%d error=%v", i, result.RowsAffected, result.Error)
		}
		var initial db.PeriodInstance
		if err := database.First(&initial, "id=?", periods[i].ID).Error; err != nil {
			t.Fatalf("read initial private period %d: %v", i, err)
		}
		if len(initial.Frozen) != 0 || initial.OwnerPlayerID != players[i] || initial.RealmID != periods[i].RealmID {
			t.Fatalf("private period %d is not an unfrozen owned period", i)
		}
	}

	ctx, cancel := context.WithTimeout(p1GatherContext(f), 15*time.Second)
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	releaseA := func() { releaseOnce.Do(func() { close(release) }) }
	var pendingA, pendingB <-chan error
	defer func() {
		releaseA()
		cancel()
		if pendingA != nil {
			<-pendingA
		}
		if pendingB != nil {
			<-pendingB
		}
	}()

	writeGather := func(i int, hold bool) error {
		return p1GatherTestSharedProfile(f.s, ctx, players[i], func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
			if !p1SharedRead(tx) {
				return errors.New("private first freeze retried without shared mode")
			}
			attempt := &db.P8GatherAttempt{PlayerID: players[i], Token: tokens[i]}
			if err := f.s.recordP1GatherEvent(tx, players[i], attempt, constUtil.P8EventMineGather, false, time.Now()); err != nil {
				return err
			}
			var current db.PeriodInstance
			if err := tx.First(&current, "id=?", periods[i].ID).Error; err != nil {
				return err
			}
			if len(current.Frozen) == 0 || current.OwnerPlayerID != players[i] || current.RealmID != periods[i].RealmID {
				return fmt.Errorf("player %d private period was not frozen", i)
			}
			var progress db.PeriodProgress
			if err := tx.First(&progress, "instance_id=? AND player_id=?", periods[i].ID, players[i]).Error; err != nil {
				return err
			}
			if progress.Value != 1 {
				return fmt.Errorf("player %d private progress is %d, want 1", i, progress.Value)
			}
			if hold {
				close(entered)
				select {
				case <-release:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		})
	}

	firstCh := make(chan error, 1)
	pendingA = firstCh
	go func() { firstCh <- writeGather(0, true) }()
	select {
	case <-entered:
	case err := <-pendingA:
		pendingA = nil
		t.Fatalf("player A did not hold its first freeze: %v", err)
	case <-ctx.Done():
		t.Fatalf("player A timed out before holding its first freeze: %v", ctx.Err())
	}

	secondCh := make(chan error, 1)
	pendingB = secondCh
	go func() { secondCh <- writeGather(1, false) }()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case err := <-pendingB:
		pendingB = nil
		if err != nil {
			t.Fatalf("player B first freeze failed: %v", err)
		}
	case <-timer.C:
		t.Fatal("player B first freeze waited for player A")
	case <-ctx.Done():
		t.Fatalf("player B first freeze timed out: %v", ctx.Err())
	}
	select {
	case err := <-pendingA:
		pendingA = nil
		t.Fatalf("player A committed before release: %v", err)
	default:
	}
	releaseA()
	select {
	case err := <-pendingA:
		pendingA = nil
		if err != nil {
			t.Fatalf("player A first freeze failed after release: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("player A did not finish after release: %v", ctx.Err())
	}

	verify := func() {
		t.Helper()
		for i := range periods {
			var current db.PeriodInstance
			if err := database.WithContext(ctx).First(&current, "id=?", periods[i].ID).Error; err != nil {
				t.Fatalf("read committed private period %d: %v", i, err)
			}
			if len(current.Frozen) == 0 || current.OwnerPlayerID != players[i] || current.RealmID != periods[i].RealmID {
				t.Fatalf("committed private period %d is not frozen for its owner", i)
			}
			var progress db.PeriodProgress
			if err := database.WithContext(ctx).First(&progress, "instance_id=? AND player_id=?", periods[i].ID, players[i]).Error; err != nil || progress.Value != 1 {
				t.Fatalf("private progress %d: value=%d error=%v", i, progress.Value, err)
			}
			var count int64
			if err := database.WithContext(ctx).Model(&db.PeriodProgress{}).Where("instance_id=? AND player_id=?", periods[i].ID, players[1-i]).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("cross-owner progress for period %d: count=%d error=%v", i, count, err)
			}
			if err := database.WithContext(ctx).Model(&db.EventFact{}).Where("player_id=? AND source_key=? AND kind=?", players[i], "p8:gather:"+tokens[i], constUtil.P8EventMineGather).Count(&count).Error; err != nil || count != 1 {
				t.Fatalf("player %d gather facts: count=%d error=%v", i, count, err)
			}
		}
	}
	verify()
	for i := range periods {
		if err := writeGather(i, false); err != nil {
			t.Fatalf("player %d gather replay: %v", i, err)
		}
	}
	verify()
}
