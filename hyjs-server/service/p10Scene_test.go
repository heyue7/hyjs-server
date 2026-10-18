package service

import (
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p10SceneRecorder struct {
	mutex    sync.Mutex
	messages []proto.Message
}

func (r *p10SceneRecorder) send(m proto.Message) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.messages = append(r.messages, proto.Clone(m))
	return nil
}
func (r *p10SceneRecorder) drain() []proto.Message {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	m := r.messages
	r.messages = nil
	return m
}
func p10SceneSetup(t *testing.T) (*p1Fixture, int64, int64) {
	t.Helper()
	f, b, c := p10Setup(t)
	if err := f.s.InitializeP10Scene(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P10ScenePolicy{}, &db.P10ScenePresence{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.InitializeP10Scene(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P10ScenePolicy{}).Where("id=?", 1).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	return f, b, c
}
func p10SceneConnect(t *testing.T, f *p1Fixture, id int64, rec *p10SceneRecorder) func() {
	t.Helper()
	var row db.Profile
	if err := db.GetIns().First(&row, id).Error; err != nil {
		t.Fatal(err)
	}
	close := f.s.RegisterP10Client(f.ctx, id, row.Session, rec.send)
	t.Cleanup(close)
	if err := f.s.SyncP10Scene(f.ctx, id, false); err != nil {
		t.Fatal(err)
	}
	return close
}
func p10SceneHasPlayer(messages []proto.Message, id int64) bool {
	for _, m := range messages {
		if p, ok := m.(*pb.SCPlayerAppear); ok {
			for _, player := range p.GetPlayer() {
				if player.GetInfo().GetPlayerId() == id {
					return true
				}
			}
		}
	}
	return false
}
func p10SceneHasDisappear(messages []proto.Message, id int64) bool {
	for _, m := range messages {
		if p, ok := m.(*pb.SCPlayerDisappear); ok {
			for _, pid := range p.GetPlayerId() {
				if pid == id {
					return true
				}
			}
		}
	}
	return false
}
func TestP10SceneRealAppearanceMoveAndTwoLineIsolation(t *testing.T) {
	f, b, c := p10SceneSetup(t)
	a := f.id
	ra, rb, rc := &p10SceneRecorder{}, &p10SceneRecorder{}, &p10SceneRecorder{}
	p10SceneConnect(t, f, a, ra)
	p10SceneConnect(t, f, b, rb)
	if !p10SceneHasPlayer(ra.drain(), b) || !p10SceneHasPlayer(rb.drain(), a) {
		t.Fatal("real mutual appearance")
	}
	parked := db.P10ScenePresence{PlayerID: c, Scene: "town_main", Line: 2, Frame: []byte{}}
	if err := db.GetIns().Create(&parked).Error; err != nil {
		t.Fatal(err)
	}
	p10SceneConnect(t, f, c, rc)
	ra.drain()
	rb.drain()
	rc.drain()
	if err := updateProfile(f.ctx, a, func(p *pb.Player, g *model.GameState) error {
		g.Position.Current = &pb.PointNew{X: proto.Float32(8), Y: proto.Float32(0), Z: proto.Float32(9)}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.SyncP10Scene(f.ctx, a, true); err != nil {
		t.Fatal(err)
	}
	messages := rb.drain()
	if len(messages) != 1 {
		t.Fatal("same-line movement missing", len(messages))
	}
	movement, ok := messages[0].(*pb.SCMovePush)
	if !ok || movement.GetPosition()[0].GetPlayerId() != a || movement.GetPosition()[0].GetCurrent().GetX() != 8 {
		t.Fatal("actual position broadcast")
	}
	if len(rc.drain()) != 0 {
		t.Fatal("movement leaked cross-line")
	}
	request := &pb.CSTransferLine{Line: proto.Int32(2)}
	r, handled, err := f.s.DispatchP10Scene(f.ctx, a, request, 1)
	if !handled || err != nil {
		t.Fatal(err)
	}
	if !p10SceneHasDisappear(rb.drain(), a) || !p10SceneHasPlayer(rc.drain(), a) {
		t.Fatal("line transition peers missing")
	}
	ra.drain()
	second, _, err := f.s.DispatchP10Scene(f.ctx, a, request, 1)
	if err != nil || !proto.Equal(r[0], second[0]) || len(rb.drain())+len(rc.drain())+len(ra.drain()) != 0 {
		t.Fatal("line retry repeated visibility")
	}
	info, _, err := f.s.DispatchP10Scene(f.ctx, a, &pb.CSMovePlayerInfo{PlayerIds: []int64{a, b, c, c}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	list := info[0].(*pb.SCMovePlayerInfo).GetPlayerInfos()
	if len(list) != 2 {
		t.Fatal("cross-line/duplicate player exposed", list)
	}
	for _, peer := range list {
		if peer.GetInfo().GetPlayerId() == b {
			t.Fatal("foreign line info")
		}
	}
}
func TestP10SceneClosedCapacityAndInstanceExclusion(t *testing.T) {
	f, b, c := p10SceneSetup(t)
	if err := db.GetIns().Model(&db.P10ScenePolicy{}).Where("id=?", 1).Update("max_players", 1).Error; err != nil {
		t.Fatal(err)
	}
	a := f.id
	ra, rb, rc := &p10SceneRecorder{}, &p10SceneRecorder{}, &p10SceneRecorder{}
	p10SceneConnect(t, f, a, ra)
	p10SceneConnect(t, f, b, rb)
	var own db.P10ScenePresence
	if err := db.GetIns().Where("player_id=?", b).First(&own).Error; err != nil {
		t.Fatal(err)
	}
	if own.Line != 2 {
		t.Fatal("auto full-line selection")
	}
	var profile db.Profile
	if err := db.GetIns().First(&profile, c).Error; err != nil {
		t.Fatal(err)
	}
	close := f.s.RegisterP10Client(f.ctx, c, profile.Session, rc.send)
	t.Cleanup(close)
	if err := f.s.SyncP10Scene(f.ctx, c, false); err == nil {
		t.Fatal("finite line capacity exceeded")
	}
	if _, _, err := f.s.DispatchP10Scene(f.ctx, a, &pb.CSTransferLine{Line: proto.Int32(2)}, 1); err == nil {
		t.Fatal("entered full line")
	}
	if err := updateProfile(f.ctx, a, func(p *pb.Player, g *model.GameState) error { g.Run = &model.CopyRun{Complete: false}; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := f.s.SyncP10Scene(f.ctx, a, false); err != nil {
		t.Fatal(err)
	}
	var row db.P10ScenePresence
	if err := db.GetIns().Where("player_id=?", a).First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.SessionHash != "" {
		t.Fatal("private copy leaked public presence")
	}
	if err := db.GetIns().Model(&db.P10ScenePolicy{}).Where("id=?", 1).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if _, handled, err := f.s.DispatchP10Scene(f.ctx, b, &pb.CSGetLineInfo{}, 0); !handled || err == nil {
		t.Fatal("disabled lines open")
	}
}
func TestP10SceneSessionRotationDisconnectAndPersistentLine(t *testing.T) {
	f, b, _ := p10SceneSetup(t)
	a := f.id
	ra, rb := &p10SceneRecorder{}, &p10SceneRecorder{}
	closeOld := p10SceneConnect(t, f, a, ra)
	p10SceneConnect(t, f, b, rb)
	ra.drain()
	rb.drain()
	var profile db.Profile
	if err := db.GetIns().First(&profile, a).Error; err != nil {
		t.Fatal(err)
	}
	fresh, err := LoginAccount(f.ctx, profile.Uid, "local-test-password")
	if err != nil {
		t.Fatal(err)
	}
	newRec := &p10SceneRecorder{}
	closeNew := f.s.RegisterP10Client(f.ctx, a, fresh.Session, newRec.send)
	t.Cleanup(closeNew)
	if err = f.s.SyncP10Scene(f.ctx, a, false); err != nil {
		t.Fatal(err)
	}
	rb.drain()
	newRec.drain()
	closeOld()
	if err = f.s.SyncP10Scene(f.ctx, b, true); err != nil {
		t.Fatal(err)
	}
	if len(newRec.drain()) != 1 || len(ra.drain()) != 0 {
		t.Fatal("old unregister removed new scene session")
	}
	if _, _, err = f.s.DispatchP10Scene(f.ctx, a, &pb.CSTransferLine{Line: proto.Int32(2)}, 1); err != nil {
		t.Fatal(err)
	}
	rb.drain()
	newRec.drain()
	closeNew()
	f.s.DisconnectP10Scene(f.ctx, a, fresh.Session)
	f.restart(t)
	if err = f.s.InitializeP10Scene(f.ctx); err != nil {
		t.Fatal(err)
	}
	reconnected := &p10SceneRecorder{}
	p10SceneConnect(t, f, a, reconnected)
	reply, _, err := f.s.DispatchP10Scene(f.ctx, a, &pb.CSGetLineInfo{}, 0)
	if err != nil || reply[0].(*pb.SCGetLineInfo).GetLine() != 2 {
		t.Fatal("reconnect lost persisted line", err)
	}
}
func TestP10SceneCurrentProfileSceneOverridesStalePresenceFrame(t *testing.T) {
	f, b, _ := p10SceneSetup(t)
	ra, rb := &p10SceneRecorder{}, &p10SceneRecorder{}
	p10SceneConnect(t, f, f.id, ra)
	p10SceneConnect(t, f, b, rb)
	ra.drain()
	rb.drain()
	if _, err := f.s.fieldTransfer(f.ctx, b, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
		t.Fatal(err)
	}
	peers, err := f.s.p10ScenePeers(db.GetIns(), "town_main", 1, f.id)
	if err != nil || len(peers) != 0 {
		t.Fatal("stale scene frame advertised moved peer", err)
	}
	if err = f.s.SyncP10Scene(f.ctx, b, false); err != nil {
		t.Fatal(err)
	}
	if !p10SceneHasDisappear(ra.drain(), b) {
		t.Fatal("scene transition missing disappear")
	}
	p10SceneCall := &pb.CSGetLineInfo{}
	reply, _, err := f.s.DispatchP10Scene(f.ctx, b, p10SceneCall, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(reply[0].(*pb.SCGetLineInfo).GetLineInfos()) != 2 {
		t.Fatal("original line list")
	}
}
