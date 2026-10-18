package service

import (
	"context"
	"errors"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

func p6Recorder(t *testing.T) (*GameService, context.Context, int64, *model.FrozenBattleInput) {
	t.Helper()
	s, ctx, _ := newDailyCopyTest(t)
	if err := s.InitializeP6(ctx); err != nil {
		t.Fatal(err)
	}
	id := dailyCopyTestPlayer(t, ctx, "p6-recorder", 10)
	f := p6Input(t, s, p6Actor("hero_047", "player", "047_1", 1, 1000), p6Actor("hero_047", "enemy", "047_1", 1, 1000), 12)
	f.Attacker.PlayerId = proto.Int64(id)
	return s, ctx, id, f
}
func p6Tx(ctx context.Context, id int64, fn func(*gorm.DB) error) error {
	return updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error { return fn(tx) })
}
func TestP6FrozenReplayAtomicLifecycleAndPlayback(t *testing.T) {
	s, ctx, id, input := p6Recorder(t)
	var recordID string
	if err := p6Tx(ctx, id, func(tx *gorm.DB) error {
		var err error
		recordID, err = s.CaptureBattleReplay(ctx, tx, id, "test", "fight", *input)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(recordID)}); err == nil {
		t.Fatal("pending replay readable")
	}
	out, err := s.ComputeFrozenBattle(ctx, input, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		return s.FinishObservedBattleReplay(ctx, tx, id, "test", "fight", out.Operates, out.Winner == 0)
	}); err != nil {
		t.Fatal(err)
	}
	reply, err := s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String("fight"), ServerId: proto.Int32(1)})
	if err != nil {
		t.Fatal(err)
	}
	play := reply[0].(*pb.SCFightPlayback)
	if play.GetRandSeed() != input.Seed || play.GetFightid() != recordID || !proto.Equal(play.GetOptRecord(), out.Operates) || !proto.Equal(play.GetAttacker(), input.Attacker) || play.GetClientResult() != play.GetServerResult() {
		t.Fatal("original replay mismatched", play)
	}
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		return s.FinishObservedBattleReplay(ctx, tx, id, "test", "fight", out.Operates, out.Winner == 0)
	}); err != nil {
		t.Fatal("idempotent finish", err)
	}
	bad := proto.Clone(out.Operates).(*pb.BattleOperate)
	bad.AttackerTotalLeftHp = proto.Float64(1)
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		return s.FinishObservedBattleReplay(ctx, tx, id, "test", "fight", bad, out.Winner == 0)
	}); err == nil {
		t.Fatal("conflicting finish accepted")
	}
	if _, err = s.fightPlayback(ctx, id+1, &pb.CSFightPlayback{FightId: proto.String(recordID)}); err == nil {
		t.Fatal("foreign replay accepted")
	}
	var record db.P6BattleRecord
	db.GetIns().First(&record, "id=?", recordID)
	if record.Authoritative || record.Status != constUtil.P6BattleObserved {
		t.Fatal(record.Status)
	}
	s.table("SkillCast", "SkillCastId", "895")["ExportFactor"] = "999"
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		got, err := s.CaptureBattleReplay(ctx, tx, id, "test", "fight", *input)
		if got != recordID {
			t.Fatal("source retry changed identity")
		}
		return err
	}); err != nil {
		t.Fatal("hot-config retry", err)
	}
	changed := *input
	changed.DefenderFirst = true
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		_, err := s.CaptureBattleReplay(ctx, tx, id, "test", "fight", changed)
		return err
	}); err == nil {
		t.Fatal("changed turn order accepted")
	}
	if err = db.GetIns().Model(&record).Update("input", []byte("{} ")).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(recordID)}); err == nil {
		t.Fatal("tampered input accepted")
	}
}
func TestP6ComputeReceiptRollbackCancelTimeoutAndLegacyCompatibility(t *testing.T) {
	s, ctx, id, input := p6Recorder(t)
	rollback := errors.New("resource grant failed")
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		p.Coin = proto.Int64(p.GetCoin() + 500)
		if _, err := s.CaptureBattleReplay(ctx, tx, id, "test", "rolled-back", *input); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	var count int64
	db.GetIns().Model(&db.P6BattleRecord{}).Where("source_fight=?", "rolled-back").Count(&count)
	if count != 0 {
		t.Fatal("capture survived profile rollback")
	}
	for _, name := range []string{"computed", "cancelled", "expired"} {
		if err := p6Tx(ctx, id, func(tx *gorm.DB) error {
			_, err := s.CaptureBattleReplay(ctx, tx, id, "test", name, *input)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	var first *model.CombatOutcome
	if err := p6Tx(ctx, id, func(tx *gorm.DB) error {
		var err error
		first, err = s.ComputeBattleReplay(ctx, tx, id, "test", "computed", nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if first.Authoritative {
		t.Fatal("uncompared engine opened")
	}
	if err := p6Tx(ctx, id, func(tx *gorm.DB) error {
		again, err := s.ComputeBattleReplay(ctx, tx, id, "test", "computed", first.Operates)
		if err == nil && !proto.Equal(first.Operates, again.Operates) {
			t.Fatal("computed retry diverged")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := p6Tx(ctx, id, func(tx *gorm.DB) error { return s.CancelBattleReplay(ctx, tx, id, "test", "cancelled") }); err != nil {
		t.Fatal(err)
	}
	if err := p6Tx(ctx, id, func(tx *gorm.DB) error {
		_, err := s.ComputeBattleReplay(ctx, tx, id, "test", "cancelled", nil)
		return err
	}); err == nil {
		t.Fatal("cancelled executed")
	}
	db.GetIns().Model(&db.P6BattleRecord{}).Where("source_fight=?", "expired").Update("expires_at", time.Now().Add(-time.Second).UnixMilli())
	if err := p6Tx(ctx, id, func(tx *gorm.DB) error {
		_, err := s.ComputeBattleReplay(ctx, tx, id, "test", "expired", nil)
		return err
	}); err == nil {
		t.Fatal("expired executed")
	}
	legacy := &pb.SCCopyFightStart{FightId: proto.String("pre-P6"), BattlePlayer: input.Attacker, BattleMonsterList: []*pb.BattleMonster{{}}}
	if err := p6Tx(ctx, id, func(tx *gorm.DB) error {
		if err := s.FinishOriginalCopyReplay(ctx, tx, id, legacy, &pb.CSCopyFightEnd{OptRecord: &pb.BattleOperate{}}); err != nil {
			return err
		}
		return s.CancelOriginalCopyReplay(ctx, tx, id, legacy)
	}); err != nil {
		t.Fatal("old native fight blocked", err)
	}
}
