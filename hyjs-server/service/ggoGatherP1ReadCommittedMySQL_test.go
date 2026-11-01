package service

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

type ggoGatherReadFenceKey struct{}

func TestGGOGatherIsolationIsScoped(t *testing.T) {
	mysqlDB := &gorm.DB{Config: &gorm.Config{Dialector: mysql.New(mysql.Config{})}}
	sqliteDB := &gorm.DB{Config: &gorm.Config{Dialector: sqlite.Open(":memory:")}}
	ordinary := context.Background()
	gather := context.WithValue(ordinary, ggoGatherCurrentReadKey{}, true)
	if len(ggoGatherTransactionOptions(mysqlDB, ordinary)) != 0 || len(ggoGatherTransactionOptions(sqliteDB, gather)) != 0 {
		t.Fatal("a non-gather or SQLite transaction changed isolation")
	}
	options := ggoGatherTransactionOptions(mysqlDB, gather)
	if len(options) != 1 || options[0].Isolation != sql.LevelReadCommitted {
		t.Fatalf("MySQL gather transaction isolation: %+v", options)
	}
}

func requireGGOP1IsolatedMySQL(t *testing.T) {
	t.Helper()
	name := os.Getenv("HYJS_P1_MYSQL_DATABASE")
	if name == "" {
		t.Skip("requires a coordinated isolated hyjs_test_p1_ MySQL database")
	}
	if !strings.HasPrefix(name, "hyjs_test_p1_") || os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" {
		t.Fatal("requires an isolated game and accounts fixture")
	}
}

func ggoGatherReadPeriod(t *testing.T, f *ggoPeriodLockFixture, end time.Time) db.PeriodInstance {
	t.Helper()
	id := fmt.Sprintf("ggo-read-fence-%d", time.Now().UnixNano())
	period := p1Instance(t, nil, id, constUtil.P8EventMineGather, "cumulative", 100, end, 0)
	database := db.GetIns()
	if err := database.Transaction(func(tx *gorm.DB) error {
		_, err := f.s.freezeP1Period(tx, period, true)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = database.Where("instance_id=?", id).Delete(&db.PeriodProgress{}).Error
		_ = database.Where("instance_id=?", id).Delete(&db.PeriodSettlement{}).Error
		_ = database.Where("id=?", id).Delete(&db.PeriodInstance{}).Error
	})
	return period
}

// Compare the identical lock interleaving with two actual MySQL transaction
// isolation levels. The first ordinary read establishes an old RR snapshot;
// settlement commits before the same transaction takes P1 SHARE.
func TestGGOGatherP1MySQLPriorSnapshotIsolation(t *testing.T) {
	requireGGOP1IsolatedMySQL(t)
	f := ggoPeriodLockSetup(t)
	database := db.GetIns()
	pool, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(8)
	for _, tc := range []struct {
		name      string
		isolation sql.IsolationLevel
		want      int64
	}{{"repeatable-read", sql.LevelRepeatableRead, 0}, {"read-committed", sql.LevelReadCommitted, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			period := ggoGatherReadPeriod(t, f, time.Now().Add(time.Hour))
			ctx, cancel := context.WithTimeout(context.WithValue(f.ctx, constUtil.P1SharedReadContextKey{}, true), 8*time.Second)
			defer cancel()
			reader := database.WithContext(ctx).Begin(&sql.TxOptions{Isolation: tc.isolation})
			if reader.Error != nil {
				t.Fatal(reader.Error)
			}
			defer reader.Rollback()
			var before int64
			if err := reader.Model(&db.PeriodSettlement{}).Where("instance_id=?", period.ID).Count(&before).Error; err != nil || before != 0 {
				t.Fatalf("initial snapshot: %d %v", before, err)
			}
			if err := database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				var row db.PeriodInstance
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, "id=?", period.ID).Error; err != nil {
					return err
				}
				return tx.Create(&db.PeriodSettlement{InstanceID: period.ID, SettledAt: time.Now().UnixMilli(), Snapshot: []byte(`{}`)}).Error
			}); err != nil {
				t.Fatal(err)
			}
			if err := p1LockPeriodIDs(reader); err != nil {
				t.Fatal(err)
			}
			var observed int64
			if err := reader.Model(&db.PeriodSettlement{}).Where("instance_id=?", period.ID).Count(&observed).Error; err != nil || observed != tc.want {
				t.Fatalf("settlement after P1 SHARE: %d, want %d, err=%v", observed, tc.want, err)
			}
		})
	}
}

// This controlled settlement record is deliberately written while the test
// period is still open. It isolates the settled-row guard from the separate
// accepted-at deadline check and exposes an RR snapshot created too early.
func TestGGOGatherP1MySQLSettledAfterPriorRead(t *testing.T) {
	requireGGOP1IsolatedMySQL(t)
	for _, action := range []string{"claim", "worker", "late-cancel"} {
		t.Run(action, func(t *testing.T) {
			f := ggoPeriodLockSetup(t)
			f.setDue(t, 0)
			period := ggoGatherReadPeriod(t, f, time.Now().Add(time.Hour))
			database := db.GetIns()
			pool, err := database.DB()
			if err != nil {
				t.Fatal(err)
			}
			pool.SetMaxOpenConns(8)
			ctx, cancel := context.WithTimeout(context.WithValue(f.ctx, ggoGatherReadFenceKey{}, action), 12*time.Second)
			defer cancel()
			ready, release := make(chan struct{}), make(chan struct{})
			var readyOnce, releaseOnce sync.Once
			releaseReader := func() { releaseOnce.Do(func() { close(release) }) }
			defer releaseReader()
			const callback = "test:ggo-gather-prior-read-fence"
			if err = database.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Context.Value(ggoGatherReadFenceKey{}) != action {
					return
				}
				target := "ggo_field_receipts"
				if action == "worker" {
					target = "p8_gather_attempts"
				}
				if tx.Statement.Table == target {
					readyOnce.Do(func() {
						close(ready)
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
			done := make(chan error, 1)
			go func() {
				switch action {
				case "claim":
					_, err := f.s.GGOFieldGatherClaim(ctx, f.claimRequest(0), f.sessions[0])
					done <- err
				case "worker":
					done <- f.s.ggoFinishDue(ctx, f.ids[0], f.starts[0].AttemptToken)
				case "late-cancel":
					_, err := f.s.GGOFieldGatherCancel(ctx, f.cancelRequest(0), f.sessions[0])
					done <- err
				}
			}()
			select {
			case <-ready:
			case err := <-done:
				t.Fatalf("gather finished before its prior read: %v", err)
			case <-ctx.Done():
				t.Fatal("gather did not reach its prior read")
			}
			// The writer follows the same period-X-before-settlement order as the
			// real cutoff worker, while keeping the period open for this guard test.
			if err = database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				var live db.PeriodInstance
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&live, "id=?", period.ID).Error; err != nil {
					return err
				}
				return tx.Create(&db.PeriodSettlement{InstanceID: period.ID, SettledAt: time.Now().UnixMilli(), Snapshot: []byte(`{}`)}).Error
			}); err != nil {
				t.Fatalf("settlement could not commit before gather P1 lock: %v", err)
			}
			releaseReader()
			select {
			case err = <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("gather did not complete after settlement")
			}
			if p1Count(t, &db.PeriodProgress{}, "instance_id=? AND player_id=?", period.ID, f.ids[0]) != 0 ||
				p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.ids[0], "ggo:gather:"+f.starts[0].AttemptToken) != 1 ||
				p1Count(t, &db.GGOFieldReceipt{}, "player_id=? AND operation=? AND business_key=?", f.ids[0], constUtil.GGOFieldOpGatherClaim, f.starts[0].AttemptToken) != 1 {
				t.Fatal("a settled window accepted progress or the gather lost its own fact/receipt")
			}
		})
	}
}

// A real cutoff may commit after the gather's first ordinary read but before
// it acquires activity SHARE. The gather's own receipt is still durable, while
// the closed period must not receive progress.
func TestGGOGatherP1MySQLCutoffAfterPriorRead(t *testing.T) {
	requireGGOP1IsolatedMySQL(t)
	f := ggoPeriodLockSetup(t)
	f.setDue(t, 0)
	deadline := time.Now().Add(4 * time.Second)
	period := ggoGatherReadPeriod(t, f, deadline)
	database := db.GetIns()
	pool, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(8)
	ctx, cancel := context.WithTimeout(context.WithValue(f.ctx, ggoGatherReadFenceKey{}, "cutoff"), 14*time.Second)
	defer cancel()
	ready, release := make(chan struct{}), make(chan struct{})
	var readyOnce, releaseOnce sync.Once
	releaseReader := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseReader()
	const callback = "test:ggo-gather-prior-read-cutoff"
	if err = database.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Context.Value(ggoGatherReadFenceKey{}) == "cutoff" && tx.Statement.Table == "ggo_field_receipts" {
			readyOnce.Do(func() {
				close(ready)
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
	done := make(chan error, 1)
	go func() {
		_, err := f.s.GGOFieldGatherClaim(ctx, f.claimRequest(0), f.sessions[0])
		done <- err
	}()
	select {
	case <-ready:
		if !time.Now().Before(deadline) {
			t.Fatal("gather did not establish its prior read before cutoff")
		}
	case err := <-done:
		t.Fatalf("gather exited before its prior read: %v", err)
	case <-ctx.Done():
		t.Fatal("gather did not reach its prior read")
	}
	timer := time.NewTimer(time.Until(deadline) + 20*time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		t.Fatal("cutoff wait timed out")
	}
	if err = f.s.settleP1Period(f.ctx, period.ID); err != nil {
		t.Fatalf("real cutoff settlement: %v", err)
	}
	if p1Count(t, &db.PeriodSettlement{}, "instance_id=?", period.ID) != 1 {
		t.Fatal("real cutoff did not commit before the gather lock")
	}
	releaseReader()
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("gather did not complete after cutoff")
	}
	if p1Count(t, &db.PeriodProgress{}, "instance_id=? AND player_id=?", period.ID, f.ids[0]) != 0 ||
		p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.ids[0], "ggo:gather:"+f.starts[0].AttemptToken) != 1 ||
		p1Count(t, &db.GGOFieldReceipt{}, "player_id=? AND operation=? AND business_key=?", f.ids[0], constUtil.GGOFieldOpGatherClaim, f.starts[0].AttemptToken) != 1 {
		t.Fatal("cutoff gather changed closed progress or lost its own fact/receipt")
	}
}
