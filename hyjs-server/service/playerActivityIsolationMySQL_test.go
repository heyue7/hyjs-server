package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Frozen shared activities must permit independent roles while each role's
// own profile and progress remain serialized with its durable event fact.
func TestPlayerConcurrencyMySQLActivityIsolation(t *testing.T) {
	if os.Getenv("HYJS_P1_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p1_ MySQL database")
	}
	f := p1Setup(t)
	peer := p1GatherSecondPlayer(t, f)
	now := time.Now()
	prefix := fmt.Sprintf("concurrency-%d", now.UnixNano())
	common := p1Instance(t, f, prefix+"-common", "draw_count", "cumulative", 1000, now.Add(time.Hour), 0)
	unrelated := p1Instance(t, f, prefix+"-unrelated", "irrelevant", "cumulative", 1000, now.Add(time.Hour), 0)
	foreign := p1Instance(t, f, prefix+"-foreign", "draw_count", "cumulative", 1000, now.Add(time.Hour), 0)
	foreign.OwnerPlayerID = f.id
	updated := db.GetIns().Model(&db.PeriodInstance{}).Where("id=?", foreign.ID).Update("owner_player_id", foreign.OwnerPlayerID)
	if updated.Error != nil || updated.RowsAffected != 1 {
		t.Fatalf("assign private activity owner: rows=%d err=%v", updated.RowsAffected, updated.Error)
	}
	for _, cfg := range []db.PeriodInstance{common, foreign} {
		frozen, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		updated = db.GetIns().Model(&db.PeriodInstance{}).Where("id=?", cfg.ID).Update("frozen", frozen)
		if updated.Error != nil || updated.RowsAffected != 1 {
			t.Fatalf("freeze activity %s: rows=%d err=%v", cfg.ID, updated.RowsAffected, updated.Error)
		}
	}

	ctx, cancel := context.WithTimeout(p1GatherContext(f), 8*time.Second)
	txA := db.GetIns().WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if txA.Error != nil {
		cancel()
		t.Fatal(txA.Error)
	}
	var peerDone, sameDone <-chan error
	defer func() {
		// Release row locks before cancellation and collect every started worker.
		_ = txA.Rollback().Error
		cancel()
		if peerDone != nil {
			<-peerDone
		}
		if sameDone != nil {
			<-sameDone
		}
	}()

	var owner db.Profile
	if err := txA.Clauses(clause.Locking{Strength: "UPDATE"}).First(&owner, f.id).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{unrelated.ID, foreign.ID} {
		var locked db.PeriodInstance
		if err := txA.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id=?", id).Error; err != nil {
			t.Fatal(err)
		}
	}
	var shared db.PeriodInstance
	if err := txA.Clauses(clause.Locking{Strength: "SHARE"}).First(&shared, "id=?", common.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.RecordP1Event(txA, f.id, "parallel-a", "draw_count", 3, now); err != nil {
		t.Fatal(err)
	}

	peerResult := make(chan error, 1)
	peerDone = peerResult
	go func() {
		peerResult <- updateProfileWithTx(ctx, peer, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
			return f.s.RecordP1Event(tx, peer, "parallel-b", "draw_count", 5, now)
		})
	}()
	select {
	case err := <-peerDone:
		peerDone = nil
		if err != nil {
			t.Fatalf("independent role failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("independent role waited for an unrelated or foreign activity")
	case <-ctx.Done():
		t.Fatalf("independent role timed out: %v", ctx.Err())
	}

	var peerProgress db.PeriodProgress
	if err := db.GetIns().WithContext(ctx).First(&peerProgress, "instance_id=? AND player_id=?", common.ID, peer).Error; err != nil || peerProgress.Value != 5 {
		t.Fatalf("independent common progress: value=%d err=%v", peerProgress.Value, err)
	}
	var foreignProgress db.PeriodProgress
	if err := db.GetIns().WithContext(ctx).First(&foreignProgress, "instance_id=? AND player_id=?", foreign.ID, peer).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("foreign activity created peer progress or read failed: %v", err)
	}
	var peerFact db.EventFact
	if err := db.GetIns().WithContext(ctx).First(&peerFact, "player_id=? AND source_key=? AND kind=?", peer, "parallel-b", "draw_count").Error; err != nil || peerFact.Amount != 5 {
		t.Fatalf("independent committed fact: amount=%d err=%v", peerFact.Amount, err)
	}

	sameStarted, sameAcquired := make(chan struct{}), make(chan struct{})
	sameResult := make(chan error, 1)
	sameDone = sameResult
	go func() {
		close(sameStarted)
		sameResult <- updateProfileWithTx(ctx, f.id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
			close(sameAcquired)
			return f.s.RecordP1Event(tx, f.id, "parallel-same", "draw_count", 1, now)
		})
	}()
	select {
	case <-sameStarted:
	case <-ctx.Done():
		t.Fatalf("same-role worker did not start: %v", ctx.Err())
	}
	select {
	case <-sameAcquired:
		t.Fatal("same role passed the held profile lock")
	case err := <-sameDone:
		sameDone = nil
		t.Fatalf("same role finished while its profile was locked: %v", err)
	case <-time.After(100 * time.Millisecond):
	case <-ctx.Done():
		t.Fatalf("same-role lock check timed out: %v", ctx.Err())
	}
	if err := txA.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-sameDone:
		sameDone = nil
		if err != nil {
			t.Fatalf("same role failed after release: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("same role did not finish after release: %v", ctx.Err())
	}
	var ownerProgress db.PeriodProgress
	if err := db.GetIns().WithContext(ctx).First(&ownerProgress, "instance_id=? AND player_id=?", common.ID, f.id).Error; err != nil || ownerProgress.Value != 4 {
		t.Fatalf("serialized common progress: value=%d err=%v", ownerProgress.Value, err)
	}
	if err := updateProfileWithTx(ctx, peer, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
		return f.s.RecordP1Event(tx, peer, "parallel-b", "draw_count", 5, now)
	}); err != nil {
		t.Fatalf("replay independent fact: %v", err)
	}
	if err := db.GetIns().WithContext(ctx).First(&peerProgress, "instance_id=? AND player_id=?", common.ID, peer).Error; err != nil || peerProgress.Value != 5 {
		t.Fatalf("replayed common progress: value=%d err=%v", peerProgress.Value, err)
	}
}
