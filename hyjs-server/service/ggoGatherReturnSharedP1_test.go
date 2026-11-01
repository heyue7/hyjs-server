package service

import (
	"context"
	"errors"
	"os"
	"reflect"
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

type ggoReturnFailureTestKey struct{}

func holdGatherSharedP1(t *testing.T, ctx context.Context) func() {
	t.Helper()
	acquired, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		shared := context.WithValue(ctx, constUtil.P1SharedReadContextKey{}, true)
		done <- db.GetIns().WithContext(shared).Transaction(func(tx *gorm.DB) error {
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
	case err := <-done:
		t.Fatalf("shared holder failed: %v", err)
	case <-ctx.Done():
		t.Fatal("shared holder timed out")
	}
	var once sync.Once
	finish := func() {
		once.Do(func() {
			close(release)
			if err := <-done; err != nil {
				t.Errorf("shared holder failed: %v", err)
			}
		})
	}
	t.Cleanup(finish)
	return finish
}

func TestGGOGatherSharedStartCancelRealMySQL(t *testing.T) {
	if os.Getenv("HYJS_P1_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p1_ MySQL database")
	}
	f := ggoPeriodLockSetup(t)
	ctx, cancel := context.WithTimeout(f.ctx, 30*time.Second)
	defer cancel()
	if _, err := f.s.GGOFieldGatherCancel(ctx, f.cancelRequest(0), f.sessions[0]); err != nil {
		t.Fatal(err)
	}
	database := db.GetIns()
	previous := database.Config.Logger
	trace := &loginPeriodLockTrace{Interface: previous}
	database.Config.Logger = trace
	defer func() { database.Config.Logger = previous }()
	finish := holdGatherSharedP1(t, ctx)
	defer finish()
	trace.shared.Store(0)
	trace.exclusive.Store(0)
	request := model.GGOFieldGatherStartRequest{
		GGOFieldMutation: model.GGOFieldMutation{GGOFieldAuth: f.auth(0), RequestID: "00000000-0000-4000-8000-000000000731"},
		PointID:          f.starts[0].PointID,
		Generation:       f.starts[0].Generation,
	}
	type startResult struct {
		response *model.GGOFieldGatherStartResponse
		err      error
	}
	started := make(chan startResult, 1)
	go func() {
		response, err := f.s.GGOFieldGatherStart(ctx, request, f.sessions[0])
		started <- startResult{response, err}
	}()
	var response *model.GGOFieldGatherStartResponse
	select {
	case result := <-started:
		if result.err != nil || result.response == nil || result.response.AttemptToken == "" {
			t.Fatalf("start failed: %+v, %v", result.response, result.err)
		}
		response = result.response
	case <-time.After(4 * time.Second):
		t.Fatal("desert start blocked behind another shared activity reader")
	}
	timer, ok := f.s.p10ClientOwner().ggoGatherTimers.Load(f.ids[0])
	if !ok {
		t.Fatal("committed start did not schedule its timer")
	}
	repeated, err := f.s.GGOFieldGatherStart(ctx, request, f.sessions[0])
	afterReplay, stillScheduled := f.s.p10ClientOwner().ggoGatherTimers.Load(f.ids[0])
	if err != nil || !reflect.DeepEqual(response, repeated) || !stillScheduled || afterReplay != timer {
		t.Fatalf("start replay changed receipt or rescheduled timer: %+v, %v", repeated, err)
	}
	cancelRequest := model.GGOFieldGatherCancelRequest{
		GGOFieldMutation: model.GGOFieldMutation{GGOFieldAuth: f.auth(0), RequestID: "00000000-0000-4000-8000-000000000732"},
		StartRequestID:   request.RequestID,
		AttemptToken:     response.AttemptToken,
	}
	type cancelResult struct {
		response *model.GGOFieldGatherCancelResponse
		err      error
	}
	cancelled := make(chan cancelResult, 1)
	go func() {
		result, err := f.s.GGOFieldGatherCancel(ctx, cancelRequest, f.sessions[0])
		cancelled <- cancelResult{result, err}
	}()
	var cancelResponse *model.GGOFieldGatherCancelResponse
	select {
	case result := <-cancelled:
		if result.err != nil || result.response == nil || result.response.Status != "cancelled" {
			t.Fatalf("cancel failed: %+v, %v", result.response, result.err)
		}
		cancelResponse = result.response
	case <-time.After(4 * time.Second):
		t.Fatal("desert cancel blocked behind another shared activity reader")
	}
	replayed, err := f.s.GGOFieldGatherCancel(ctx, cancelRequest, f.sessions[0])
	if err != nil || !reflect.DeepEqual(cancelResponse, replayed) {
		t.Fatalf("cancel replay changed receipt: %+v, %v", replayed, err)
	}
	if trace.shared.Load() == 0 || trace.exclusive.Load() != 0 {
		t.Fatalf("unexpected activity locks: shared=%d exclusive=%d", trace.shared.Load(), trace.exclusive.Load())
	}
	var attempt db.P8GatherAttempt
	if err := database.First(&attempt, "player_id=?", f.ids[0]).Error; err != nil || attempt.Active || attempt.Token != response.AttemptToken {
		t.Fatalf("cancelled attempt not persisted once: %+v, %v", attempt, err)
	}
	// A cancellation arriving after the deadline must roll back the shared
	// attempt before its claim records a fact with the original exclusive lock.
	request.RequestID = "00000000-0000-4000-8000-000000000733"
	due, err := f.s.GGOFieldGatherStart(ctx, request, f.sessions[0])
	if err != nil || due == nil {
		t.Fatalf("second start failed: %+v, %v", due, err)
	}
	if err = database.Model(&db.P8GatherAttempt{}).Where("player_id=? AND token=?", f.ids[0], due.AttemptToken).Update("ready_at", time.Now().Add(-time.Second).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	late := model.GGOFieldGatherCancelRequest{
		GGOFieldMutation: model.GGOFieldMutation{GGOFieldAuth: f.auth(0), RequestID: "00000000-0000-4000-8000-000000000734"},
		StartRequestID:   request.RequestID,
		AttemptToken:     due.AttemptToken,
	}
	lateResult := make(chan cancelResult, 1)
	go func() {
		result, err := f.s.GGOFieldGatherCancel(ctx, late, f.sessions[0])
		lateResult <- cancelResult{result, err}
	}()
	select {
	case result := <-lateResult:
		t.Fatalf("late cancel did not wait for exclusive retry: %+v, %v", result.response, result.err)
	case <-time.After(250 * time.Millisecond):
	}
	if count := p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.ids[0], "ggo:gather:"+due.AttemptToken); count != 0 {
		t.Fatalf("uncommitted late claim leaked %d facts", count)
	}
	finish()
	result := <-lateResult
	if result.err != nil || result.response == nil || result.response.Status != "settled" || result.response.ClaimReceipt == nil {
		t.Fatalf("late cancel did not recover the settled claim: %+v, %v", result.response, result.err)
	}
	lateReplay, err := f.s.GGOFieldGatherCancel(ctx, late, f.sessions[0])
	if err != nil || !reflect.DeepEqual(result.response, lateReplay) || p1Count(t, &db.EventFact{}, "player_id=? AND source_key=?", f.ids[0], "ggo:gather:"+due.AttemptToken) != 1 {
		t.Fatalf("late claim replay changed the receipt or fact: %+v, %v", lateReplay, err)
	}
}

func TestGGOFieldReturnSharedP1RealMySQL(t *testing.T) {
	if os.Getenv("HYJS_P1_MYSQL_DATABASE") == "" {
		t.Skip("requires an isolated hyjs_test_p1_ MySQL database")
	}
	f := ggoTradeSetup(t)
	if err := db.GetIns().AutoMigrate(&db.FieldBossContribution73{}); err != nil {
		t.Fatal(err)
	}
	requestID := "00000000-0000-4000-8000-000000000791"
	prepared, err := f.s.GGOFieldLeavePrepare(f.ctx, model.GGOFieldLeavePrepareRequest{
		GGOFieldMutation: model.GGOFieldMutation{GGOFieldAuth: f.auth(0), RequestID: requestID}, Mode: "scene", TargetScene: "town",
	}, f.sessions[0])
	if err != nil || prepared.Status != "prepared" {
		t.Fatalf("leave preparation failed: %+v, %v", prepared, err)
	}
	runtime := f.s.withRuntimeTables(f.s.Tables)
	runtime.GGOFieldEconomy = f.config
	ctx, cancel := context.WithTimeout(context.WithValue(f.ctx, constUtil.GameServiceContextKey, runtime), 20*time.Second)
	defer cancel()
	database := db.GetIns()
	forced := errors.New("forced desert return profile failure")
	const callback = "test:ggo-return-shared-profile-failure"
	if err := database.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "profiles" && tx.Statement.Context.Value(ggoReturnFailureTestKey{}) == true {
			tx.AddError(forced)
		}
	}); err != nil {
		t.Fatal(err)
	}
	failedCtx := context.WithValue(ctx, ggoReturnFailureTestKey{}, true)
	if _, err := runtime.fieldTransfer(failedCtx, f.ids[0], &pb.CSTransferScene{Target: proto.String("town")}); !errors.Is(err, forced) {
		t.Fatalf("forced return failure was lost: %v", err)
	}
	if err := database.Callback().Update().Remove(callback); err != nil {
		t.Fatal(err)
	}
	if count := p1Count(t, &db.GGOFieldReceipt{}, "player_id=? AND operation=? AND business_key=?", f.ids[0], constUtil.GGOFieldOpLeaveComplete, requestID); count != 0 {
		t.Fatalf("failed return persisted %d completion receipts", count)
	}
	var original db.Profile
	if err := database.First(&original, f.ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	_, originalGame, err := decodeProfile(&original)
	if err != nil || originalGame.Position.GetScene() != constUtil.GGOFieldScene || originalGame.GGOField.Chips[0] != 10 || ggoCreditBalance(originalGame) != 0 {
		t.Fatalf("failed return changed the saved scene or credit: %v", err)
	}
	previous := database.Config.Logger
	trace := &loginPeriodLockTrace{Interface: previous}
	database.Config.Logger = trace
	defer func() { database.Config.Logger = previous }()
	finish := holdGatherSharedP1(t, ctx)
	defer finish()
	trace.shared.Store(0)
	trace.exclusive.Store(0)
	done := make(chan error, 2)
	gate, err := fieldPoint("41.11,-2.25,-42.33")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		go func() {
			_, err := runtime.fieldTransfer(ctx, f.ids[0], &pb.CSTransferScene{Target: proto.String("town"), Point: proto.Clone(gate).(*pb.PointNew)})
			done <- err
		}()
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("desert return blocked behind another shared activity reader")
	}
	if trace.shared.Load() == 0 || trace.exclusive.Load() != 0 {
		t.Fatalf("unexpected activity locks: shared=%d exclusive=%d", trace.shared.Load(), trace.exclusive.Load())
	}
	finish()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("concurrent return failed: %v", err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("concurrent return did not complete after the window lock was released")
	}
	status, err := f.s.GGOFieldLeaveStatus(ctx, model.GGOFieldLeaveStatusRequest{GGOFieldAuth: f.auth(0), LeaveRequestID: requestID}, f.sessions[0])
	if err != nil || status.Status != "completed" || status.Settlement == nil || status.Settlement.CreditAdded != 30 || status.Settlement.CreditBalance != 30 {
		t.Fatalf("desert settlement or receipt mismatch: %+v, %v", status, err)
	}
	if count := p1Count(t, &db.GGOFieldReceipt{}, "player_id=? AND operation=? AND business_key=?", f.ids[0], constUtil.GGOFieldOpLeaveComplete, requestID); count != 1 {
		t.Fatalf("leave receipt count=%d", count)
	}
	// Reconnect the isolated database and recover through the public status API.
	pool, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err = pool.Close(); err != nil {
		t.Fatal(err)
	}
	config := db.MySQLConfig{Host: os.Getenv("HYJS_P1_MYSQL_HOST"), User: os.Getenv("HYJS_P1_MYSQL_USER"), Password: os.Getenv("HYJS_P1_MYSQL_PASSWORD"), Database: os.Getenv("HYJS_P1_MYSQL_DATABASE")}
	if err = db.InitGameMySQL(ctx, config); err != nil {
		t.Fatal(err)
	}
	recovered, err := f.s.GGOFieldLeaveStatus(ctx, model.GGOFieldLeaveStatusRequest{GGOFieldAuth: f.auth(0), LeaveRequestID: requestID}, f.sessions[0])
	if err != nil || recovered.Status != "completed" || recovered.Settlement == nil || recovered.Settlement.CreditAdded != 30 || recovered.Settlement.CreditBalance != 30 {
		t.Fatalf("cold leave recovery changed the receipt: %+v, %v", recovered, err)
	}
}
