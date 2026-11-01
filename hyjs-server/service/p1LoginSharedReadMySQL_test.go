package service

import (
	"context"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

type loginPeriodLockTrace struct {
	logger.Interface
	shared    atomic.Int64
	exclusive atomic.Int64
}

func (trace *loginPeriodLockTrace) Trace(_ context.Context, _ time.Time, statement func() (string, int64), _ error) {
	query, _ := statement()
	query = strings.ToLower(query)
	if !strings.Contains(query, "p1_period_instances") {
		return
	}
	if strings.Contains(query, "for share") {
		trace.shared.Add(1)
	}
	if strings.Contains(query, "for update") {
		trace.exclusive.Add(1)
	}
}

// InnoDB verifies that an initial write rolls back and retries exclusively,
// whereas an unchanged same-day login can pass another player's shared read.
func verifyLoginSharedAndRetry(t *testing.T, f *p1Fixture, login func(context.Context, int64) error, checkFacts func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), 15*time.Second)
	defer cancel()
	database := db.GetIns()
	previous := database.Config.Logger
	trace := &loginPeriodLockTrace{Interface: previous}
	database.Config.Logger = trace
	defer func() { database.Config.Logger = previous }()

	holdShared := func() (func(), <-chan error) {
		t.Helper()
		acquired := make(chan struct{})
		release := make(chan struct{})
		finished := make(chan error, 1)
		go func() {
			shared := context.WithValue(ctx, constUtil.P1SharedReadContextKey{}, true)
			finished <- database.WithContext(shared).Transaction(func(tx *gorm.DB) error {
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
		case err := <-finished:
			t.Fatalf("failed to hold shared periods: %v", err)
		case <-ctx.Done():
			t.Fatal("shared period holder timed out")
		}
		return func() { close(release) }, finished
	}

	release, holderDone := holdShared()
	trace.shared.Store(0)
	trace.exclusive.Store(0)
	first := make(chan error, 1)
	go func() { first <- login(ctx, f.id) }()
	select {
	case err := <-first:
		release()
		t.Fatalf("new login facts did not wait for the exclusive retry: %v", err)
	case <-time.After(250 * time.Millisecond):
	}
	release()
	if err := <-holderDone; err != nil {
		t.Fatal(err)
	}
	if err := <-first; err != nil {
		t.Fatalf("initial login failed after releasing shared lock: %v", err)
	}
	if trace.shared.Load() == 0 || trace.exclusive.Load() == 0 {
		t.Fatalf("initial login did not retry after shared attempt: shared=%d exclusive=%d", trace.shared.Load(), trace.exclusive.Load())
	}
	checkFacts()

	release, holderDone = holdShared()
	trace.shared.Store(0)
	trace.exclusive.Store(0)
	repeated := make(chan error, 1)
	go func() { repeated <- login(ctx, f.id) }()
	select {
	case err := <-repeated:
		if err != nil {
			release()
			t.Fatalf("repeat login failed: %v", err)
		}
	case <-time.After(4 * time.Second):
		release()
		t.Fatal("repeat login blocked behind another shared read")
	}
	if trace.shared.Load() == 0 || trace.exclusive.Load() != 0 {
		release()
		t.Fatalf("repeat login took an exclusive activity lock: shared=%d exclusive=%d", trace.shared.Load(), trace.exclusive.Load())
	}
	release()
	if err := <-holderDone; err != nil {
		t.Fatal(err)
	}
	checkFacts()
}

func TestP1LoginSharedRetryRealMySQL(t *testing.T) {
	if os.Getenv("HYJS_P1_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p1_ MySQL database")
	}
	f := p1Setup(t)
	verifyLoginSharedAndRetry(t, f, f.s.RecordP1Login, func() {
		if n := p1Count(t, &db.EventFact{}, "player_id=? AND source_key=? AND kind=?", f.id, "login:"+p1Day(time.Now()), constUtil.P1EventLoginDay); n != 1 {
			t.Fatalf("login fact count=%d", n)
		}
	})
}

func TestP4LoginSharedRetryRealMySQL(t *testing.T) {
	if os.Getenv("HYJS_P4_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p4_ MySQL database")
	}
	f := p4Setup(t)
	verifyLoginSharedAndRetry(t, f, f.s.RecordP4Login, func() {
		collect := p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P4KindCollect)
		power := p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P4KindPower)
		if collect != 1 || power != 1 {
			t.Fatalf("collection/power facts changed: %d/%d", collect, power)
		}
	})
}

func TestP1LoginWithP4HooksSharedRetryRealMySQL(t *testing.T) {
	if os.Getenv("HYJS_P4_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p4_ MySQL database")
	}
	f := p4Setup(t)
	verifyLoginSharedAndRetry(t, f, f.s.RecordP1Login, func() {
		login := p1Count(t, &db.EventFact{}, "player_id=? AND source_key=? AND kind=?", f.id, "login:"+p1Day(time.Now()), constUtil.P1EventLoginDay)
		collect := p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P4KindCollect)
		power := p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P4KindPower)
		if login != 1 || collect != 1 || power != 1 {
			t.Fatalf("login/collection/power facts changed: %d/%d/%d", login, collect, power)
		}
	})
}
