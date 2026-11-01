package service

import (
	"context"
	"database/sql"
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

// The query connector is also used by the shared-read tests; no server is
// contacted while checking the generated MySQL lock strengths.
func TestGGOPeriodBeforeLineLockModes(t *testing.T) {
	connector := &p1SharedQueryConnector{}
	connection := sql.OpenDB(connector)
	t.Cleanup(func() { _ = connection.Close() })
	database, err := gorm.Open(mysql.New(mysql.Config{Conn: connection, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = ggoLockPeriodsBeforeLine(database, &GameService{}); err != nil || len(connector.queries) != 0 {
		t.Fatalf("an uninitialized P1 fixture should issue no activity query: %v %v", connector.queries, err)
	}
	shared := database.WithContext(context.WithValue(context.Background(), constUtil.P1SharedReadContextKey{}, true))
	for i := 0; i < 2; i++ {
		if err = ggoLockPeriodsBeforeLine(shared, &GameService{P1Ready: true}); err != nil {
			t.Fatal(err)
		}
	}
	if err = ggoLockPeriodsBeforeLine(database, &GameService{P1Ready: true}); err != nil {
		t.Fatal(err)
	}
	if len(connector.queries) != 3 || !strings.HasSuffix(connector.queries[0], "FOR SHARE") ||
		!strings.HasSuffix(connector.queries[1], "FOR SHARE") || !strings.HasSuffix(connector.queries[2], "FOR UPDATE") {
		t.Fatalf("unexpected activity lock strengths: %v", connector.queries)
	}
}

type ggoPeriodLockFixture struct {
	*ggoTradeTestFixture
	starts [2]*model.GGOFieldGatherStartResponse
}

func ggoPeriodLockSetup(t *testing.T) *ggoPeriodLockFixture {
	t.Helper()
	f := &ggoPeriodLockFixture{ggoTradeTestFixture: ggoTradeSetup(t)}
	if err := db.GetIns().AutoMigrate(&db.GGOFieldLine{}, &db.GemCraftState{}, &db.CookingCraftState{}); err != nil {
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
	if err := database.First(&beforeProfile, f.ids[1]).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.First(&beforeLine, "server_id=? AND line=?", f.s.OriginalServerID(), 1).Error; err != nil {
		t.Fatal(err)
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
	var attempt db.P8GatherAttempt
	if err := database.First(&attempt, "player_id=?", f.ids[1]).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(beforeProfile, afterProfile) || !reflect.DeepEqual(beforeLine, afterLine) || !attempt.Active ||
		p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.ids[1], "ggo:gather:"+claim.AttemptToken) != 0 ||
		p1Count(t, &db.GGOFieldReceipt{}, "player_id=? AND request_id=?", f.ids[1], claim.RequestID) != 0 {
		t.Fatal("failed receipt did not roll back profile, line, attempt, fact and receipt")
	}
	first, err := f.s.GGOFieldGatherClaim(f.ctx, claim, f.sessions[1])
	if err != nil {
		t.Fatalf("claim retry: %v", err)
	}
	again, err := f.s.GGOFieldGatherClaim(f.ctx, claim, f.sessions[1])
	if err != nil || !reflect.DeepEqual(first, again) ||
		p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.ids[1], "ggo:gather:"+claim.AttemptToken) != 1 ||
		p1Count(t, &db.GGOFieldReceipt{}, "player_id=? AND request_id=?", f.ids[1], claim.RequestID) != 1 {
		t.Fatalf("claim replay did not retain one result and fact: %+v %+v %v", first, again, err)
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

// This test must use a separately coordinated local MySQL lease. A holds the
// activity SHARE lock and then cancels on the line. B concurrently claims on
// that same line; it must wait for activity X before obtaining the line X lock.
func TestGGOPeriodLineRealMySQLSharedCancelAgainstClaim(t *testing.T) {
	name := os.Getenv("HYJS_P1_MYSQL_DATABASE")
	if name == "" {
		t.Skip("requires a coordinated isolated hyjs_test_p1_ MySQL database")
	}
	if !strings.HasPrefix(name, "hyjs_test_p1_") || os.Getenv("HYJS_GAME_ACCOUNTS_DATABASE") != "" {
		t.Fatal("requires an isolated game and accounts fixture")
	}
	f := ggoPeriodLockSetup(t)
	f.setDue(t, 1)
	database := db.GetIns()
	pool, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(6)
	pool.SetMaxIdleConns(6)

	ctx, cancel := context.WithTimeout(f.ctx, 12*time.Second)
	defer cancel()
	sharedReady, startCancel := make(chan struct{}), make(chan struct{})
	sharedDone := make(chan error, 1)
	claimReady := make(chan struct{})
	claimDone := make(chan error, 1)
	var signalOnce sync.Once
	const callback = "test:ggo-period-mysql-wait"
	if err = database.Callback().Row().Before("gorm:row").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Context.Value(ggoLockTestContextKey{}) == "claim" && strings.Contains(tx.Statement.SQL.String(), "p1_period_instances") && strings.Contains(tx.Statement.SQL.String(), "FOR UPDATE") {
			signalOnce.Do(func() { close(claimReady) })
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Callback().Row().Remove(callback) })
	go func() {
		move := context.WithValue(ctx, constUtil.GameRequestContextKey, &pb.CSMove{})
		move = context.WithValue(move, constUtil.GameServiceContextKey, f.s)
		err := updateFieldProfileWithTx(move, f.ids[0], func(tx *gorm.DB, _ *pb.Player, game *model.GameState) error {
			if !p1SharedRead(tx) {
				return errors.New("movement did not hold shared activity locks")
			}
			close(sharedReady)
			select {
			case <-startCancel:
			case <-ctx.Done():
				return ctx.Err()
			}
			var attempt db.P8GatherAttempt
			if err := tx.First(&attempt, "player_id=? AND token=? AND active=?", f.ids[0], f.starts[0].AttemptToken, true).Error; err != nil {
				return err
			}
			req := f.cancelRequest(0)
			_, err := ggoCancelAttempt(tx, f.s, f.ids[0], game, &attempt, req.RequestID, ggoCancelHash(req.StartRequestID, req.AttemptToken), time.Now().UnixMilli())
			return err
		})
		sharedDone <- err
	}()
	select {
	case <-sharedReady:
	case err := <-sharedDone:
		t.Fatalf("shared movement failed before cancellation: %v", err)
	case <-ctx.Done():
		t.Fatal("shared movement did not obtain activity locks")
	}
	go func() {
		claimCtx := context.WithValue(ctx, ggoLockTestContextKey{}, "claim")
		_, err := f.s.GGOFieldGatherClaim(claimCtx, f.claimRequest(1), f.sessions[1])
		claimDone <- err
	}()
	select {
	case <-claimReady:
	case err := <-claimDone:
		t.Fatalf("claim ended before attempting the activity lock: %v", err)
	case <-ctx.Done():
		t.Fatal("claim did not attempt its activity lock")
	}
	close(startCancel)
	select {
	case err := <-sharedDone:
		if err != nil {
			t.Fatalf("shared cancellation could not complete ahead of claim: %v", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("claim retained the line while waiting for the shared activity lock")
	}
	select {
	case err := <-claimDone:
		if err != nil {
			t.Fatalf("claim failed after cancellation released its locks: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("claim did not complete after activity lock release")
	}
	for i := range f.ids {
		var attempt db.P8GatherAttempt
		if err := database.First(&attempt, "player_id=?", f.ids[i]).Error; err != nil || attempt.Active {
			t.Fatalf("player %d attempt did not commit exactly once: %+v %v", f.ids[i], attempt, err)
		}
	}
	if p1Count(t, &db.GGOFieldReceipt{}, "player_id=? AND request_id=?", f.ids[0], f.cancelRequest(0).RequestID) != 1 ||
		p1Count(t, &db.GGOFieldReceipt{}, "player_id=? AND request_id=?", f.ids[1], f.claimRequest(1).RequestID) != 1 ||
		p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.ids[1], "ggo:gather:"+f.starts[1].AttemptToken) != 1 {
		t.Fatal("concurrent cancellation or claim lost its exactly-once receipt or fact")
	}
}
