package service

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestCopyRunRecoveryTokenAndLegacyJSON(t *testing.T) {
	var saved struct {
		Run *model.CopyRun `json:"run"`
	}
	if err := json.Unmarshal([]byte(`{"run":{"chapter":"1","mission":"1","passed":[],"complete":false}}`), &saved); err != nil {
		t.Fatal(err)
	}
	status, err := copyRunStatus(saved.Run)
	if err != nil || !status.Active || status.ChapterID != "1" || status.MissionID != "1" || !ggoResumeHex(status.RunToken, 64) {
		t.Fatal("legacy run status", status, err)
	}
	raw, err := json.Marshal(saved.Run)
	if err != nil || strings.Contains(string(raw), "runId") || status.RunToken != p6Digest(raw) {
		t.Fatal("legacy token or omitted generation changed", err)
	}
	saved.Run.Passed = append(saved.Run.Passed, "1")
	advanced, err := copyRunStatus(saved.Run)
	if err != nil || advanced.RunToken == status.RunToken {
		t.Fatal("progress did not change token", err)
	}
	saved.Run.Complete = true
	complete, err := copyRunStatus(saved.Run)
	if err != nil || *complete != (model.CopyRunRecoveryResponse{}) {
		t.Fatal("completed run was reported active", complete, err)
	}
	empty, err := copyRunStatus(nil)
	if err != nil || *empty != (model.CopyRunRecoveryResponse{}) {
		t.Fatal("missing run was reported active", empty, err)
	}
}

// copyRunRecoveryTransactions exercises the original entry and fight methods,
// the authenticated HTTP service boundary, and the original leave dispatcher.
// The caller selects the isolated SQLite or managed MySQL fixture.
func copyRunRecoveryTransactions(t *testing.T) {
	t.Helper()
	f := p1Setup(t)
	for _, initialize := range []func(context.Context) error{
		f.s.InitializeCopyDrops, f.s.InitializeDailyCopy, f.s.InitCooking, f.s.InitializeP6,
	} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	requestContext := func(request proto.Message) context.Context {
		ctx := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
		return context.WithValue(ctx, constUtil.GameRequestContextKey, request)
	}
	if err := updateProfile(requestContext(&pb.CSCopyLeave{}), f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Level = proto.Int32(100)
		p.Energy, p.Stamina = proto.Int32(1000), proto.Int32(1000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	readProfile := func() db.Profile {
		t.Helper()
		var row db.Profile
		if err := db.GetIns().First(&row, f.id).Error; err != nil {
			t.Fatal(err)
		}
		return row
	}
	initial := readProfile()
	session := append([]byte(nil), initial.Session...)
	auth := model.GGOFieldAuth{PlayerID: f.id, SessionHex: hex.EncodeToString(session)}
	status := func() *model.CopyRunRecoveryResponse {
		t.Helper()
		out, err := f.s.CopyRunStatus(f.ctx, auth, session)
		if err != nil {
			t.Fatal("status", err)
		}
		return out
	}
	enter := func() *model.CopyRun {
		t.Helper()
		request := &pb.CSCopyEnter{ChapterId: proto.String("1"), MissionId: proto.String("1")}
		if _, err := f.s.copyEnter(requestContext(request), f.id, request); err != nil {
			t.Fatal("original entry", err)
		}
		_, game := f.profile(t)
		if game.Run == nil || len(game.Run.RunID) != 32 || !ggoResumeHex(game.Run.RunID, 32) {
			t.Fatal("entry did not persist a random 128-bit run identity")
		}
		return game.Run
	}
	start := func() db.P6BattleRecord {
		t.Helper()
		request := &pb.CSCopyFightStart{ChapterId: proto.String("1"), MissionId: proto.String("1"), TaskId: proto.String("1")}
		messages, err := f.s.copyFightStart(requestContext(request), f.id, request)
		if err != nil || len(messages) != 1 {
			t.Fatal("original fight start", err)
		}
		frame, ok := messages[0].(*pb.SCCopyFightStart)
		if !ok || len(frame.GetBattleMonsterList()) != 1 {
			t.Fatal("fixture needs a single-group original fight")
		}
		var record db.P6BattleRecord
		if err := db.GetIns().Where("player_id=? AND source=? AND source_fight=?", f.id, "copy", frame.GetFightId()).First(&record).Error; err != nil {
			t.Fatal("original P6 capture", err)
		}
		return record
	}
	readRecord := func(id string) db.P6BattleRecord {
		t.Helper()
		var record db.P6BattleRecord
		if err := db.GetIns().First(&record, "id=?", id).Error; err != nil {
			t.Fatal(err)
		}
		return record
	}
	assertProfileSame := func(before, after db.Profile) {
		t.Helper()
		if before.Player == nil || after.Player == nil || before.Game == nil || after.Game == nil ||
			*before.Player != *after.Player || *before.Game != *after.Game {
			t.Fatal("read or rejected request changed player/game bytes")
		}
	}
	assertOnlyRunRemoved := func(before, after db.Profile) {
		t.Helper()
		if before.Player == nil || after.Player == nil || before.Game == nil || after.Game == nil {
			t.Fatal("missing player/game")
		}
		var leftPlayer, rightPlayer pb.Player
		var leftGame, rightGame map[string]json.RawMessage
		if json.Unmarshal([]byte(*before.Player), &leftPlayer) != nil || json.Unmarshal([]byte(*after.Player), &rightPlayer) != nil ||
			json.Unmarshal([]byte(*before.Game), &leftGame) != nil || json.Unmarshal([]byte(*after.Game), &rightGame) != nil {
			t.Fatal("invalid persisted fixture")
		}
		if _, ok := leftGame["run"]; !ok {
			t.Fatal("expected a Run before leave")
		}
		if _, ok := rightGame["run"]; ok {
			t.Fatal("Run survived leave")
		}
		delete(leftGame, "run")
		delete(rightGame, "run")
		if !proto.Equal(&leftPlayer, &rightPlayer) || !reflect.DeepEqual(leftGame, rightGame) {
			t.Fatal("leave changed another player or game field")
		}
	}
	assertConflict := func(token string) {
		t.Helper()
		before := readProfile()
		_, err := f.s.CopyRunLeave(f.ctx, auth, session, token)
		var business *model.BizCode
		if !errors.As(err, &business) || !strings.Contains(business.Msg, "副本进度已变化") {
			t.Fatal("changed Run did not return an explicit conflict", err)
		}
		assertProfileSame(before, readProfile())
	}
	leave := func(token string) {
		t.Helper()
		before := readProfile()
		out, err := f.s.CopyRunLeave(f.ctx, auth, session, token)
		if err != nil || out == nil || *out != (model.CopyRunRecoveryResponse{}) {
			t.Fatal("explicit leave", out, err)
		}
		assertOnlyRunRemoved(before, readProfile())
	}

	first := enter()
	firstToken := status().RunToken
	paid, _ := f.profile(t)
	reentered := enter()
	afterReentry, _ := f.profile(t)
	if reentered.RunID != first.RunID || !proto.Equal(paid, afterReentry) || status().RunToken != firstToken {
		t.Fatal("same Run entry changed the generation or charged again")
	}
	record := start()
	pending := status()
	if !pending.Active || pending.RunToken == firstToken || record.Status != constUtil.P6BattleCaptured {
		t.Fatal("pending fight did not change the Run")
	}
	assertConflict(firstToken)
	beforeRead := readProfile()
	if got := status(); *got != *pending {
		t.Fatal("pending status changed between reads")
	}
	assertProfileSame(beforeRead, readProfile())
	badSession := append([]byte(nil), session...)
	badSession[0] ^= 1
	badAuth := model.GGOFieldAuth{PlayerID: f.id, SessionHex: hex.EncodeToString(badSession)}
	if _, err := f.s.CopyRunStatus(f.ctx, badAuth, badSession); err == nil {
		t.Fatal("wrong session read Run")
	}
	if _, err := f.s.CopyRunLeave(f.ctx, badAuth, badSession, pending.RunToken); err == nil {
		t.Fatal("wrong session left Run")
	}
	oldServer := f.s.ServerID
	f.s.ServerID = f.s.OriginalServerID() + 1
	_, statusErr := f.s.CopyRunStatus(f.ctx, auth, session)
	_, leaveErr := f.s.CopyRunLeave(f.ctx, auth, session, pending.RunToken)
	f.s.ServerID = oldServer
	if statusErr == nil || leaveErr == nil {
		t.Fatal("foreign realm accepted")
	}
	assertProfileSame(beforeRead, readProfile())

	// An observed record cannot be cancelled. Its failure must roll back the
	// entire explicit leave, including any profile recovery performed first.
	if err := db.GetIns().Model(&db.P6BattleRecord{}).Where("id=?", record.ID).Update("status", constUtil.P6BattleObserved).Error; err != nil {
		t.Fatal(err)
	}
	beforeFailure := readProfile()
	if _, err := f.s.CopyRunLeave(f.ctx, auth, session, pending.RunToken); err == nil {
		t.Fatal("uncancellable replay accepted")
	}
	assertProfileSame(beforeFailure, readProfile())
	if got := readRecord(record.ID); got.Status != constUtil.P6BattleObserved {
		t.Fatal("failed cancellation changed the P6 record")
	}
	if err := db.GetIns().Model(&db.P6BattleRecord{}).Where("id=?", record.ID).Update("status", constUtil.P6BattleCaptured).Error; err != nil {
		t.Fatal(err)
	}
	leave(pending.RunToken)
	if got := readRecord(record.ID); got.Status != constUtil.P6BattleCancelled {
		t.Fatal("pending P6 record was not cancelled", got.Status)
	}
	again, err := f.s.CopyRunLeave(f.ctx, auth, session, pending.RunToken)
	if err != nil || again == nil || again.Active {
		t.Fatal("repeated leave did not succeed", err)
	}

	second := enter()
	secondStatus := status()
	if first.RunID == second.RunID || secondStatus.RunToken == firstToken {
		t.Fatal("new Run reused an earlier generation/token")
	}
	assertConflict(firstToken)
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, game *model.GameState) error {
		game.Run.Passed = append(game.Run.Passed, "1")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	assertConflict(secondStatus.RunToken)
	beforeNative := readProfile()
	native := &pb.CSCopyLeave{}
	messages, err := f.s.dispatch(requestContext(native), &model.GamePacket{PlayerId: f.id, ZoneId: int32(f.s.OriginalServerID()), Session: session, Body: native}, "")
	if err != nil || len(messages) != 1 {
		t.Fatal("native leave", err)
	}
	if _, ok := messages[0].(*pb.SCCopyLeave); !ok {
		t.Fatal("native leave response changed")
	}
	assertOnlyRunRemoved(beforeNative, readProfile())

	enter()
	expiredRecord := start()
	if err := db.GetIns().Model(&db.P6BattleRecord{}).Where("id=?", expiredRecord.ID).Update("expires_at", time.Now().Add(-time.Minute).UnixMilli()).Error; err != nil {
		t.Fatal(err)
	}
	beforeExpired := readProfile()
	expiredStatus := status()
	if !expiredStatus.Active || readRecord(expiredRecord.ID).Status != constUtil.P6BattleCaptured {
		t.Fatal("status expired or cleared an existing P6 fight")
	}
	assertProfileSame(beforeExpired, readProfile())
	leave(expiredStatus.RunToken)
	if got := readRecord(expiredRecord.ID); got.Status != constUtil.P6BattleExpired {
		t.Fatal("expired pending record was not closed as expired", got.Status)
	}

	enter()
	nativeRecord := start()
	beforeNative = readProfile()
	messages, err = f.s.dispatch(requestContext(native), &model.GamePacket{PlayerId: f.id, ZoneId: int32(f.s.OriginalServerID()), Session: session, Body: native}, "")
	if err != nil || len(messages) != 1 {
		t.Fatal("native pending-fight leave", err)
	}
	if _, ok := messages[0].(*pb.SCCopyLeave); !ok {
		t.Fatal("native pending-fight response changed")
	}
	assertOnlyRunRemoved(beforeNative, readProfile())
	if got := readRecord(nativeRecord.ID); got.Status != constUtil.P6BattleCancelled {
		t.Fatal("native leave did not cancel pending P6", got.Status)
	}
}

func TestCopyRunRecoveryTransactionsSQLite(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	copyRunRecoveryTransactions(t)
}

func TestCopyRunRecoveryTransactionsMySQL(t *testing.T) {
	if !strings.HasPrefix(os.Getenv("HYJS_P1_MYSQL_DATABASE"), "hyjs_test_p1_") {
		t.Skip("requires the leased isolated P1 MySQL schema")
	}
	copyRunRecoveryTransactions(t)
}
