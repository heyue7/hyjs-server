package service

import (
	"context"
	"database/sql"
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
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Stop after the period scan returns, while the transaction still holds its
// directory guard. Shared gathers use an ordinary READ COMMITTED snapshot.
type p1GatherCatalogScanBarrier struct {
	logger.Interface
	armed   atomic.Bool
	shared  bool
	scanned chan struct{}
	release chan struct{}
}

func (b *p1GatherCatalogScanBarrier) Trace(ctx context.Context, _ time.Time, statement func() (string, int64), _ error) {
	query, _ := statement()
	query = strings.ToLower(query)
	shared := ctx.Value(constUtil.P1SharedReadContextKey{}) == true
	if shared != b.shared || !strings.Contains(query, "select * from p1_period_instances") ||
		(b.shared && (strings.Contains(query, "for share") || strings.Contains(query, "for update"))) ||
		(!b.shared && !strings.Contains(query, "for update")) || !b.armed.CompareAndSwap(true, false) {
		return
	}
	close(b.scanned)
	select {
	case <-b.release:
	case <-ctx.Done():
	}
}

// This is a transaction-level reproduction, not a test of the current admin
// UI. The typed publisher is invoked with a synthetic mine definition because
// the current public mode catalogue does not offer mine or fish activities.
// A safe implementation must keep the publisher behind the gather commit, or
// otherwise include a publication which committed before the gather.
func TestP1GatherCatalogSharedPhantomMySQL(t *testing.T) {
	testP1GatherCatalogPhantomMySQL(t, true)
}

func TestP1GatherCatalogExclusivePhantomMySQL(t *testing.T) {
	testP1GatherCatalogPhantomMySQL(t, false)
}

// Run each variant in its own fresh, isolated database.
func testP1GatherCatalogPhantomMySQL(t *testing.T, shared bool) {
	t.Helper()
	if os.Getenv("HYJS_P1_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p1_ MySQL database")
	}
	f := p1GatherFixture(t)
	if err := db.GetIns().AutoMigrate(&db.P8GatherAttempt{}); err != nil {
		t.Fatal(err)
	}
	var existing int64
	if err := db.GetIns().Model(&db.PeriodInstance{}).Where("kind=? AND enabled=?", constUtil.P8EventMineGather, true).Count(&existing).Error; err != nil || existing != 0 {
		t.Fatalf("reproduction requires a fresh database without an enabled mine period: %d %v", existing, err)
	}
	started := db.P8GatherAttempt{PlayerID: f.id, Token: "catalog-phantom", Active: true}
	if err := db.GetIns().Create(&started).Error; err != nil {
		t.Fatal(err)
	}
	before, _ := f.profile(t)
	ctx, cancel := context.WithTimeout(p1GatherContext(f), 15*time.Second)
	defer cancel()
	database := db.GetIns()
	previous := database.Config.Logger
	barrier := &p1GatherCatalogScanBarrier{Interface: previous, shared: shared, scanned: make(chan struct{}), release: make(chan struct{})}
	barrier.armed.Store(true)
	database.Config.Logger = barrier
	defer func() { database.Config.Logger = previous }()
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(barrier.release) }) }
	defer release()
	gatherDone := make(chan error, 1)
	go func() {
		apply := func(tx *gorm.DB, player *pb.Player, _ *model.GameState) error {
			var attempt db.P8GatherAttempt
			if err := tx.First(&attempt, "player_id=? AND active=?", f.id, true).Error; err != nil {
				return err
			}
			attempt.Active = false
			if err := tx.Save(&attempt).Error; err != nil {
				return err
			}
			player.Coin = proto.Int64(player.GetCoin() + 1)
			return f.s.recordP1GatherEvent(tx, f.id, &attempt, constUtil.P8EventMineGather, false, time.Now())
		}
		if shared {
			gatherDone <- p1GatherTestSharedProfile(f.s, ctx, f.id, apply)
		} else {
			gatherDone <- p1GatherTestExclusiveProfile(f.s, ctx, f.id, apply)
		}
	}()
	select {
	case <-barrier.scanned:
	case err := <-gatherDone:
		t.Fatalf("gather finished before the full period scan: %v", err)
	case <-ctx.Done():
		t.Fatal("gather did not reach the full period scan")
	}
	now := time.Now()
	definition := adminActivityDefinition{mode: "isolated-mine", domain: "period", kind: constUtil.P8EventMineGather}
	form := model.AdminActivityForm{
		Mode: "isolated-mine", Name: "隔离矿工活动", Source: "隔离测试",
		StartAt: now.Add(-time.Minute).UnixMilli(), EndAt: now.Add(time.Hour).UnixMilli(), ClaimEndAt: now.Add(2 * time.Hour).UnixMilli(),
		Milestones: []model.AdminActivityMilestone{{Goal: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 1}}}},
	}
	const periodID = "p1:catalog-phantom"
	type publishResult struct {
		factVisible int64
		err         error
	}
	publisherStarted := make(chan struct{})
	publishDone := make(chan publishResult, 1)
	go func() {
		var visible int64
		close(publisherStarted)
		err := database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var err error
			tx, err = p1AcquireGatherCatalogWriter(tx)
			if err != nil {
				return err
			}
			if err := f.s.adminPersistTypedActivity(tx, definition, periodID, form, true, false); err != nil {
				return err
			}
			return tx.Model(&db.EventFact{}).Where("player_id=? AND source_key=?", f.id, "p8:gather:catalog-phantom").Count(&visible).Error
		}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		publishDone <- publishResult{factVisible: visible, err: err}
	}()
	<-publisherStarted
	var publishedEarly *publishResult
	select {
	case result := <-publishDone:
		publishedEarly = &result
	case <-time.After(time.Second):
	case <-ctx.Done():
		t.Fatal("publisher timed out during the scan")
	}
	release()
	select {
	case err := <-gatherDone:
		if err != nil {
			t.Fatalf("gather transaction failed: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("gather did not commit")
	}
	var published publishResult
	if publishedEarly != nil {
		published = *publishedEarly
	} else {
		select {
		case published = <-publishDone:
		case <-ctx.Done():
			t.Fatal("publisher did not finish after the gather")
		}
	}
	if published.err != nil {
		t.Fatalf("publisher failed: %v", published.err)
	}
	var attempt db.P8GatherAttempt
	if err := database.First(&attempt, "player_id=?", f.id).Error; err != nil {
		t.Fatal(err)
	}
	after, _ := f.profile(t)
	facts := p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.id, "p8:gather:catalog-phantom")
	progress := p1Count(t, &db.PeriodProgress{}, "instance_id=? AND player_id=?", periodID, f.id)
	if attempt.Active || after.GetCoin() != before.GetCoin()+1 || facts != 1 {
		t.Fatal("gather or its durable fact did not commit exactly once")
	}
	if publishedEarly != nil && progress == 0 {
		t.Fatalf("phantom reproduced: enabled mine period committed after the scan but before gather commit; fact=%d progress=%d", facts, progress)
	}
	if publishedEarly == nil && published.factVisible != 1 {
		t.Fatalf("publisher passed before the admitted gather committed: visible fact=%d", published.factVisible)
	}
}
