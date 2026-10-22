package service

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestField45BossCountdownUsesSavedDeadlineAndClearsOldMap(t *testing.T) {
	now := time.UnixMilli(1792600000000)
	field := &model.FieldState{
		Bosses: []*pb.SideBoss{
			{Uuid: proto.String("dead"), Nature: proto.Int32(0), State: proto.Int32(3)},
			{Uuid: proto.String("live"), Nature: proto.Int32(1), State: proto.Int32(1)},
		},
		RespawnAt: map[string]int64{"dead": now.UnixMilli() + 3600000, "live": now.UnixMilli() + 90000},
	}
	first := fieldBossCountdownProjection(field, now)
	second := fieldBossCountdownProjection(field, now.Add(2*time.Second))
	if len(first) != 6 || first[0].(*pb.SCSideBossCountdown).GetTime() != 3600 || second[0].(*pb.SCSideBossCountdown).GetTime() != 3598 {
		t.Fatal("saved deadline was not projected in seconds")
	}
	for _, value := range fieldBossCountdownProjection(nil, now) {
		if value.(*pb.SCSideBossCountdown).GetTime() != 0 {
			t.Fatal("previous map's native countdown was not cleared")
		}
	}
	if first[1].(*pb.SCSideBossCountdown).GetTime() != 0 || field.RespawnAt["dead"] != now.UnixMilli()+3600000 {
		t.Fatal("live boss exposed a cooldown or projection changed saved state")
	}
}

func TestField45SnowBattleRequiresPackageWithSnowCompatibility(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.InitializeClientUpdates(f.ctx); err != nil {
		t.Fatal(err)
	}
	key := clientUpdateDBKey(db.GetIns())
	t.Cleanup(func() { clientUpdateReady.Delete(key) })
	var profile db.Profile
	if err := db.GetIns().First(&profile, f.id).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.BindClientLogin(f.ctx, &profile, "hyjs/2026102201/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	if checkFieldBattleClient(db.GetIns(), f.id, "field_001") != nil || checkFieldBattleClient(db.GetIns(), f.id, "field_002") == nil {
		t.Fatal("legacy package was not limited to compatible field battle routes")
	}
	if err := f.s.BindClientLogin(f.ctx, &profile, "hyjs/2026102202/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"); err != nil {
		t.Fatal(err)
	}
	if err := checkFieldBattleClient(db.GetIns(), f.id, "field_002"); err != nil {
		t.Fatal("qualified package was blocked", err)
	}
}

func TestField45SnowMonsterStartAndFieldToTownGate(t *testing.T) {
	f := p1Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeDailyCopy(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Level = proto.Int32(40)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, scene := range []string{"field_001", "field_002"} {
		if _, err := f.s.fieldTransfer(f.ctx, f.id, &pb.CSTransferScene{Target: proto.String(scene)}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.fieldScene(f.ctx, f.id, false); err != nil {
			t.Fatal(err)
		}
		_, state := fieldBoss33State(t, f.id)
		if scene == "field_002" {
			started, err := f.s.fieldFightStart(f.ctx, f.id, &pb.CSSidePveFightStart{MonsterId: state.Field.Monsters[0].Uuid})
			if err != nil || started[0].(*pb.SCSidePveFightStart).GetCode() != 1 || len(started[0].(*pb.SCSidePveFightStart).GetBattleMonsterList()) == 0 {
				t.Fatal("original snow monster failed to start", err)
			}
			// Reconnect releases the real unfinished fight without a result/reward.
			if _, err := f.s.fieldScene(f.ctx, f.id, true); err != nil {
				t.Fatal(err)
			}
		}
		portal := &pb.PointNew{X: proto.Float32(-3.9), Y: proto.Float32(.12), Z: proto.Float32(-6.4)}
		if _, err := f.s.fieldTransfer(f.ctx, f.id, &pb.CSTransferScene{Target: proto.String("town"), Point: portal}); err != nil {
			t.Fatal(err)
		}
		_, returned := fieldBoss33State(t, f.id)
		gate, err := fieldPoint("41.11,-2.25,-42.33")
		if err != nil || !proto.Equal(returned.Position.Current, gate) || !returned.PositionLoading {
			t.Fatal("field return did not use original area_25 gate and loading handshake")
		}
	}
	// A town-local transfer still uses its own original destination.
	point, _ := fieldPoint("13.76,.12,.32")
	if _, err := f.s.fieldTransfer(f.ctx, f.id, &pb.CSTransferScene{Target: proto.String("town"), Point: point}); err != nil {
		t.Fatal(err)
	}
	_, state := fieldBoss33State(t, f.id)
	if !proto.Equal(state.Position.Current, point) {
		t.Fatal("town-local destination changed")
	}
}
