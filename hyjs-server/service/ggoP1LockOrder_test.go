package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type ggoNoSQLTrace struct {
	logger.Interface
	statements []string
}

func (trace *ggoNoSQLTrace) Trace(_ context.Context, _ time.Time, statement func() (string, int64), _ error) {
	query, _ := statement()
	trace.statements = append(trace.statements, query)
}

// The query connector is also used by the shared-read tests; no server is
// contacted while checking that the old global prelock issues no SQL.
func TestGGOPeriodBeforeLineNoGlobalPrelock(t *testing.T) {
	connector := &p1SharedQueryConnector{}
	connection := sql.OpenDB(connector)
	t.Cleanup(func() { _ = connection.Close() })
	trace := &ggoNoSQLTrace{Interface: logger.Default.LogMode(logger.Silent)}
	database, err := gorm.Open(mysql.New(mysql.Config{Conn: connection, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing: true, Logger: trace,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = ggoLockPeriodsBeforeLine(database, &GameService{}); err != nil || len(connector.queries) != 0 || len(trace.statements) != 0 {
		t.Fatalf("an uninitialized P1 fixture should issue no SQL: %v %v %v", connector.queries, trace.statements, err)
	}
	shared := database.WithContext(context.WithValue(context.Background(), constUtil.P1SharedReadContextKey{}, true))
	for i := 0; i < 2; i++ {
		if err = ggoLockPeriodsBeforeLine(shared, &GameService{P1Ready: true}); err != nil || len(connector.queries) != 0 || len(trace.statements) != 0 {
			t.Fatalf("shared activity prelock should issue no SQL: %v %v %v", connector.queries, trace.statements, err)
		}
	}
	if err = ggoLockPeriodsBeforeLine(database, &GameService{P1Ready: true}); err != nil || len(connector.queries) != 0 || len(trace.statements) != 0 {
		t.Fatalf("activity prelock should issue no SQL: %v %v %v", connector.queries, trace.statements, err)
	}
}

type ggoPeriodLockFixture struct {
	*ggoTradeTestFixture
	starts [2]*model.GGOFieldGatherStartResponse
}

func ggoPeriodLockSetup(t *testing.T) *ggoPeriodLockFixture {
	t.Helper()
	f := &ggoPeriodLockFixture{ggoTradeTestFixture: ggoTradeSetup(t)}
	if err := db.GetIns().AutoMigrate(&db.GGOFieldLine{}, &db.GGOFieldPoint{}, &db.GemCraftState{}, &db.CookingCraftState{}); err != nil {
		t.Fatal(err)
	}
	f.s.GGOIntegrationReady = true
	// Cancel the timer immediately after each start, then control deadlines in
	// the isolated database. This test never runs a background service.
	f.config.Runtime.Resource.MineGatherMillis = 300000
	f.config.Runtime.Resource.FishGatherMillis = 300000
	t.Cleanup(func() {
		for _, id := range f.ids {
			if task, ok := f.s.p10ClientOwner().ggoGatherTimers.LoadAndDelete(id); ok {
				task.(*ggoGatherTimer).timer.Stop()
			}
		}
		f.s.ggoStopNoticeTimers()
	})

	type point struct {
		node     ggoResourceNode
		position *model.AdminFieldGGOPosition
	}
	var points []point
	err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		row, state, _, err := ggoLoadLine(tx, f.s.OriginalServerID(), 1, f.config, "isolated-trade-config", time.Now().UnixMilli())
		if err != nil {
			return err
		}
		for i := range state.Resources {
			node := &state.Resources[i]
			if node.Kind != "mine" || len(points) == 2 {
				continue
			}
			for _, configured := range f.config.Runtime.Points {
				if configured.ID == node.PointID {
					node.Level, node.Remaining = 3, 3
					points = append(points, point{node: *node, position: configured.StandPosition})
					break
				}
			}
		}
		return ggoLineSave(tx, row, state, time.Now().UnixMilli())
	})
	if err != nil || len(points) != 2 {
		t.Fatalf("prepare two isolated resource points: %v (%d points)", err, len(points))
	}
	for i, selected := range points {
		if err := db.GetIns().Create(&db.GemCraftState{PlayerID: f.ids[i], Level: 3}).Error; err != nil {
			t.Fatal(err)
		}
		if err := updateProfile(f.ctx, f.ids[i], func(_ *pb.Player, game *model.GameState) error {
			game.Position.Current = &pb.PointNew{X: proto.Float32(float32(*selected.position.X)), Y: proto.Float32(float32(*selected.position.Y)), Z: proto.Float32(float32(*selected.position.Z))}
			game.Field.Side.Tiredness = proto.Int32(200)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		req := model.GGOFieldGatherStartRequest{
			GGOFieldMutation: model.GGOFieldMutation{GGOFieldAuth: f.auth(i), RequestID: fmt.Sprintf("00000000-0000-4000-8000-%012d", 700+i)},
			PointID:          selected.node.PointID, Generation: selected.node.Generation,
		}
		started, err := f.s.GGOFieldGatherStart(f.ctx, req, f.sessions[i])
		if err != nil || started == nil || started.AttemptToken == "" {
			t.Fatalf("start gather %d: %+v, %v", i, started, err)
		}
		f.starts[i] = started
		if task, ok := f.s.p10ClientOwner().ggoGatherTimers.LoadAndDelete(f.ids[i]); ok {
			task.(*ggoGatherTimer).timer.Stop()
		}
	}
	return f
}

func (f *ggoPeriodLockFixture) setDue(t *testing.T, i int) {
	t.Helper()
	if err := db.GetIns().Model(&db.P8GatherAttempt{}).Where("player_id=? AND token=?", f.ids[i], f.starts[i].AttemptToken).
		Update("ready_at", time.Now().Add(-time.Second).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
}

func (f *ggoPeriodLockFixture) claimRequest(i int) model.GGOFieldGatherClaimRequest {
	return model.GGOFieldGatherClaimRequest{GGOFieldMutation: model.GGOFieldMutation{GGOFieldAuth: f.auth(i), RequestID: fmt.Sprintf("00000000-0000-4000-8000-%012d", 710+i)}, StartRequestID: f.starts[i].RequestID, AttemptToken: f.starts[i].AttemptToken}
}

func (f *ggoPeriodLockFixture) cancelRequest(i int) model.GGOFieldGatherCancelRequest {
	return model.GGOFieldGatherCancelRequest{GGOFieldMutation: model.GGOFieldMutation{GGOFieldAuth: f.auth(i), RequestID: fmt.Sprintf("00000000-0000-4000-8000-%012d", 720+i)}, StartRequestID: f.starts[i].RequestID, AttemptToken: f.starts[i].AttemptToken}
}

type ggoLockTestContextKey struct{}

func TestGGOPeriodLineGatherRollbackAndReplay(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	f := ggoPeriodLockSetup(t)
	f.setDue(t, 1)
	database := db.GetIns()
	var beforeProfile, afterProfile db.Profile
	var beforeLine, afterLine db.GGOFieldLine
	var beforePoint, afterPoint db.GGOFieldPoint
	if err := database.First(&beforeProfile, f.ids[1]).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.First(&beforeLine, "server_id=? AND line=?", f.s.OriginalServerID(), 1).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.First(&beforePoint, "server_id=? AND line=? AND point_id=?", f.s.OriginalServerID(), 1, f.starts[1].PointID).Error; err != nil {
		t.Fatal(err)
	}
	var beforeState ggoPointPayload
	if err := json.Unmarshal(beforePoint.Payload, &beforeState); err != nil {
		t.Fatal(err)
	}
	if beforeState.Resource == nil || beforeState.Resource.PointID != f.starts[1].PointID || beforeState.Resource.ReservedBy != f.ids[1] || beforeState.Resource.AttemptToken != f.starts[1].AttemptToken {
		t.Fatal("claim fixture lost the accepted reservation")
	}
	forced := errors.New("injected claim receipt failure")
	const callback = "test:ggo-period-receipt-failure"
	if err := database.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Context.Value(ggoLockTestContextKey{}) == "fail-receipt" && tx.Statement.Table == "ggo_field_receipts" {
			tx.AddError(forced)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Callback().Create().Remove(callback) })
	claim := f.claimRequest(1)
	failedCtx := context.WithValue(f.ctx, ggoLockTestContextKey{}, "fail-receipt")
	if _, err := f.s.GGOFieldGatherClaim(failedCtx, claim, f.sessions[1]); !errors.Is(err, forced) {
		t.Fatalf("receipt failure was not returned: %v", err)
	}
	if err := database.First(&afterProfile, f.ids[1]).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.First(&afterLine, "server_id=? AND line=?", f.s.OriginalServerID(), 1).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.First(&afterPoint, "server_id=? AND line=? AND point_id=?", f.s.OriginalServerID(), 1, f.starts[1].PointID).Error; err != nil {
		t.Fatal(err)
	}
	var attempt db.P8GatherAttempt
	if err := database.First(&attempt, "player_id=?", f.ids[1]).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(beforeProfile, afterProfile) || !reflect.DeepEqual(beforeLine, afterLine) || !reflect.DeepEqual(beforePoint, afterPoint) || !attempt.Active ||
		p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.ids[1], "ggo:gather:"+claim.AttemptToken) != 0 ||
		p1Count(t, &db.GGOFieldReceipt{}, "player_id=? AND request_id=?", f.ids[1], claim.RequestID) != 0 {
		t.Fatal("failed receipt did not roll back profile, line, point, attempt, fact and receipt")
	}
	first, err := f.s.GGOFieldGatherClaim(f.ctx, claim, f.sessions[1])
	if err != nil {
		t.Fatalf("claim retry: %v", err)
	}
	var committedPoint db.GGOFieldPoint
	if err := database.First(&committedPoint, "server_id=? AND line=? AND point_id=?", f.s.OriginalServerID(), 1, f.starts[1].PointID).Error; err != nil {
		t.Fatal(err)
	}
	var committedState ggoPointPayload
	if err := json.Unmarshal(committedPoint.Payload, &committedState); err != nil {
		t.Fatal(err)
	}
	if committedState.Resource == nil || committedState.Resource.PointID != beforeState.Resource.PointID ||
		committedPoint.Revision != beforePoint.Revision+1 || committedState.Resource.Remaining != beforeState.Resource.Remaining-1 ||
		committedState.Resource.ReservedBy != 0 || committedState.Resource.AttemptToken != "" || committedState.Resource.ReadyAtMS != 0 {
		t.Fatal("claim did not consume the selected point exactly once")
	}
	again, err := f.s.GGOFieldGatherClaim(f.ctx, claim, f.sessions[1])
	if err != nil || !reflect.DeepEqual(first, again) ||
		p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.ids[1], "ggo:gather:"+claim.AttemptToken) != 1 ||
		p1Count(t, &db.GGOFieldReceipt{}, "player_id=? AND request_id=?", f.ids[1], claim.RequestID) != 1 {
		t.Fatalf("claim replay did not retain one result and fact: %+v %+v %v", first, again, err)
	}
	var replayPoint db.GGOFieldPoint
	if err := database.First(&replayPoint, "server_id=? AND line=? AND point_id=?", f.s.OriginalServerID(), 1, f.starts[1].PointID).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(committedPoint, replayPoint) {
		t.Fatal("claim replay changed the selected point")
	}
	cancel := f.cancelRequest(0)
	one, err := f.s.GGOFieldGatherCancel(f.ctx, cancel, f.sessions[0])
	if err != nil {
		t.Fatalf("cancel gather: %v", err)
	}
	two, err := f.s.GGOFieldGatherCancel(f.ctx, cancel, f.sessions[0])
	if err != nil || !reflect.DeepEqual(one, two) || one.Status != "cancelled" ||
		p1Count(t, &db.GGOFieldReceipt{}, "player_id=? AND request_id=?", f.ids[0], cancel.RequestID) != 1 {
		t.Fatalf("cancel replay did not retain one receipt: %+v %+v %v", one, two, err)
	}
}

// This test requires a separately coordinated local MySQL lease. A holds one
// cancelled point while B claims a different point on the same line.
func TestGGOPeriodPointRealMySQLCancelDoesNotBlockOtherClaim(t *testing.T) {
	name := os.Getenv("HYJS_P1_MYSQL_DATABASE")
	if name == "" {
		t.Skip("requires a coordinated isolated hyjs_test_p1_ MySQL database")
	}
	if !strings.HasPrefix(name, "hyjs_test_p1_") || os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" {
		t.Fatal("requires an isolated game and accounts fixture")
	}
	f := ggoPeriodLockSetup(t)
	f.setDue(t, 1)
	if f.starts[0].PointID == f.starts[1].PointID {
		t.Fatal("fixture must reserve two different points")
	}
	database := db.GetIns()
	pool, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(6)
	pool.SetMaxIdleConns(6)

	type cancelResult struct {
		response *model.GGOFieldGatherCancelResponse
		err      error
	}
	type claimResult struct {
		response *model.GGOFieldGatherClaimResponse
		err      error
	}
	ctx, cancel := context.WithTimeout(f.ctx, 12*time.Second)
	pointHeld, releasePoint := make(chan struct{}), make(chan struct{})
	cancelDone := make(chan cancelResult, 1)
	claimDone := make(chan claimResult, 1)
	var heldOnce, releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releasePoint) }) }
	cancelReceived, claimStarted, claimReceived := false, false, false
	defer func() {
		release()
		cancel()
		if !cancelReceived {
			select {
			case <-cancelDone:
			case <-time.After(5 * time.Second):
				t.Error("cancellation goroutine did not exit")
			}
		}
		if claimStarted && !claimReceived {
			select {
			case <-claimDone:
			case <-time.After(5 * time.Second):
				t.Error("claim goroutine did not exit")
			}
		}
	}()

	cancelRequest := f.cancelRequest(0)
	claimRequest := f.claimRequest(1)
	go func() {
		move := context.WithValue(ctx, constUtil.GameRequestContextKey, &pb.CSMove{})
		move = context.WithValue(move, constUtil.GameServiceContextKey, f.s)
		var response *model.GGOFieldGatherCancelResponse
		err := updateFieldProfileWithTx(move, f.ids[0], func(tx *gorm.DB, _ *pb.Player, game *model.GameState) error {
			var attempt db.P8GatherAttempt
			if err := tx.First(&attempt, "player_id=? AND token=? AND active=?", f.ids[0], f.starts[0].AttemptToken, true).Error; err != nil {
				return err
			}
			var err error
			response, err = ggoCancelAttempt(tx, f.s, f.ids[0], game, &attempt, cancelRequest.RequestID, ggoCancelHash(cancelRequest.StartRequestID, cancelRequest.AttemptToken), time.Now().UnixMilli())
			if err != nil {
				return err
			}
			heldOnce.Do(func() { close(pointHeld) })
			select {
			case <-releasePoint:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		cancelDone <- cancelResult{response: response, err: err}
	}()
	select {
	case <-pointHeld:
	case result := <-cancelDone:
		cancelReceived = true
		t.Fatalf("cancellation finished before holding its point: %v", result.err)
	case <-ctx.Done():
		t.Fatal("cancellation did not hold its point")
	}

	claimStarted = true
	go func() {
		response, err := f.s.GGOFieldGatherClaim(ctx, claimRequest, f.sessions[1])
		claimDone <- claimResult{response: response, err: err}
	}()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	var firstClaim *model.GGOFieldGatherClaimResponse
	select {
	case result := <-claimDone:
		claimReceived = true
		if result.err != nil || result.response == nil {
			t.Fatalf("other point claim failed while cancellation held its transaction: %v", result.err)
		}
		firstClaim = result.response
	case <-timer.C:
		t.Fatal("other point claim did not complete within 2 seconds")
	case <-ctx.Done():
		t.Fatal("context ended before other point claim completed")
	}
	select {
	case result := <-cancelDone:
		cancelReceived = true
		t.Fatalf("cancellation finished before its point was released: %v", result.err)
	default:
	}

	release()
	var firstCancel *model.GGOFieldGatherCancelResponse
	select {
	case result := <-cancelDone:
		cancelReceived = true
		if result.err != nil || result.response == nil || result.response.Status != "cancelled" {
			t.Fatalf("cancellation did not commit: %v", result.err)
		}
		firstCancel = result.response
	case <-time.After(4 * time.Second):
		t.Fatal("cancellation did not complete after its point was released")
	case <-ctx.Done():
		t.Fatal("context ended before cancellation completed")
	}

	type playerSnapshot struct {
		profile db.Profile
		attempt db.P8GatherAttempt
		point   db.GGOFieldPoint
	}
	snapshot := func() [2]playerSnapshot {
		t.Helper()
		var result [2]playerSnapshot
		for i := range f.ids {
			if err := database.First(&result[i].profile, f.ids[i]).Error; err != nil {
				t.Fatal(err)
			}
			if err := database.First(&result[i].attempt, "player_id=?", f.ids[i]).Error; err != nil {
				t.Fatal(err)
			}
			if err := database.First(&result[i].point, "server_id=? AND line=? AND point_id=?", f.s.OriginalServerID(), 1, f.starts[i].PointID).Error; err != nil {
				t.Fatal(err)
			}
		}
		return result
	}
	beforeReplay := snapshot()
	if beforeReplay[0].attempt.Active || beforeReplay[1].attempt.Active ||
		p1Count(t, &db.GGOFieldReceipt{}, "player_id=? AND request_id=?", f.ids[0], cancelRequest.RequestID) != 1 ||
		p1Count(t, &db.GGOFieldReceipt{}, "player_id=? AND request_id=?", f.ids[1], claimRequest.RequestID) != 1 ||
		p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.ids[1], "ggo:gather:"+f.starts[1].AttemptToken) != 1 {
		t.Fatal("concurrent cancellation or claim lost its exactly-once attempt, receipt or fact")
	}

	const callback = "test:ggo-point-replay-write"
	const replayMarker = "point-isolation-replay"
	var replayWrites []string
	recordWrite := func(tx *gorm.DB) {
		if tx.Statement.Context.Value(ggoLockTestContextKey{}) == replayMarker {
			replayWrites = append(replayWrites, tx.Statement.Table)
		}
	}
	if err := database.Callback().Create().Before("gorm:create").Register(callback, recordWrite); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Callback().Create().Remove(callback) })
	if err := database.Callback().Update().Before("gorm:update").Register(callback, recordWrite); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Callback().Update().Remove(callback) })
	if err := database.Callback().Delete().Before("gorm:delete").Register(callback, recordWrite); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Callback().Delete().Remove(callback) })

	replayCtx, replayCancel := context.WithTimeout(context.WithValue(f.ctx, ggoLockTestContextKey{}, replayMarker), 8*time.Second)
	defer replayCancel()
	cancelReplay, err := f.s.GGOFieldGatherCancel(replayCtx, cancelRequest, f.sessions[0])
	if err != nil || !reflect.DeepEqual(firstCancel, cancelReplay) {
		t.Fatalf("cancellation replay changed its result: %v", err)
	}
	claimReplay, err := f.s.GGOFieldGatherClaim(replayCtx, claimRequest, f.sessions[1])
	if err != nil || !reflect.DeepEqual(firstClaim, claimReplay) {
		t.Fatalf("claim replay changed its result: %v", err)
	}
	if len(replayWrites) != 0 {
		t.Fatalf("replays attempted writes: %v", replayWrites)
	}
	if afterReplay := snapshot(); !reflect.DeepEqual(beforeReplay, afterReplay) ||
		p1Count(t, &db.GGOFieldReceipt{}, "player_id=? AND request_id=?", f.ids[0], cancelRequest.RequestID) != 1 ||
		p1Count(t, &db.GGOFieldReceipt{}, "player_id=? AND request_id=?", f.ids[1], claimRequest.RequestID) != 1 ||
		p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.ids[1], "ggo:gather:"+f.starts[1].AttemptToken) != 1 {
		t.Fatal("replay changed a profile, point, attempt, receipt or fact")
	}
}
