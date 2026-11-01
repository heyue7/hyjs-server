package service

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestPlayerConcurrencyMySQLPVPDoesNotBlockMovement(t *testing.T) {
	name := os.Getenv("HYJS_P12_PK_MYSQL_DATABASE")
	if name == "" {
		t.Skip("requires a coordinated isolated hyjs_test_p12_pk_ MySQL database")
	}
	if !strings.HasPrefix(name, "hyjs_test_p12_pk_") || os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" {
		t.Fatal("requires an isolated game and accounts fixture")
	}

	f, b, c, sessions := p12FieldPVPSetup(t)
	a := f.id
	p12FieldPVPEnable(t)
	f.s.P12FieldPVPReady = true
	playerConcurrencySceneSameLine(t, a, b, c)
	database := db.GetIns()
	pool, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(6)
	pool.SetMaxIdleConns(6)

	var before db.P10ScenePresence
	if err := database.First(&before, "player_id=?", c).Error; err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(f.ctx, 30*time.Second)
	type markerKey struct{}
	marker := new(int)
	fightCtx := context.WithValue(ctx, markerKey{}, marker)
	type fightResult struct {
		messages []proto.Message
		err      error
	}
	var fightDone <-chan fightResult
	var movementDone <-chan error
	entered, release := make(chan struct{}), make(chan struct{})
	var blockOnce, releaseOnce sync.Once
	releaseFight := func() { releaseOnce.Do(func() { close(release) }) }
	callback := database.Callback().Create()
	const callbackName = "test:player-concurrency-pvp-record"
	registered := false
	defer func() {
		releaseFight()
		cancel()
		if fightDone != nil {
			<-fightDone
		}
		if movementDone != nil {
			<-movementDone
		}
		if registered {
			if err := callback.Remove(callbackName); err != nil {
				t.Errorf("remove battle callback: %v", err)
			}
		}
	}()
	if err := callback.Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "p6_battle_records" || tx.Statement.Context.Value(markerKey{}) != marker {
			return
		}
		blockOnce.Do(func() {
			close(entered)
			select {
			case <-release:
			case <-tx.Statement.Context.Done():
				tx.AddError(tx.Statement.Context.Err())
			}
		})
	}); err != nil {
		t.Fatalf("register battle callback: %v", err)
	}
	registered = true

	request := &pb.CSSidePvpFightResult{PlayerId: proto.Int64(b), FightType: proto.Int32(0)}
	stamp := time.Now().UnixNano()
	if stamp <= 0 {
		t.Fatal("fight stamp must be positive")
	}
	fightCh := make(chan fightResult, 1)
	fightDone = fightCh
	go func() {
		messages, err := f.s.p12FieldPVPFightAt(fightCtx, a, sessions[a], request, stamp, time.Now())
		fightCh <- fightResult{messages: messages, err: err}
	}()
	entryTimer := time.NewTimer(15 * time.Second)
	defer entryTimer.Stop()
	select {
	case <-entered:
	case result := <-fightDone:
		fightDone = nil
		t.Fatalf("fight completed before reaching its record write: %v", result.err)
	case <-entryTimer.C:
		t.Fatal("fight did not reach its record write")
	case <-ctx.Done():
		t.Fatalf("fight timed out before record write: %v", ctx.Err())
	}

	movementCh := make(chan error, 1)
	movementDone = movementCh
	go func() {
		if err := playerConcurrencyScenePosition(ctx, c, 72); err != nil {
			movementCh <- fmt.Errorf("update third player's position: %w", err)
			return
		}
		if err := f.s.SyncP10Scene(ctx, c, true); err != nil {
			movementCh <- fmt.Errorf("sync third player's movement: %w", err)
			return
		}
		var after db.P10ScenePresence
		if err := database.WithContext(ctx).First(&after, "player_id=?", c).Error; err != nil {
			movementCh <- fmt.Errorf("read third player's scene presence: %w", err)
			return
		}
		var frame pb.MovePlayer
		if err := proto.Unmarshal(after.Frame, &frame); err != nil {
			movementCh <- fmt.Errorf("decode third player's scene frame: %w", err)
			return
		}
		if after.Scene != before.Scene || after.Line != before.Line || frame.GetPosition().GetCurrent().GetX() != 72 {
			movementCh <- fmt.Errorf("third player's updated position was not published on the same line")
			return
		}
		movementCh <- nil
	}()
	movementTimer := time.NewTimer(2 * time.Second)
	defer movementTimer.Stop()
	select {
	case err := <-movementDone:
		movementDone = nil
		if err != nil {
			t.Fatalf("third player's movement failed: %v", err)
		}
	case <-movementTimer.C:
		t.Fatal("third player's movement waited for the fight transaction")
	case <-ctx.Done():
		t.Fatalf("third player's movement timed out: %v", ctx.Err())
	}
	select {
	case result := <-fightDone:
		fightDone = nil
		t.Fatalf("fight was no longer blocked during movement: %v", result.err)
	default:
	}

	releaseFight()
	var fight fightResult
	select {
	case fight = <-fightDone:
		fightDone = nil
	case <-ctx.Done():
		t.Fatalf("fight did not finish after release: %v", ctx.Err())
	}
	if fight.err != nil || len(fight.messages) == 0 {
		t.Fatalf("fight failed after release: %v", fight.err)
	}
	response, ok := fight.messages[0].(*pb.SCSidePvpFightResult)
	if !ok || response.GetCode() != 1 {
		t.Fatalf("fight did not return a successful result: %+v", fight.messages[0])
	}
	countReceipts := func() int64 {
		t.Helper()
		var count int64
		if err := database.WithContext(ctx).Model(&db.P12FieldPVPReceipt{}).Where("player_id=?", a).Count(&count).Error; err != nil {
			t.Fatalf("count fight receipts: %v", err)
		}
		return count
	}
	if count := countReceipts(); count != 1 {
		t.Fatalf("fight created %d receipts", count)
	}
	var receipt db.P12FieldPVPReceipt
	if err := database.WithContext(ctx).First(&receipt, "player_id=? AND stamp=?", a, stamp).Error; err != nil {
		t.Fatalf("read fight receipt by stamp: %v", err)
	}
	replay, err := f.s.p12FieldPVPFightAt(ctx, a, sessions[a], request, stamp, time.Now())
	if err != nil || len(replay) == 0 {
		t.Fatalf("fight replay failed: %v", err)
	}
	replayResponse, ok := replay[0].(*pb.SCSidePvpFightResult)
	if !ok || !proto.Equal(response, replayResponse) {
		t.Fatal("fight replay changed the recorded result")
	}
	if count := countReceipts(); count != 1 {
		t.Fatalf("fight replay left %d receipts", count)
	}
}
