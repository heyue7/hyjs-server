package service

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// returnLine86Dispatch uses the native TCP frame parser and records the same
// sink pushes followed by dispatcher replies as serveGameConnection writes.
func returnLine86Dispatch(t *testing.T, f *ggoTradeTestFixture, i int, opcode int32, body proto.Message, recorder *p10SceneRecorder) []proto.Message {
	t.Helper()
	payload, err := proto.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	raw := make([]byte, 28+len(payload))
	binary.BigEndian.PutUint32(raw[0:4], uint32(len(raw)-4))
	binary.BigEndian.PutUint32(raw[4:8], 1)
	binary.BigEndian.PutUint32(raw[8:12], uint32(opcode))
	binary.BigEndian.PutUint32(raw[12:16], uint32(f.s.OriginalServerID()))
	binary.BigEndian.PutUint64(raw[16:24], uint64(f.ids[i]))
	binary.BigEndian.PutUint32(raw[24:28], uint32(len(payload)))
	copy(raw[28:], payload)
	packet, err := model.ParseGamePacket(raw, true)
	if err != nil {
		t.Fatal(err)
	}
	packet.Session = f.sessions[i]
	original := proto.Clone(packet.Body)
	messages := f.s.Dispatch(f.ctx, packet, "127.0.0.1")
	if !proto.Equal(packet.Body, original) {
		t.Fatalf("opcode %d request was modified", opcode)
	}
	for _, message := range messages {
		if failure, ok := message.(*pb.ErrorMessage); ok {
			t.Fatalf("opcode %d rejected: %s", opcode, failure.GetMsg())
		}
		if err := recorder.send(message); err != nil {
			t.Fatal(err)
		}
	}
	return messages
}

func returnLine86Prepare(t *testing.T, f *ggoTradeTestFixture, i int, target int32, requestID string) {
	t.Helper()
	response, err := f.s.GGOFieldLeavePrepare(f.ctx, model.GGOFieldLeavePrepareRequest{
		GGOFieldMutation: model.GGOFieldMutation{GGOFieldAuth: f.auth(i), RequestID: requestID}, Mode: "line", TargetLine: target,
	}, f.sessions[i])
	if err != nil || response.Status != "prepared" {
		t.Fatalf("prepare line %d: response=%v err=%v", target, response, err)
	}
}

func returnLine86VisibilityOpcodes(t *testing.T, messages []proto.Message, playerID int64) []int32 {
	t.Helper()
	var opcodes []int32
	for _, message := range messages {
		raw, err := model.EncodeGameResponses([]proto.Message{message})
		if err != nil || len(raw) < 8 || int(binary.BigEndian.Uint32(raw[:4])) != len(raw)-8 {
			t.Fatalf("invalid original response frame: %v", err)
		}
		opcode := int32(binary.BigEndian.Uint32(raw[4:8]))
		switch opcode {
		case 30028:
			var appear pb.SCPlayerAppear
			if err := proto.Unmarshal(raw[8:], &appear); err != nil {
				t.Fatal(err)
			}
			for _, player := range appear.GetPlayer() {
				if player.GetInfo().GetPlayerId() == playerID {
					if player.GetInfo().GetServerId() <= 0 || player.GetPosition().GetPlayerId() != playerID ||
						player.GetPosition().GetScene() != "town_side_"+constUtil.GGOFieldScene || player.GetPosition().GetCurrent() == nil {
						t.Fatal("appearance is missing the original same-line identity or position")
					}
					opcodes = append(opcodes, opcode)
				}
			}
		case 30029:
			var disappear pb.SCPlayerDisappear
			if err := proto.Unmarshal(raw[8:], &disappear); err != nil {
				t.Fatal(err)
			}
			for _, id := range disappear.GetPlayerId() {
				if id == playerID {
					opcodes = append(opcodes, opcode)
				}
			}
		}
	}
	return opcodes
}

func returnLine86Setup(t *testing.T) (*ggoTradeTestFixture, [2]*p10SceneRecorder) {
	t.Helper()
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	return returnLine86SetupCurrentDB(t)
}

func returnLine86SetupCurrentDB(t *testing.T) (*ggoTradeTestFixture, [2]*p10SceneRecorder) {
	t.Helper()
	f := ggoTradeSetup(t)
	f.s.GGOIntegrationReady = true
	f.s.GGOFieldConfig, f.s.GGOFieldEconomy = f.config, f.config
	if err := db.GetIns().AutoMigrate(&db.FieldBossContribution73{}, &db.P10SocialReceipt{}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP10Scene(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P10ScenePolicy{}).Where("id=?", constUtil.P10ScenePolicyID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	recorders := [2]*p10SceneRecorder{{}, {}}
	for i, id := range f.ids {
		value, ok := p10LiveClient(p10ClientKey{f.s.p10ClientOwner(), id})
		if !ok {
			t.Fatal("fixture socket missing")
		}
		value.(*p10ClientSink).send = recorders[i].send
		if err := f.s.SyncP10Scene(f.ctx, id, false); err != nil {
			t.Fatal(err)
		}
	}
	for _, recorder := range recorders {
		recorder.drain()
	}
	return f, recorders
}

func TestP10DesertReturnLineStationaryNativeVisibility(t *testing.T) {
	for _, around := range []int32{0, 1} {
		t.Run(fmt.Sprintf("showArround=%d", around), func(t *testing.T) {
			returnLine86Stationary(t, around)
		})
	}
}

func returnLine86Stationary(t *testing.T, around int32) {
	t.Helper()
	f, recorders := returnLine86Setup(t)
	watcher, mover := f.ids[0], f.ids[1]
	var watcherMessages []proto.Message
	for step, line := range []int32{2, 1} {
		requestID := fmt.Sprintf("00000000-0000-4000-8000-%012d", step+201)
		returnLine86Prepare(t, f, 1, line, requestID)
		out := returnLine86Dispatch(t, f, 1, 30061, &pb.CSTransferLine{Line: proto.Int32(line)}, recorders[1])
		if len(out) != 1 {
			t.Fatalf("line %d returned %d replies", line, len(out))
		}
		if _, ok := out[0].(*pb.SCTransferLine); !ok {
			t.Fatalf("line %d returned %T", line, out[0])
		}
		if step == 0 {
			departure := recorders[0].drain()
			if !p10SceneHasDisappear(departure, mover) {
				t.Fatal("watcher did not receive the mover's line departure")
			}
			watcherMessages = append(watcherMessages, departure...)
		}
		// Retain socket push ordering across transfer and position responses.
		// The r12 native SCTransferLine handler sets line without showArround.
		current := &pb.CSCurrentPosition{Line: proto.Int32(line)}
		if around != 0 {
			current.ShowArround = proto.Int32(around)
		}
		returnLine86Dispatch(t, f, 1, 30024, current, recorders[1])
		if step == 0 {
			recorders[1].drain()
		}
	}
	returned := recorders[0].drain()
	watcherMessages = append(watcherMessages, returned...)
	if !p10SceneHasPlayer(returned, mover) {
		t.Errorf("stationary return did not send SCPlayerAppear to watcher: %v", returned)
	}
	if sequence := returnLine86VisibilityOpcodes(t, watcherMessages, mover); !slices.Equal(sequence, []int32{30029, 30028}) {
		t.Errorf("watcher visibility frame sequence = %v, want [30029 30028]", sequence)
	}
	var sawCurrent bool
	var peerCount int
	for _, message := range recorders[1].drain() {
		switch m := message.(type) {
		case *pb.SCCurrentPosition:
			sawCurrent = true
		case *pb.SCPlayerAppear:
			if p10SceneHasPlayer([]proto.Message{m}, watcher) {
				if !sawCurrent {
					t.Error("same-line peer reached switching socket before SCCurrentPosition")
				}
				peerCount++
			}
		}
	}
	if !sawCurrent || peerCount != 1 {
		t.Fatalf("switcher did not receive self then one peer: self=%v peers=%d", sawCurrent, peerCount)
	}
	// A subsequent ordinary request with showArround=0 is not a line reload.
	returnLine86Dispatch(t, f, 1, 30024, &pb.CSCurrentPosition{}, recorders[1])
	if messages := recorders[1].drain(); p10SceneHasPlayer(messages, watcher) {
		t.Fatalf("ordinary showArround=0 unexpectedly replayed peers: %v", messages)
	}
	if messages := recorders[0].drain(); p10SceneHasPlayer(messages, mover) || p10SceneHasDisappear(messages, mover) {
		t.Fatalf("ordinary current position repeated visibility: %v", messages)
	}
}

func returnLine86Pending(t *testing.T, f *ggoTradeTestFixture, line int32) {
	t.Helper()
	var row db.P10ScenePresence
	if err := db.GetIns().First(&row, "player_id=?", f.ids[1]).Error; err != nil {
		t.Fatal(err)
	}
	if row.Scene != constUtil.GGOFieldScene || row.Line != line || row.SessionHash != p6Digest(f.sessions[1]) || len(row.Frame) != 0 {
		t.Fatalf("pending presence lost target, session, or marker: scene=%s line=%d session=%v frameSize=%d", row.Scene, row.Line, row.SessionHash == p6Digest(f.sessions[1]), len(row.Frame))
	}
	var isNull, length int
	if err := db.GetIns().Raw("SELECT frame IS NULL, length(frame) FROM p10_scene_presence WHERE player_id=?", f.ids[1]).Row().Scan(&isNull, &length); err != nil {
		t.Fatal(err)
	}
	if isNull != 0 || length != 0 {
		t.Fatalf("pending frame must be an empty non-NULL blob: null=%d length=%d", isNull, length)
	}
}

func TestP10DesertPendingHeartbeatDisconnectColdResume(t *testing.T) {
	f, recorders := returnLine86Setup(t)
	mover := f.ids[1]
	returnLine86Prepare(t, f, 1, 2, "00000000-0000-4000-8000-000000000301")
	returnLine86Dispatch(t, f, 1, 30061, &pb.CSTransferLine{Line: proto.Int32(2)}, recorders[1])
	if !p10SceneHasDisappear(recorders[0].drain(), mover) {
		t.Fatal("watcher did not receive the original departure")
	}
	returnLine86Dispatch(t, f, 1, 30024, &pb.CSCurrentPosition{}, recorders[1])
	recorders[1].drain()
	requestID := "00000000-0000-4000-8000-000000000302"
	returnLine86Prepare(t, f, 1, 1, requestID)
	returnLine86Dispatch(t, f, 1, 30061, &pb.CSTransferLine{Line: proto.Int32(1)}, recorders[1])
	returnLine86Pending(t, f, 1)
	status, err := f.s.GGOFieldLeaveStatus(f.ctx, model.GGOFieldLeaveStatusRequest{GGOFieldAuth: f.auth(1), LeaveRequestID: requestID}, f.sessions[1])
	if err != nil || status.Status != "completed" || !status.TargetCurrent {
		t.Fatalf("pending line lost committed leave receipt: status=%v err=%v", status, err)
	}
	returnLine86Dispatch(t, f, 1, 30020, &pb.CSPingTcp{}, recorders[1])
	returnLine86Pending(t, f, 1)
	if messages := recorders[0].drain(); len(messages) != 0 {
		t.Fatalf("loading heartbeat exposed or removed pending actor: %v", messages)
	}

	key := p10ClientKey{f.s.p10ClientOwner(), mover}
	value, ok := p10LiveClient(key)
	if !ok {
		t.Fatal("live mover missing before disconnect")
	}
	sink := value.(*p10ClientSink)
	when := time.Now().UnixMilli()
	sink.disconnectedAt.Store(when)
	if err := f.s.p10DisconnectScene(f.ctx, mover, f.sessions[1], when); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.s.ggoCancelOfflineTimer(mover) })
	var disconnected db.P10ScenePresence
	if err := db.GetIns().First(&disconnected, "player_id=?", mover).Error; err != nil {
		t.Fatal(err)
	}
	if disconnected.SessionHash != "" || disconnected.OfflineUntil <= when || len(disconnected.Frame) != 0 {
		t.Fatal("disconnect did not preserve normal offline cleanup and pending frame")
	}
	p10ClientSinks.CompareAndDelete(key, sink)

	// Reopen the isolated SQLite file to ensure the pending state survives a
	// cold store and reconnect, instead of relying on a process-local flag.
	var databases []struct {
		Name string
		File string
	}
	if err := db.GetIns().Raw("PRAGMA database_list").Scan(&databases).Error; err != nil {
		t.Fatal(err)
	}
	var path string
	for _, entry := range databases {
		if entry.Name == "main" {
			path = entry.File
		}
	}
	if path == "" {
		t.Fatal("isolated SQLite file missing")
	}
	sqlDB, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	if err = sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	if err = db.InitGame(f.ctx, path); err != nil {
		t.Fatal(err)
	}
	database := db.GetIns()
	sqlDB, err = database.DB()
	if err != nil {
		t.Fatal(err)
	}
	f.s.adminFieldRuntimeCache.Store(&adminFieldRuntimeSnapshot{serverID: f.s.OriginalServerID(), database: database, sqlDatabase: sqlDB, signature: adminFieldDeploymentSignature(nil), tables: f.s.Tables, ggo: f.config, ggoEconomy: f.config, ggoRevision: "isolated-trade-config"})
	recorders[1].drain()
	closeSocket := f.s.RegisterP10Client(f.ctx, mover, f.sessions[1], recorders[1].send)
	t.Cleanup(closeSocket)
	returnLine86Dispatch(t, f, 1, 30024, &pb.CSCurrentPosition{Line: proto.Int32(1)}, recorders[1])
	if sequence := returnLine86VisibilityOpcodes(t, recorders[0].drain(), mover); !slices.Equal(sequence, []int32{30028}) {
		t.Fatalf("cold reconnect visibility sequence = %v, want [30028]", sequence)
	}
	var gotSelf, gotPeer bool
	for _, message := range recorders[1].drain() {
		switch message.(type) {
		case *pb.SCCurrentPosition:
			gotSelf = true
		case *pb.SCPlayerAppear:
			if p10SceneHasPlayer([]proto.Message{message}, f.ids[0]) {
				if !gotSelf || gotPeer {
					t.Fatal("cold reconnect peer did not follow self exactly once")
				}
				gotPeer = true
			}
		}
	}
	if !gotSelf || !gotPeer {
		t.Fatal("cold reconnect missing ordered self and peer")
	}
	if err := db.GetIns().First(&disconnected, "player_id=?", mover).Error; err != nil {
		t.Fatal(err)
	}
	if disconnected.SessionHash != p6Digest(f.sessions[1]) || disconnected.OfflineUntil != 0 || len(disconnected.Frame) == 0 {
		t.Fatal("confirmed reconnect did not restore the authenticated frame")
	}
}

func TestP10DesertPendingWriteFailureAndRetry(t *testing.T) {
	f, recorders := returnLine86Setup(t)
	returnLine86Prepare(t, f, 1, 2, "00000000-0000-4000-8000-000000000501")
	returnLine86Dispatch(t, f, 1, 30061, &pb.CSTransferLine{Line: proto.Int32(2)}, recorders[1])
	returnLine86Dispatch(t, f, 1, 30024, &pb.CSCurrentPosition{}, recorders[1])
	for _, recorder := range recorders {
		recorder.drain()
	}
	returnLine86Prepare(t, f, 1, 1, "00000000-0000-4000-8000-000000000502")
	returnLine86Dispatch(t, f, 1, 30061, &pb.CSTransferLine{Line: proto.Int32(1)}, recorders[1])
	recorders[1].drain()
	callback := "test:desert-pending-write-failure"
	updates := db.GetIns().Callback().Update()
	if err := updates.Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "p10_scene_presence" {
			tx.AddError(errors.New("isolated pending write failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	registered := true
	t.Cleanup(func() {
		if registered {
			_ = updates.Remove(callback)
		}
	})
	returnLine86Dispatch(t, f, 1, 30024, &pb.CSCurrentPosition{Line: proto.Int32(1)}, recorders[1])
	if messages := recorders[0].drain(); len(messages) != 0 {
		t.Fatalf("failed presence write published a visibility frame: %v", messages)
	}
	if messages := recorders[1].drain(); p10SceneHasPlayer(messages, f.ids[0]) {
		t.Fatalf("failed presence write reported a restored peer snapshot: %v", messages)
	}
	returnLine86Pending(t, f, 1)
	if err := updates.Remove(callback); err != nil {
		t.Fatal(err)
	}
	registered = false
	returnLine86Dispatch(t, f, 1, 30024, &pb.CSCurrentPosition{Line: proto.Int32(1)}, recorders[1])
	if sequence := returnLine86VisibilityOpcodes(t, recorders[0].drain(), f.ids[1]); !slices.Equal(sequence, []int32{30028}) {
		t.Fatalf("retry visibility sequence = %v, want [30028]", sequence)
	}
	var self bool
	var peers int
	for _, message := range recorders[1].drain() {
		switch message.(type) {
		case *pb.SCCurrentPosition:
			self = true
		case *pb.SCPlayerAppear:
			if p10SceneHasPlayer([]proto.Message{message}, f.ids[0]) {
				if !self {
					t.Fatal("retry peer preceded self")
				}
				peers++
			}
		}
	}
	if !self || peers != 1 {
		t.Fatalf("retry did not return self and exactly one peer: self=%v peers=%d", self, peers)
	}
}

// Simulate an independent service synchronization after the profile handshake
// commits and before the TCP dispatcher writes its CurrentPosition response.
func TestP10DesertPendingConcurrentSyncBeforePositionReply(t *testing.T) {
	f, recorders := returnLine86Setup(t)
	returnLine86Prepare(t, f, 1, 2, "00000000-0000-4000-8000-000000000601")
	returnLine86Dispatch(t, f, 1, 30061, &pb.CSTransferLine{Line: proto.Int32(2)}, recorders[1])
	returnLine86Dispatch(t, f, 1, 30024, &pb.CSCurrentPosition{Line: proto.Int32(2)}, recorders[1])
	for _, recorder := range recorders {
		recorder.drain()
	}
	returnLine86Prepare(t, f, 1, 1, "00000000-0000-4000-8000-000000000602")
	returnLine86Dispatch(t, f, 1, 30061, &pb.CSTransferLine{Line: proto.Int32(1)}, recorders[1])
	recorders[1].drain()
	nativeCtx := context.WithValue(f.ctx, ggoNativeSessionKey{}, f.sessions[1])
	if _, err := f.s.fieldScene(nativeCtx, f.ids[1], false); err != nil {
		t.Fatal(err)
	}
	if err := f.s.SyncP10Scene(f.ctx, f.ids[1], false); err != nil {
		t.Fatal(err)
	}
	returnLine86Dispatch(t, f, 1, 30024, &pb.CSCurrentPosition{Line: proto.Int32(1)}, recorders[1])
	if sequence := returnLine86VisibilityOpcodes(t, recorders[0].drain(), f.ids[1]); !slices.Equal(sequence, []int32{30028}) {
		t.Fatalf("interleaved watcher sequence = %v, want [30028]", sequence)
	}
	var self bool
	var peers int
	for _, message := range recorders[1].drain() {
		switch message.(type) {
		case *pb.SCCurrentPosition:
			self = true
		case *pb.SCPlayerAppear:
			if p10SceneHasPlayer([]proto.Message{message}, f.ids[0]) {
				if !self {
					t.Error("interleaved sync sent peer before self")
				}
				peers++
			}
		}
	}
	if !self || peers != 1 {
		t.Fatalf("interleaved sync did not preserve one ordered peer: self=%v peers=%d", self, peers)
	}
}

func TestP10DesertLineRequestMatchesCurrentPresence(t *testing.T) {
	f, recorders := returnLine86Setup(t)
	returnLine86Prepare(t, f, 1, 2, "00000000-0000-4000-8000-000000000701")
	returnLine86Dispatch(t, f, 1, 30061, &pb.CSTransferLine{Line: proto.Int32(2)}, recorders[1])
	returnLine86Dispatch(t, f, 1, 30024, &pb.CSCurrentPosition{Line: proto.Int32(2)}, recorders[1])
	for _, recorder := range recorders {
		recorder.drain()
	}
	returnLine86Prepare(t, f, 1, 1, "00000000-0000-4000-8000-000000000702")
	returnLine86Dispatch(t, f, 1, 30061, &pb.CSTransferLine{Line: proto.Int32(1)}, recorders[1])
	recorders[1].drain()
	// A delayed old-line request still confirms self, but cannot retrieve
	// peers through the added line fallback for a different persisted line.
	returnLine86Dispatch(t, f, 1, 30024, &pb.CSCurrentPosition{Line: proto.Int32(2)}, recorders[1])
	if messages := recorders[1].drain(); p10SceneHasPlayer(messages, f.ids[0]) {
		t.Fatalf("old-line request retrieved target-line peers: %v", messages)
	}
	if sequence := returnLine86VisibilityOpcodes(t, recorders[0].drain(), f.ids[1]); !slices.Equal(sequence, []int32{30028}) {
		t.Fatalf("position confirmation visibility sequence = %v", sequence)
	}
	for _, line := range []int32{1, 1, 0, 2, 99} {
		returnLine86Dispatch(t, f, 1, 30024, &pb.CSCurrentPosition{Line: proto.Int32(line)}, recorders[1])
		messages := recorders[1].drain()
		var currentCount, peerCount int
		for _, message := range messages {
			switch message.(type) {
			case *pb.SCCurrentPosition:
				currentCount++
			case *pb.SCPlayerAppear:
				if p10SceneHasPlayer([]proto.Message{message}, f.ids[0]) {
					if currentCount != 1 {
						t.Fatal("line snapshot arrived before self")
					}
					peerCount++
				}
			}
		}
		wantPeers := 0
		if line == 1 {
			wantPeers = 1
		}
		if currentCount != 1 || peerCount != wantPeers {
			t.Fatalf("line %d snapshot: self=%d peers=%d", line, currentCount, peerCount)
		}
		if messages := recorders[0].drain(); p10SceneHasPlayer(messages, f.ids[1]) || p10SceneHasDisappear(messages, f.ids[1]) {
			t.Fatalf("repeated position request changed watcher visibility: %v", messages)
		}
	}
	// The pre-existing showArround contract ignores the requested line and
	// lists only the authenticated actor's actual current line.
	returnLine86Dispatch(t, f, 1, 30024, &pb.CSCurrentPosition{Line: proto.Int32(99), ShowArround: proto.Int32(1)}, recorders[1])
	if messages := recorders[1].drain(); !p10SceneHasPlayer(messages, f.ids[0]) {
		t.Fatal("showArround no longer returns actual-line peers")
	}
}

func TestP10NonDesertLinePositionKeepsShowAroundContract(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	f, other, _ := p10SceneSetup(t)
	for _, id := range []int64{f.id, other} {
		if err := updateProfile(f.ctx, id, func(player *pb.Player, _ *model.GameState) error {
			player.Level = proto.Int32(25)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.fieldTransfer(f.ctx, id, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.fieldScene(f.ctx, id, false); err != nil {
			t.Fatal(err)
		}
	}
	selfRecorder, peerRecorder := &p10SceneRecorder{}, &p10SceneRecorder{}
	p10SceneConnect(t, f, f.id, selfRecorder)
	p10SceneConnect(t, f, other, peerRecorder)
	selfRecorder.drain()
	peerRecorder.drain()
	request := scene49Packet(t, f, f.id, 30024, &pb.CSCurrentPosition{Line: proto.Int32(1)})
	result := f.s.Dispatch(f.ctx, request, "127.0.0.1")
	if len(result) == 0 {
		t.Fatal("non-desert self response missing")
	}
	if _, ok := result[0].(*pb.SCCurrentPosition); !ok || p10SceneHasPlayer(result, other) || p10SceneHasPlayer(selfRecorder.drain(), other) {
		t.Fatal("non-desert showArround=0 unexpectedly returned peers")
	}
	if messages := peerRecorder.drain(); p10SceneHasPlayer(messages, f.id) || p10SceneHasDisappear(messages, f.id) {
		t.Fatal("non-desert current position changed other actor visibility")
	}
}

func TestP10DesertLineReceiptReplayAfterConfirmedReturn(t *testing.T) {
	f, recorders := returnLine86Setup(t)
	returnLine86Prepare(t, f, 1, 2, "00000000-0000-4000-8000-000000000401")
	request := &pb.CSTransferLine{Line: proto.Int32(2)}
	first, handled, err := f.s.DispatchP10Scene(f.ctx, f.ids[1], request, 401)
	if err != nil || !handled || len(first) != 1 {
		t.Fatalf("initial receipt: handled=%v err=%v", handled, err)
	}
	returnLine86Dispatch(t, f, 1, 30024, &pb.CSCurrentPosition{}, recorders[1])
	for _, recorder := range recorders {
		recorder.drain()
	}
	replay, handled, err := f.s.DispatchP10Scene(f.ctx, f.ids[1], request, 401)
	if err != nil || !handled || len(replay) != 1 || !proto.Equal(first[0], replay[0]) {
		t.Fatalf("current receipt replay changed: handled=%v err=%v", handled, err)
	}
	for _, recorder := range recorders {
		if messages := recorder.drain(); len(messages) != 0 {
			t.Fatalf("receipt replay changed visible presence: %v", messages)
		}
	}
	returnLine86Prepare(t, f, 1, 1, "00000000-0000-4000-8000-000000000402")
	returnLine86Dispatch(t, f, 1, 30061, &pb.CSTransferLine{Line: proto.Int32(1)}, recorders[1])
	returnLine86Dispatch(t, f, 1, 30024, &pb.CSCurrentPosition{}, recorders[1])
	for _, recorder := range recorders {
		recorder.drain()
	}
	if _, handled, err = f.s.DispatchP10Scene(f.ctx, f.ids[1], request, 401); !handled || err == nil {
		t.Fatal("old receipt unexpectedly moved the actor back to an obsolete line")
	}
	for _, recorder := range recorders {
		if messages := recorder.drain(); len(messages) != 0 {
			t.Fatalf("obsolete receipt changed visible presence: %v", messages)
		}
	}
}

func TestP10DesertPendingFrameGORMBinding(t *testing.T) {
	returnLine86Setup(t)
	result := db.GetIns().Session(&gorm.Session{DryRun: true, SkipDefaultTransaction: true}).Save(&db.P10ScenePresence{
		PlayerID: 999, Scene: constUtil.GGOFieldScene, Line: 1, SessionHash: "session", Frame: []byte{},
	})
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	for _, value := range result.Statement.Vars {
		if frame, ok := value.([]byte); ok && frame != nil && len(frame) == 0 {
			return
		}
	}
	t.Fatalf("GORM did not bind the pending frame as non-nil empty bytes: %#v", result.Statement.Vars)
}
