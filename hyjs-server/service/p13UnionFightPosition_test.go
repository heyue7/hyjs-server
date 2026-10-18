package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

// TestP13UnionFightPositionLoading 复现HTTP换场后旧TownTCP排队移动，原Current握手成功前静默忽略且冷启动保留门禁。
func TestP13UnionFightPositionLoadingOldTownMoveLoginConfirmColdCurrentThenValidMove(t *testing.T) {
	f, ids, _, clock, match := p13UFFieldSetup(t)
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "隔离场景加载握手；明确本地出生/HP"}).Error; err != nil {
		t.Fatal(err)
	}
	now := time.UnixMilli(clock.Load())
	if _, err := f.s.p13UFFieldEnter(f.ctx, ids[0], now); err != nil {
		t.Fatal(err)
	}
	var before db.P13UnionFightFieldPlayer
	if err := db.GetIns().First(&before, "match_id=? AND player_id=?", match.ID, ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	read := func() bool {
		var row db.Profile
		if err := db.GetIns().First(&row, ids[0]).Error; err != nil {
			t.Fatal(err)
		}
		_, game, err := decodeProfile(&row)
		if err != nil {
			t.Fatal(err)
		}
		if game.Position.Current.GetX() != before.X || game.Position.Current.GetY() != before.Y || game.Position.Current.GetZ() != before.Z {
			t.Fatal("queued old point overwrote persisted spawn", game.Position)
		}
		return game.PositionLoading
	}
	if !read() {
		t.Fatal("actual enter did not arm scene loading gate")
	}
	old := &pb.CSMove{PointNew: &pb.PointNew{X: proto.Float32(-9.357699), Y: proto.Float32(10.6129), Z: proto.Float32(37.3301)}}
	for _, request := range []proto.Message{&pb.CSLoginScene{}, &pb.CSMoveConfirm{}, &pb.CSUFInit{}} {
		if _, handled, err := f.s.DispatchP13UnionFightField(f.ctx, ids[0], request); err != nil || !handled {
			t.Fatal("original non-Current handshake", err)
		}
		if !read() {
			t.Fatal("Login/MoveConfirm/Init cleared original loading gate")
		}
		response, err := f.s.p13UFFieldMove(f.ctx, ids[0], old, now)
		if err != nil || len(response) != 0 {
			t.Fatal("old Town point must be ignored before map/state/speed rejection", err, response)
		}
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err := f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	if !read() {
		t.Fatal("cold database restart lost pending scene load")
	}
	if response, err := f.s.p13UFFieldMove(f.ctx, ids[0], old, now); err != nil || len(response) != 0 {
		t.Fatal("cold pending old move was processed", err)
	}
	var recovered db.P13UnionFightFieldPlayer
	if err := db.GetIns().First(&recovered, "match_id=? AND player_id=?", match.ID, ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	if before.FrameHash != recovered.FrameHash || before.BattleHash != recovered.BattleHash || before.X != recovered.X || before.Y != recovered.Y || before.Z != recovered.Z || before.UpdatedAt != recovered.UpdatedAt || before.MovementDebt != recovered.MovementDebt {
		t.Fatal("ignored move changed actor state")
	}
	frames, err := f.s.p13UFFieldFrames(f.ctx, ids[0], &pb.CSCurrentPosition{}, now)
	if err != nil || frames[0].(*pb.SCCurrentPosition).Self.Position.Current.GetX() != before.X {
		t.Fatal("original Current reply", err)
	}
	if read() {
		t.Fatal("Current successful transaction did not release loading gate")
	}
	clock.Store(clock.Load() + 1000)
	target := &pb.PointNew{X: proto.Float32(before.X + 1), Y: proto.Float32(before.Y), Z: proto.Float32(before.Z)}
	if response, err := f.s.p13UFFieldMove(f.ctx, ids[0], &pb.CSMove{PointNew: target}, time.UnixMilli(clock.Load())); err != nil || len(response) != 1 {
		t.Fatal("valid new scene move after Current", err)
	}
	if _, err = f.s.p13UFFieldLeave(f.ctx, ids[0], &pb.CSTransferScene{Target: proto.String("town_main")}, time.UnixMilli(clock.Load())); err != nil {
		t.Fatal(err)
	}
	var row db.Profile
	db.GetIns().First(&row, ids[0])
	_, game, err := decodeProfile(&row)
	if err != nil || game.Position.GetScene() != "town_main" || !game.PositionLoading {
		t.Fatal("actual leave did not arm Town load", err)
	}
}

// TestP13UnionFightPositionLoadingSameSceneNextRound 验证上轮在线未踢时，同名场景的新实例也重新等待原Current握手。
func TestP13UnionFightPositionLoadingSameSceneNextRoundOldMoveIgnored(t *testing.T) {
	f, ids, first, clock, match := p13UFFieldSetup(t)
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "隔离真实两轮实例加载握手"}).Error; err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return time.UnixMilli(clock.Load()) }
	if _, err := f.s.p13UFFieldEnter(f.ctx, ids[0], now()); err != nil {
		t.Fatal(err)
	}
	frames, err := f.s.p13UFFieldFrames(f.ctx, ids[0], &pb.CSCurrentPosition{}, now())
	if err != nil {
		t.Fatal(err)
	}
	oldPoint := proto.Clone(frames[0].(*pb.SCCurrentPosition).Self.Position.Current).(*pb.PointNew)
	oldPoint.X = proto.Float32(oldPoint.GetX() + 1)
	clock.Add(1000)
	if _, err = f.s.p13UFFieldMove(f.ctx, ids[0], &pb.CSMove{PointNew: oldPoint}, now()); err != nil {
		t.Fatal(err)
	}
	// 不调用旧轮worker，重现停止服务跨过上轮结束后先进入第二轮的真实顺序。
	clock.Store(first + 300000)
	if _, err = f.s.p13UFFieldEnter(f.ctx, ids[0], now()); err != nil {
		t.Fatal(err)
	}
	var active db.P13UnionFightFieldPlayer
	if err = db.GetIns().First(&active, "player_id=? AND left_at=0", ids[0]).Error; err != nil || active.MatchID == match.ID {
		t.Fatal("genuine second round did not replace active scope", err)
	}
	var profile db.Profile
	if err = db.GetIns().First(&profile, ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	_, game, err := decodeProfile(&profile)
	if err != nil || !game.PositionLoading {
		t.Fatal("same scene new scope did not arm loading gate", err)
	}
	if reply, err := f.s.p13UFFieldMove(f.ctx, ids[0], &pb.CSMove{PointNew: oldPoint}, now()); err != nil || len(reply) != 0 {
		t.Fatal("old round queued point was applied", err)
	}
	var unchanged db.P13UnionFightFieldPlayer
	if err = db.GetIns().First(&unchanged, "match_id=? AND player_id=?", active.MatchID, ids[0]).Error; err != nil || active.X != unchanged.X || active.Y != unchanged.Y || active.Z != unchanged.Z || active.FrameHash != unchanged.FrameHash {
		t.Fatal("old round queued point modified new instance", err)
	}
	frames, err = f.s.p13UFFieldFrames(f.ctx, ids[0], &pb.CSCurrentPosition{}, now())
	if err != nil || frames[1].(*pb.SCUnionFightTransfer).GetUnionFightId() != active.MatchID {
		t.Fatal("new actual Current instance", err)
	}
	db.GetIns().First(&profile, ids[0])
	_, game, err = decodeProfile(&profile)
	if err != nil || game.PositionLoading {
		t.Fatal("new actual Current did not release loading gate", err)
	}
}
