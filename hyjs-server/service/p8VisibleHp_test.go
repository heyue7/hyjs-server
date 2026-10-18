package service

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// 测试使用真实建号/转场/在线订阅，HP为明确隔离状态，未冒称自然战斗伤害。
func p8VisibleHpSetup(t *testing.T) (*p1Fixture, int64, func()) {
	t.Helper()
	f, target, _ := p10SceneSetup(t)
	for _, id := range []int64{f.id, target} {
		if _, err := f.s.fieldTransfer(f.ctx, id, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.fieldScene(f.ctx, id, false); err != nil {
			t.Fatal(err)
		}
	}
	p10SceneConnect(t, f, f.id, &p10SceneRecorder{})
	closeTarget := p10SceneConnect(t, f, target, &p10SceneRecorder{})
	return f, target, closeTarget
}

func TestP8VisibleHpReadsCurrentProfileAndNeverHeals(t *testing.T) {
	f, target, _ := p8VisibleHpSetup(t)
	for _, hp := range []float64{0.375, 0} {
		if err := updateProfile(f.ctx, target, func(p *pb.Player, g *model.GameState) error {
			g.Field.Side.CurHp = proto.Float64(hp)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		var before db.Profile
		if err := db.GetIns().First(&before, target).Error; err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ {
			// 不刷新同屏帧；查询必须读取新存档，不返回旧满血缓存。
			messages, handled, err := f.s.DispatchP8Life(f.ctx, f.id, &pb.CSGetHp{PlayerId: proto.Int64(target)}, 0)
			if err != nil || !handled || len(messages) != 1 || messages[0].(*pb.SCGetHp).GetHp() != hp {
				t.Fatalf("current visible HP=%v: handled=%v err=%v", hp, handled, err)
			}
		}
		var after db.Profile
		if err := db.GetIns().First(&after, target).Error; err != nil {
			t.Fatal(err)
		}
		if *before.Player != *after.Player || *before.Game != *after.Game {
			t.Fatal("another player's HP query changed their persisted state")
		}
	}
}

func TestP8VisibleHpRejectsForeignLineLoadingAndStaleSession(t *testing.T) {
	f, target, closeTarget := p8VisibleHpSetup(t)
	var original db.P10ScenePresence
	if err := db.GetIns().First(&original, "player_id=?", target).Error; err != nil {
		t.Fatal(err)
	}
	deny := func() {
		t.Helper()
		if _, _, err := f.s.DispatchP8Life(f.ctx, f.id, &pb.CSGetHp{PlayerId: proto.Int64(target)}, 0); err == nil {
			t.Fatal("unavailable peer HP was exposed")
		}
	}
	if err := db.GetIns().Model(&db.P10ScenePresence{}).Where("player_id=?", target).Update("line", 2).Error; err != nil {
		t.Fatal(err)
	}
	deny()
	if err := db.GetIns().Save(&original).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, target, func(p *pb.Player, g *model.GameState) error { g.PositionLoading = true; return nil }); err != nil {
		t.Fatal(err)
	}
	deny()
	if err := updateProfile(f.ctx, target, func(p *pb.Player, g *model.GameState) error {
		g.PositionLoading = false
		g.Run = &model.CopyRun{Complete: false}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	deny()
	if err := updateProfile(f.ctx, target, func(p *pb.Player, g *model.GameState) error { g.Run = nil; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P10ScenePresence{}).Where("player_id=?", target).Update("session_hash", "stale").Error; err != nil {
		t.Fatal(err)
	}
	deny()
	if err := db.GetIns().Save(&original).Error; err != nil {
		t.Fatal(err)
	}
	closeTarget()
	deny()
	p10SceneConnect(t, f, target, &p10SceneRecorder{})
	if _, _, err := f.s.DispatchP8Life(f.ctx, f.id, &pb.CSGetHp{PlayerId: proto.Int64(target)}, 0); err != nil {
		t.Fatal("current session reconnect did not restore visibility", err)
	}
	if err := db.GetIns().Model(&db.P10ScenePolicy{}).Where("id=?", 1).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	deny()
}
