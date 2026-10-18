package service

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP10SecondFieldCurrentSideVisibilityMovementAndLineIsolation(t *testing.T) {
	f, other, town := p10SceneSetup(t)
	for _, id := range []int64{f.id, other} {
		// 隔离账号仅准备原第二图25级准入；不准备货币、道具或战斗结果。
		if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
			p.Level = proto.Int32(25)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.fieldTransfer(f.ctx, id, &pb.CSTransferScene{Target: proto.String("field_002")}); err != nil {
			t.Fatal(err)
		}
		var row db.Profile
		if err := db.GetIns().First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		_, loading, err := decodeProfile(&row)
		if err != nil || p10SceneScope(loading) != "" {
			t.Fatal("second field exposed a player before original scene handshake", err)
		}
		if _, err = f.s.fieldScene(f.ctx, id, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := updateProfile(f.ctx, other, func(p *pb.Player, g *model.GameState) error {
		g.Field.Side.CurHp = proto.Float64(0.625)
		g.Field.Side.Anger = proto.Float64(12.5)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var (
		selfRecorder  = &p10SceneRecorder{}
		otherRecorder = &p10SceneRecorder{}
		townRecorder  = &p10SceneRecorder{}
	)
	p10SceneConnect(t, f, f.id, selfRecorder)
	p10SceneConnect(t, f, other, otherRecorder)
	p10SceneConnect(t, f, town, townRecorder)
	if !p10SceneHasPlayer(selfRecorder.drain(), other) || !p10SceneHasPlayer(otherRecorder.drain(), f.id) || p10SceneHasPlayer(townRecorder.drain(), f.id) {
		t.Fatal("second-field appearance did not preserve scene separation")
	}
	info, _, err := f.s.DispatchP10Scene(f.ctx, f.id, &pb.CSMovePlayerInfo{PlayerIds: []int64{other}}, 0)
	if err != nil || len(info) == 0 {
		t.Fatal("second-field original player-info query", err)
	}
	peers := info[0].(*pb.SCMovePlayerInfo).GetPlayerInfos()
	if len(peers) != 1 || peers[0].GetInfo().GetPlayerSide().GetCurHp() != 0.625 || peers[0].GetInfo().GetPlayerSide().GetAnger() != 12.5 {
		t.Fatal("second-field move frame omitted persisted side HP/AP")
	}
	var before db.Profile
	if err = db.GetIns().First(&before, other).Error; err != nil {
		t.Fatal(err)
	}
	hp, err := f.s.p8VisibleHp(f.ctx, f.id, other)
	if err != nil || hp[0].(*pb.SCGetHp).GetHp() != 0.625 {
		t.Fatal("second-field visible HP did not read current profile", err)
	}
	var after db.Profile
	if err = db.GetIns().First(&after, other).Error; err != nil || *before.Player != *after.Player || *before.Game != *after.Game {
		t.Fatal("read-only second-field HP query changed the target", err)
	}
	selfRecorder.drain()
	otherRecorder.drain()
	if err = updateProfile(f.ctx, other, func(p *pb.Player, g *model.GameState) error {
		g.Position.Current = &pb.PointNew{X: proto.Float32(67), Y: proto.Float32(15), Z: proto.Float32(84)}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = f.s.SyncP10Scene(f.ctx, other, true); err != nil {
		t.Fatal(err)
	}
	foundMovement := false
	for _, out := range selfRecorder.drain() {
		if move, ok := out.(*pb.SCMovePush); ok && len(move.Position) == 1 && move.Position[0].GetPlayerId() == other && move.Position[0].GetCurrent().GetX() == 67 {
			foundMovement = true
		}
	}
	if !foundMovement || p10SceneHasPlayer(townRecorder.drain(), other) {
		t.Fatal("second-field current movement was missing or leaked to town")
	}
	if _, _, err = f.s.DispatchP10Scene(f.ctx, other, &pb.CSTransferLine{Line: proto.Int32(2)}, 0); err != nil {
		t.Fatal(err)
	}
	if !p10SceneHasDisappear(selfRecorder.drain(), other) {
		t.Fatal("second-field original zero-stamp line change omitted disappearance")
	}
	if _, err = f.s.p8VisibleHp(f.ctx, f.id, other); err == nil {
		t.Fatal("second-field HP leaked across lines")
	}
	if _, err = f.s.fieldTransfer(f.ctx, other, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.fieldScene(f.ctx, other, false); err != nil {
		t.Fatal(err)
	}
	if err = f.s.SyncP10Scene(f.ctx, other, false); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p8VisibleHp(f.ctx, f.id, other); err == nil {
		t.Fatal("second-field HP query crossed into first field")
	}
}
