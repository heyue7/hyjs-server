package service

import (
	"bytes"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Exercise the real native dispatcher and profile transaction, including the
// old P16 admission guard and the request-local published runtime snapshot.
func TestGGONativeTransferAdmissionAndPosition(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	f := ggoTradeSetup(t)
	if err := f.s.InitializeP10Scene(f.ctx); err != nil {
		t.Fatal(err)
	}
	id := f.ids[0]
	if err := updateProfile(f.ctx, id, func(player *pb.Player, game *model.GameState) error {
		player.Level = proto.Int32(60)
		game.Position = &pb.Position{PlayerId: proto.Int64(id), Scene: proto.String("town"), Current: &pb.PointNew{X: proto.Float32(0), Y: proto.Float32(0), Z: proto.Float32(0)}}
		game.PositionLoading, game.GGOField, game.Field = false, nil, nil
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	f.config.Runtime.MinClientBuild = 2026102904
	session := f.sessions[0]
	packet := func(target string, token []byte) *model.GamePacket {
		return &model.GamePacket{Opcode: 30032, PlayerId: id, ZoneId: int32(f.s.OriginalServerID()), Session: token, Body: &pb.CSTransferScene{Target: proto.String(target)}}
	}
	rejected := func(target string, token []byte, want string) {
		t.Helper()
		out := f.s.Dispatch(f.ctx, packet(target, token), "127.0.0.1")
		if len(out) != 1 {
			t.Fatalf("%s: unexpected response count %d", target, len(out))
		}
		failure, ok := out[0].(*pb.ErrorMessage)
		if !ok || failure.GetMsg() != want {
			t.Fatalf("%s: unexpected rejection %v", target, out[0])
		}
		var row db.Profile
		if err := db.GetIns().First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		_, game, err := decodeProfile(&row)
		if err != nil {
			t.Fatal(err)
		}
		if game.Position.GetScene() != "town" || game.GGOField != nil {
			t.Fatal("rejected admission changed profile")
		}
	}
	if !constUtil.GGOExtractionReady {
		rejected("field_003", session, constUtil.P16UnavailableFieldTip)
	}
	f.s.GGOIntegrationReady = true
	for _, scene := range []string{"field_004", "field_005", "field_006"} {
		rejected(scene, session, constUtil.P16UnavailableFieldTip)
	}
	snapshot := f.s.adminFieldRuntimeCache.Load()
	if snapshot == nil || snapshot.ggo == nil {
		t.Fatal("missing fixture runtime snapshot")
	}
	withoutPublishedRuntime := *snapshot
	withoutPublishedRuntime.ggo = nil
	f.s.adminFieldRuntimeCache.Store(&withoutPublishedRuntime)
	rejected("field_003", session, constUtil.P8FieldSceneUnavailable)
	f.s.adminFieldRuntimeCache.Store(snapshot)

	setBinding := func(build int64, hash string) {
		t.Helper()
		if err := db.GetIns().Model(&db.ClientUpdateSession{}).Where("player_id=?", id).Updates(map[string]any{"build_code": build, "session_hash": hash}).Error; err != nil {
			t.Fatal(err)
		}
	}
	setBinding(2026103101, clientUpdateSessionHash(session))
	rejected("field_003", session, constUtil.ClientUpdateDownloadText)
	other := bytes.Repeat([]byte{42}, 16)
	if bytes.Equal(other, session) {
		other[0]++
	}
	setBinding(2026103102, clientUpdateSessionHash(other))
	rejected("field_003", session, constUtil.ClientUpdateDownloadText)
	setBinding(2026103102, clientUpdateSessionHash(session))
	rejected("field_003", other, "本地会话无效，请重新登录")

	out := f.s.Dispatch(f.ctx, packet("field_003", session), "127.0.0.1")
	if len(out) != 1 {
		t.Fatalf("native transfer returned %d messages", len(out))
	}
	transfer, ok := out[0].(*pb.SCTransferScene)
	if !ok || transfer.GetResult() != 1 {
		t.Fatalf("native transfer rejected: %v", out[0])
	}
	var row db.Profile
	if err := db.GetIns().First(&row, id).Error; err != nil {
		t.Fatal(err)
	}
	_, game, err := decodeProfile(&row)
	if err != nil {
		t.Fatal(err)
	}
	if game.Position.GetScene() != constUtil.GGOFieldScene || !game.PositionLoading || game.GGOField == nil || game.GGOField.PositionRevision < 1 || !ggoPointInsideSafe(game.Position.GetCurrent(), f.config.Runtime.SafeArea) {
		t.Fatalf("incorrect admitted state: %v", game.Position)
	}
	out = f.s.Dispatch(f.ctx, &model.GamePacket{Opcode: 30024, PlayerId: id, ZoneId: int32(f.s.OriginalServerID()), Session: session, Body: &pb.CSCurrentPosition{}}, "127.0.0.1")
	found := false
	for _, message := range out {
		if _, ok := message.(*pb.SCCurrentPosition); ok {
			found = true
		}
		if failure, ok := message.(*pb.ErrorMessage); ok {
			t.Fatalf("position handshake failed: %s", failure.GetMsg())
		}
	}
	if !found {
		t.Fatal("native position response missing")
	}
	if err := db.GetIns().First(&row, id).Error; err != nil {
		t.Fatal(err)
	}
	_, game, err = decodeProfile(&row)
	if err != nil {
		t.Fatal(err)
	}
	if game.PositionLoading || game.GGOField.PositionConfirmed != game.GGOField.PositionRevision || game.GGOField.PositionSession != clientUpdateSessionHash(session) {
		t.Fatal("position handshake not persisted")
	}
}
