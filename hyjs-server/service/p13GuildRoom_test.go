package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

func p13RoomTransfer(t *testing.T, f *p1Fixture, id int64) {
	t.Helper()
	if _, err := f.s.fieldTransfer(f.ctx, id, &pb.CSTransferScene{Target: proto.String(constUtil.P13GuildRoomTarget)}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.SyncP10Scene(f.ctx, id, false); err != nil {
		t.Fatal(err)
	}
}
func p13RoomPosition(t *testing.T, f *p1Fixture, id int64) string {
	t.Helper()
	var row db.Profile
	if err := db.GetIns().First(&row, id).Error; err != nil {
		t.Fatal(err)
	}
	_, g, err := decodeProfile(&row)
	if err != nil {
		t.Fatal(err)
	}
	return g.Position.GetScene()
}
func p13RoomHasRefresh(messages []proto.Message) bool {
	for _, m := range messages {
		if _, ok := m.(*pb.SCRefreshScene); ok {
			return true
		}
	}
	return false
}

// TestP13GuildRoom 隔离数据库内使用真实会员/会话/位置，验证原房间帧和即时撤权。
func TestP13GuildRoomOriginalSceneIsolationLoginBuildingAndRevocation(t *testing.T) {
	f, ids := p13GuildSetup(t)
	now := time.Now()
	p13GuildOpen(t, false)
	if err := f.s.InitializeP10Scene(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P10ScenePolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("领地甲")}, 1, now)
	ga := p13GuildID(t, f, ids[0], now)
	p13Join(t, f, ids[0], ids[1], ga, 2, now)
	p13Join(t, f, ids[0], ids[2], ga, 3, now)
	p13GuildCall(t, f, ids[3], &pb.CSUnionCreate{Name: proto.String("领地乙")}, 1, now)
	gb := p13GuildID(t, f, ids[3], now)
	recorders := make([]*p10SceneRecorder, len(ids))
	for i, id := range ids {
		recorders[i] = &p10SceneRecorder{}
		var row db.Profile
		if err := db.GetIns().First(&row, id).Error; err != nil {
			t.Fatal(err)
		}
		detach := f.s.RegisterP10Client(f.ctx, id, row.Session, recorders[i].send)
		t.Cleanup(detach)
	}
	if _, err := f.s.fieldTransfer(f.ctx, ids[4], &pb.CSTransferScene{Target: proto.String(constUtil.P13GuildRoomTarget)}); err == nil {
		t.Fatal("outsider entered guild room")
	}
	if err := updateProfile(f.ctx, ids[1], func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(29); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.fieldTransfer(f.ctx, ids[1], &pb.CSTransferScene{Target: proto.String(constUtil.P13GuildRoomTarget)}); err == nil {
		t.Fatal("level29 entered original30 room")
	}
	if err := updateProfile(f.ctx, ids[1], func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(30); return nil }); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids[:4] {
		p13RoomTransfer(t, f, id)
	}
	if !p10SceneHasPlayer(recorders[0].drain(), ids[1]) || !p10SceneHasPlayer(recorders[1].drain(), ids[0]) {
		t.Fatal("same guild appearances missing")
	}
	if len(recorders[3].drain()) != 0 || len(recorders[4].drain()) != 0 {
		t.Fatal("foreign/outsider room appearance leak")
	}
	for _, id := range ids[:4] {
		var row db.P10ScenePresence
		if err := db.GetIns().First(&row, "player_id=?", id).Error; err != nil {
			t.Fatal(err)
		}
		expected := ga
		if id == ids[3] {
			expected = gb
		}
		if row.Scene != constUtil.P13GuildRoomScopePrefix+expected || row.Line != 1 {
			t.Fatal("room instance key", row)
		}
		frame := &pb.MovePlayer{}
		if err := proto.Unmarshal(row.Frame, frame); err != nil {
			t.Fatal(err)
		}
		if frame.Position.GetScene() != constUtil.P13GuildRoomTarget {
			t.Fatal("private scope leaked to original wire scene")
		}
	}
	for _, req := range []proto.Message{&pb.CSMovePlayerInfo{PlayerIds: ids}, &pb.CSRefreshMovePlayerInfo{}} {
		messages, handled, err := f.s.DispatchP10Scene(f.ctx, ids[0], req, 10)
		if !handled || err != nil {
			t.Fatal(err)
		}
		peers := messages[0].(*pb.SCMovePlayerInfo).GetPlayerInfos()
		if len(peers) != 3 {
			t.Fatal("same guild visible members", len(peers))
		}
		for _, peer := range peers {
			if peer.GetInfo().GetPlayerId() == ids[3] || peer.GetInfo().GetPlayerId() == ids[4] {
				t.Fatal("foreign room info leak")
			}
		}
	}
	lines, _, err := f.s.DispatchP10Scene(f.ctx, ids[0], &pb.CSGetLineInfo{}, 11)
	if err != nil || len(lines[0].(*pb.SCGetLineInfo).LineInfos) != 1 {
		t.Fatal("guild one line", err)
	}
	if _, _, err = f.s.DispatchP10Scene(f.ctx, ids[0], &pb.CSTransferLine{Line: proto.Int32(2)}, 12); err == nil {
		t.Fatal("invented guild line2")
	}
	var before db.Profile
	if err = db.GetIns().First(&before, ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	for _, login := range []bool{false, true} {
		messages, err := f.s.fieldScene(f.ctx, ids[0], login)
		if err != nil {
			t.Fatal(err)
		}
		building, side := false, false
		for _, m := range messages {
			switch b := m.(type) {
			case *pb.SCUnionBuilding:
				building = b.HeroStatueOpen != nil && b.GuardTreeOpen != nil && b.GetHeroStatueOpen() == 0 && b.GetGuardTreeOpen() == 0
			case *pb.SCFieldPlayerState:
				side = len(b.Player) == 1 && b.Player[0].GetPlayerId() == ids[0] && b.Player[0].GetKills() == 0
			case *pb.SCCurrentPosition:
				if b.Self.Position.GetScene() != constUtil.P13GuildRoomTarget || b.GetLimit() != 35 {
					t.Fatal("original current room capacity", b)
				}
			case *pb.SCLoginScene:
				if b.Self.Position.GetScene() != constUtil.P13GuildRoomTarget {
					t.Fatal("login lost union routing")
				}
			}
		}
		if !building || !side {
			t.Fatal("original room building/self state missing")
		}
	}
	building, handled, err := f.s.DispatchP13Guild(f.ctx, ids[0], &pb.CSRefreshUnionBuilding{}, 13)
	if !handled || err != nil || building[0].(*pb.SCUnionBuilding).GetHeroStatueOpen() != 0 {
		t.Fatal("refresh unopened building", err)
	}
	var after db.Profile
	if err = db.GetIns().First(&after, ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	if *before.Player != *after.Player {
		t.Fatal("room query changed player assets")
	}
	for _, r := range recorders {
		r.drain()
	}
	if err = updateProfile(f.ctx, ids[0], func(p *pb.Player, g *model.GameState) error {
		g.Position.Current = &pb.PointNew{X: proto.Float32(7), Y: proto.Float32(0), Z: proto.Float32(9)}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = f.s.SyncP10Scene(f.ctx, ids[0], true); err != nil {
		t.Fatal(err)
	}
	for _, i := range []int{1, 2} {
		m := recorders[i].drain()
		if len(m) != 1 {
			t.Fatal("guild movement", i, len(m))
		}
		if _, ok := m[0].(*pb.SCMovePush); !ok {
			t.Fatal("original movement frame", m)
		}
	}
	if len(recorders[3].drain()) != 0 {
		t.Fatal("foreign movement leak")
	}
	p13GuildCall(t, f, ids[2], &pb.CSQuitUnion{}, 20, now)
	if p13RoomPosition(t, f, ids[2]) != "town_main" || !p13RoomHasRefresh(recorders[2].drain()) || !p10SceneHasDisappear(recorders[0].drain(), ids[2]) {
		t.Fatal("quit persisted route/revoke missing")
	}
	if _, _, err = f.s.DispatchP13Guild(f.ctx, ids[2], &pb.CSRefreshUnionBuilding{}, 21); err == nil {
		t.Fatal("quit member refreshed building")
	}
	p13GuildCall(t, f, ids[0], &pb.CSRemoveUnionPlayer{PlayerId: proto.Int64(ids[1])}, 22, now)
	if p13RoomPosition(t, f, ids[1]) != "town_main" || !p13RoomHasRefresh(recorders[1].drain()) {
		t.Fatal("kick room revocation")
	}
	p13GuildCall(t, f, ids[0], &pb.CSDissolveUnion{}, 23, now)
	if p13RoomPosition(t, f, ids[0]) != "town_main" || !p13RoomHasRefresh(recorders[0].drain()) {
		t.Fatal("dissolve room revocation")
	}
	if p13RoomPosition(t, f, ids[3]) != constUtil.P13GuildRoomTarget {
		t.Fatal("foreign guild dissolution changed other instance")
	}
	// 持久位置保留原房间名，进程重开后按真实会员重新确定专属实例。
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	var row db.Profile
	if err = db.GetIns().First(&row, ids[3]).Error; err != nil {
		t.Fatal(err)
	}
	detach := f.s.RegisterP10Client(f.ctx, ids[3], row.Session, recorders[3].send)
	t.Cleanup(detach)
	if err = f.s.SyncP10Scene(f.ctx, ids[3], false); err != nil {
		t.Fatal(err)
	}
	scope, err := f.s.P13GuildRoomPresenceScope(db.GetIns(), ids[3], mustP13RoomGame(t, &row))
	if err != nil || scope != constUtil.P13GuildRoomScopePrefix+gb {
		t.Fatal("restart own instance", scope, err)
	}
	if err = updateProfile(f.ctx, ids[2], func(p *pb.Player, g *model.GameState) error {
		g.Position.Scene = proto.String(constUtil.P13GuildRoomTarget)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	messages, err := f.s.fieldScene(f.ctx, ids[2], true)
	if err != nil || messages[0].(*pb.SCLoginScene).Self.Position.GetScene() != "town_main" {
		t.Fatal("stale revoked room login", err)
	}
}
func mustP13RoomGame(t *testing.T, row *db.Profile) *model.GameState {
	t.Helper()
	_, g, err := decodeProfile(row)
	if err != nil {
		t.Fatal(err)
	}
	return g
}
