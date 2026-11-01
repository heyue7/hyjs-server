package service

import (
	"math"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP8Town77TransferPersistsAndSettlesOnce(t *testing.T) {
	f := p1Setup(t)
	f.s.P8LifeReady = true
	for _, load := range []func(string) error{f.s.LoadP5Tables, f.s.LoadP8Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func() error{
		func() error { return f.s.InitializeP5(f.ctx) },
		func() error { return f.s.InitializeP8Gather(f.ctx) },
		func() error { return f.s.InitializeP8Life(f.ctx) },
	} {
		if err := init(); err != nil {
			t.Fatal(err)
		}
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(30); return nil }); err != nil {
		t.Fatal(err)
	}
	transfer := func(target string) {
		t.Helper()
		if _, err := f.s.fieldTransfer(f.ctx, f.id, &pb.CSTransferScene{Target: proto.String(target)}); err != nil {
			t.Fatal(err)
		}
	}
	transfer("field_001")
	if err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		g.Field.Side.CurHp = proto.Float64(.2)
		g.Field.Side.BattlePosHps = nil
		safe, err := f.s.p8SafeZone(g)
		if err != nil || !safe {
			t.Fatal("expected original safe spawn", safe, err)
		}
		return tx.Model(&db.P8LifeState{}).Where("player_id=?", f.id).Updates(map[string]any{"in_safe": true, "safe_recovery_at": time.Now().Add(-15 * time.Second).UnixMilli()}).Error
	}); err != nil {
		t.Fatal(err)
	}
	transfer("town_main")
	_, before := f.profile(t)
	if before.Field.TownRecoveryAt <= 0 || math.Abs(before.Field.Side.GetCurHp()-.3) > 1e-9 {
		t.Fatal("return baseline/HP", before.Field)
	}
	// A repeated town alias and passive query cannot advance or consume the baseline.
	transfer("town")
	if err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		_, err := f.s.RefreshP8Survival(tx, p, g, time.Now().Add(time.Hour))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_, after := f.profile(t)
	if after.Field.TownRecoveryAt != before.Field.TownRecoveryAt || after.Field.Side.GetCurHp() != before.Field.Side.GetCurHp() {
		t.Fatal("passive town request consumed healing")
	}
	// A legacy safe-zone baseline must not count the town interval a second time.
	if err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		g.Field.TownRecoveryAt = time.Now().Add(-15 * time.Second).UnixMilli()
		return tx.Model(&db.P8LifeState{}).Where("player_id=?", f.id).Updates(map[string]any{"in_safe": true, "safe_recovery_at": time.Now().Add(-time.Hour).UnixMilli()}).Error
	}); err != nil {
		t.Fatal(err)
	}
	transfer("field_001")
	_, after = f.profile(t)
	if math.Abs(after.Field.Side.GetCurHp()-.4) > 1e-9 || after.Field.TownRecoveryAt != 0 {
		t.Fatal("town and safe clocks counted the same elapsed interval twice", after.Field.Side.GetCurHp())
	}
	transfer("town_main")
	// The time fixture avoids sleeping and retains the actual transfer/DB path.
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		g.Field.TownRecoveryAt = time.Now().Add(-time.Hour).UnixMilli()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	transfer("field_001")
	_, after = f.profile(t)
	if math.Abs(after.Field.Side.GetCurHp()-1) > 1e-9 || after.Field.TownRecoveryAt != 0 {
		t.Fatal("cold transfer did not settle town healing", after.Field)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { return f.s.p8Heal(p, g, .4, true) }); err != nil {
		t.Fatal(err)
	}
	transfer("field_001")
	_, after = f.profile(t)
	if math.Abs(after.Field.Side.GetCurHp()-.4) > 1e-9 {
		t.Fatal("transfer retry reused town interval")
	}
}

func TestP8Town77ClockAndDeath(t *testing.T) {
	s := &GameService{}
	now := time.Now()
	for _, hp := range []float64{0, 1} {
		g := &model.GameState{Field: &model.FieldState{TownRecoveryAt: now.Add(-time.Hour).UnixMilli(), Side: &pb.PlayerSide{CurHp: proto.Float64(hp)}}}
		if err := s.p8TownTransferRecovery(&pb.Player{}, g, "town_main", "field_001", now); err != nil || g.Field.Side.GetCurHp() != hp || g.Field.TownRecoveryAt != 0 {
			t.Fatal("dead/full transfer changed HP", err)
		}
	}
	g := &model.GameState{Field: &model.FieldState{TownRecoveryAt: now.Add(time.Hour).UnixMilli(), Side: &pb.PlayerSide{CurHp: proto.Float64(.4)}}}
	if err := s.p8TownTransferRecovery(&pb.Player{}, g, "town_main", "field_001", now); err != nil || g.Field.Side.GetCurHp() != .4 || g.Field.TownRecoveryAt != 0 {
		t.Fatal("backward clock accrued HP", err)
	}
}

// Movement can be the request that advances the safe-zone clock. The same
// authenticated response must carry that committed HP to the original client.
func TestP8MoveReturnsCommittedSafeRecovery(t *testing.T) {
	f := p1Setup(t)
	f.s.P8LifeReady = true
	for _, load := range []func(string) error{f.s.LoadP5Tables, f.s.LoadP8Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func() error{
		func() error { return f.s.InitializeP5(f.ctx) },
		func() error { return f.s.InitializeP8Gather(f.ctx) },
		func() error { return f.s.InitializeP8Life(f.ctx) },
	} {
		if err := init(); err != nil {
			t.Fatal(err)
		}
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Level = proto.Int32(30); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.fieldTransfer(f.ctx, f.id, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
		t.Fatal(err)
	}
	if err := updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		g.PositionLoading = false
		g.Field.Side.CurHp = proto.Float64(.2)
		g.Field.Side.BattlePosHps = nil
		g.Field.Fight = nil
		safe, err := f.s.p8SafeZone(g)
		if err != nil || !safe {
			t.Fatal("expected safe spawn", safe, err)
		}
		return tx.Model(&db.P8LifeState{}).Where("player_id=?", f.id).Updates(map[string]any{"in_safe": true, "safe_recovery_at": time.Now().Add(-15 * time.Second).UnixMilli()}).Error
	}); err != nil {
		t.Fatal(err)
	}
	var owner db.Profile
	if err := db.GetIns().First(&owner, f.id).Error; err != nil {
		t.Fatal(err)
	}
	_, before := f.profile(t)
	move := &pb.CSMove{PointNew: proto.Clone(before.Position.Current).(*pb.PointNew)}
	packet := &model.GamePacket{Opcode: 30026, PlayerId: f.id, ZoneId: 1, Session: owner.Session, Body: move}
	result := f.s.Dispatch(f.ctx, packet, "127.0.0.1")
	var healed *pb.PlayerSide
	for _, message := range result {
		if failure, ok := message.(*pb.ErrorMessage); ok {
			t.Fatal(failure.GetMsg())
		}
		if state, ok := message.(*pb.SCFieldPlayerState); ok && len(state.GetPlayer()) == 1 {
			healed = state.GetPlayer()[0]
		}
	}
	_, saved := f.profile(t)
	if healed == nil || math.Abs(healed.GetCurHp()-.3) > 1e-9 || math.Abs(saved.Field.Side.GetCurHp()-.3) > 1e-9 {
		t.Fatal("committed safe recovery was not returned to the moving client", healed, saved.Field.Side.GetCurHp())
	}
	for _, message := range f.s.Dispatch(f.ctx, packet, "127.0.0.1") {
		if _, ok := message.(*pb.SCFieldPlayerState); ok {
			t.Fatal("same recovery interval emitted twice")
		}
		if failure, ok := message.(*pb.ErrorMessage); ok {
			t.Fatal(failure.GetMsg())
		}
	}
	_, repeated := f.profile(t)
	if math.Abs(repeated.Field.Side.GetCurHp()-.3) > 1e-9 {
		t.Fatal("repeated move granted recovery twice")
	}
}
