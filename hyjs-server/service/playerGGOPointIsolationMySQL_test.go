package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/db"
	"hyjs-server/model"
)

func TestPlayerConcurrencyMySQLGGOPoints(t *testing.T) {
	name := os.Getenv("HYJS_P1_MYSQL_DATABASE")
	if name == "" {
		t.Skip("requires a coordinated isolated hyjs_test_p1_ MySQL database")
	}
	if !strings.HasPrefix(name, "hyjs_test_p1_") || os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" {
		t.Fatal("requires an isolated game and accounts fixture")
	}

	f := ggoPeriodLockSetup(t)
	if err := db.GetIns().AutoMigrate(&db.SharedFieldBoss55{}); err != nil {
		t.Fatal(err)
	}
	f.setDue(t, 0)
	f.setDue(t, 1)
	database := db.GetIns()
	pool, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(6)
	pool.SetMaxIdleConns(6)

	ctx, cancel := context.WithTimeout(f.ctx, 8*time.Second)
	type claimResult struct {
		response *model.GGOFieldGatherClaimResponse
		err      error
	}
	type snapshotResult struct {
		response *model.GGOFieldSnapshotResponse
		err      error
	}
	var pendingA, pendingB <-chan claimResult
	var pendingSnapshot <-chan snapshotResult
	var txA *gorm.DB
	var locked, registered bool
	const callback = "test:player-concurrency-ggo-no-line-update"
	defer func() {
		if locked {
			if err := txA.Rollback().Error; err != nil && !errors.Is(err, sql.ErrTxDone) {
				t.Errorf("release point lock: %v", err)
			}
		}
		cancel()
		if pendingA != nil {
			<-pendingA
		}
		if pendingB != nil {
			<-pendingB
		}
		if pendingSnapshot != nil {
			<-pendingSnapshot
		}
		if registered {
			if err := database.Callback().Update().Remove(callback); err != nil {
				t.Errorf("remove update callback: %v", err)
			}
		}
	}()

	var attempts [2]db.P8GatherAttempt
	var line db.GGOFieldLine
	if err := database.WithContext(ctx).First(&line, "server_id=? AND line=?", f.s.OriginalServerID(), 1).Error; err != nil {
		t.Fatal(err)
	}
	if !line.PointStateReady || line.DeploymentRevision != "isolated-trade-config" || line.CycleMS == 0 {
		t.Fatal("fixture did not finish point migration")
	}
	readPoint := func(i int) *ggoResourceNode {
		t.Helper()
		var point db.GGOFieldPoint
		if err := database.WithContext(ctx).First(&point, "server_id=? AND line=? AND point_id=?", f.s.OriginalServerID(), 1, attempts[i].PosID).Error; err != nil {
			t.Fatalf("read point %d: %v", i, err)
		}
		var payload ggoPointPayload
		if err := json.Unmarshal(point.Payload, &payload); err != nil || payload.Resource == nil {
			t.Fatalf("decode resource point %d: %v", i, err)
		}
		if payload.Resource.PointID != attempts[i].PosID {
			t.Fatalf("point %d payload does not match attempt", i)
		}
		return payload.Resource
	}
	for i := range attempts {
		if err := database.WithContext(ctx).First(&attempts[i], "player_id=? AND token=?", f.ids[i], f.starts[i].AttemptToken).Error; err != nil {
			t.Fatalf("read gather attempt %d: %v", i, err)
		}
		if !attempts[i].Active || attempts[i].PosID == "" || attempts[i].PosID != f.starts[i].PointID {
			t.Fatalf("invalid started gather attempt %d", i)
		}
		point := readPoint(i)
		if point.Remaining != 3 || point.ReservedBy != f.ids[i] || point.AttemptToken != attempts[i].Token {
			t.Fatalf("unexpected initial resource point %d: %+v", i, point)
		}
	}
	if attempts[0].PosID == attempts[1].PosID {
		t.Fatal("fixture must use distinct points on the same line")
	}

	txA = database.WithContext(ctx).Begin()
	if txA.Error != nil {
		t.Fatalf("begin point lock transaction: %v", txA.Error)
	}
	locked = true
	var held db.GGOFieldPoint
	if err := txA.Clauses(clause.Locking{Strength: "UPDATE"}).First(&held, "server_id=? AND line=? AND point_id=?", f.s.OriginalServerID(), 1, attempts[0].PosID).Error; err != nil {
		t.Fatalf("lock player A point: %v", err)
	}
	if err := database.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "ggo_field_lines" {
			tx.AddError(errors.New("steady point action wrote shared line"))
		}
	}); err != nil {
		t.Fatalf("register update callback: %v", err)
	}
	registered = true

	startClaim := func(i int) <-chan claimResult {
		out := make(chan claimResult, 1)
		go func() {
			response, err := f.s.GGOFieldGatherClaim(ctx, f.claimRequest(i), f.sessions[i])
			out <- claimResult{response: response, err: err}
		}()
		return out
	}
	waitClaim := func(ch <-chan claimResult, who string) claimResult {
		t.Helper()
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		select {
		case result := <-ch:
			return result
		case <-timer.C:
			t.Fatalf("%s did not finish within 2 seconds", who)
		case <-ctx.Done():
			t.Fatalf("%s context ended: %v", who, ctx.Err())
		}
		return claimResult{}
	}
	countFact := func(i int) int64 {
		t.Helper()
		var count int64
		if err := database.WithContext(ctx).Model(&db.EventFact{}).Where("player_id=? AND source_key=?", f.ids[i], "ggo:gather:"+attempts[i].Token).Count(&count).Error; err != nil {
			t.Fatalf("count gather facts %d: %v", i, err)
		}
		return count
	}

	pendingB = startClaim(1)
	first := waitClaim(pendingB, "player B claim")
	pendingB = nil
	if first.err != nil || first.response == nil {
		t.Fatalf("player B claim: %+v, %v", first.response, first.err)
	}
	if count := countFact(1); count != 1 {
		t.Fatalf("player B claim created %d gather facts", count)
	}

	snapshotCh := make(chan snapshotResult, 1)
	pendingSnapshot = snapshotCh
	go func() {
		response, err := f.s.GGOFieldSnapshot(ctx, f.auth(1), f.sessions[1])
		snapshotCh <- snapshotResult{response: response, err: err}
	}()
	timer := time.NewTimer(2 * time.Second)
	var snapshot snapshotResult
	select {
	case snapshot = <-pendingSnapshot:
		pendingSnapshot = nil
	case <-timer.C:
		t.Fatal("player B snapshot did not finish within 2 seconds")
	case <-ctx.Done():
		t.Fatalf("player B snapshot context ended: %v", ctx.Err())
	}
	timer.Stop()
	if snapshot.err != nil || snapshot.response == nil || snapshot.response.Line != 1 {
		t.Fatalf("player B snapshot: %+v, %v", snapshot.response, snapshot.err)
	}
	remaining := make(map[string]int64)
	for _, resource := range snapshot.response.Resources {
		remaining[resource.PointID] = resource.Remaining
	}
	if value, ok := remaining[attempts[0].PosID]; !ok || value != 3 {
		t.Fatal("player B snapshot did not include player A's locked point with its original stock")
	}
	if value, ok := remaining[attempts[1].PosID]; !ok || value != 2 {
		t.Fatal("player B snapshot did not include player B's updated point")
	}

	pendingB = startClaim(1)
	replay := waitClaim(pendingB, "player B replay")
	pendingB = nil
	if replay.err != nil || !reflect.DeepEqual(first.response, replay.response) {
		t.Fatalf("player B replay changed its result: %+v, %v", replay.response, replay.err)
	}
	if count := countFact(1); count != 1 {
		t.Fatalf("player B replay left %d gather facts", count)
	}

	pendingA = startClaim(0)
	timer = time.NewTimer(100 * time.Millisecond)
	select {
	case result := <-pendingA:
		pendingA = nil
		t.Fatalf("player A claim completed while its point was locked: %+v, %v", result.response, result.err)
	case <-timer.C:
	case <-ctx.Done():
		t.Fatalf("player A wait context ended: %v", ctx.Err())
	}
	timer.Stop()
	if err := txA.Commit().Error; err != nil {
		t.Fatalf("release player A point lock: %v", err)
	}
	locked = false
	resultA := waitClaim(pendingA, "player A claim")
	pendingA = nil
	if resultA.err != nil || resultA.response == nil {
		t.Fatalf("player A claim: %+v, %v", resultA.response, resultA.err)
	}

	for i := range attempts {
		point := readPoint(i)
		if point.Remaining != 2 || point.ReservedBy != 0 || point.AttemptToken != "" {
			t.Fatalf("unexpected committed resource point %d: %+v", i, point)
		}
		var after db.P8GatherAttempt
		if err := database.WithContext(ctx).First(&after, "player_id=? AND token=?", f.ids[i], attempts[i].Token).Error; err != nil {
			t.Fatalf("read committed attempt %d: %v", i, err)
		}
		if after.Active {
			t.Fatalf("gather attempt %d remains active", i)
		}
		if count := countFact(i); count != 1 {
			t.Fatalf("player %d has %d gather facts", i, count)
		}
	}
}
