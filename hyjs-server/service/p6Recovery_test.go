package service

import (
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"testing"
)

func TestP6RecoveryPreservesFrozenVersionAndChecksStoredResult(t *testing.T) {
	s, ctx, id, input := p6Recorder(t)
	var recordID string
	if err := p6Tx(ctx, id, func(tx *gorm.DB) error {
		var err error
		recordID, err = s.CaptureBattleReplay(ctx, tx, id, "recover", "reconnect", *input)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	s.table("SkillCast", "SkillCastId", "895")["ExportFactor"] = "999"
	if err := p6Tx(ctx, id, func(tx *gorm.DB) error {
		frozen, err := s.RecoverFrozenBattle(ctx, tx, id, "recover", "reconnect")
		if err == nil && (frozen.ConfigurationHash != input.ConfigurationHash || frozen.Seed != input.Seed || !proto.Equal(frozen.Attacker, input.Attacker)) {
			t.Fatal("reconnect loaded mutable state")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := p6Tx(ctx, id, func(tx *gorm.DB) error {
		_, err := s.ComputeBattleReplay(ctx, tx, id, "recover", "reconnect", nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(recordID)}); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P6BattleRecord{}).Where("id=?", recordID).Update("server_result", 2).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(recordID)}); err == nil {
		t.Fatal("stored result tamper accepted")
	}
	if err := p6Tx(ctx, id, func(tx *gorm.DB) error {
		_, err := s.ComputeBattleReplay(ctx, tx, id, "recover", "reconnect", nil)
		return err
	}); err == nil {
		t.Fatal("computed receipt tamper accepted")
	}
}

func TestP6ReplayRejectsChangedSummaryAndPreservesUnusedVictoryTransport(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6Input(t, s, p6Actor("hero_047", "a", "047_1", 1, 1000), p6Actor("hero_047", "d", "047_1", 1, 1000), 12)
	out, err := s.ComputeFrozenBattle(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	changed := proto.Clone(out.Operates).(*pb.BattleOperate)
	changed.FinalRoundNum = proto.Int32(out.FinalRound + 1)
	if _, err = s.ComputeFrozenBattle(t.Context(), input, changed); err == nil {
		t.Fatal("changed final round accepted")
	}
	changed = proto.Clone(out.Operates).(*pb.BattleOperate)
	changed.AttackerTotalLeftHp = proto.Float64(changed.GetAttackerTotalLeftHp() + 1)
	if _, err = s.ComputeFrozenBattle(t.Context(), input, changed); err == nil {
		t.Fatal("client claimed hp accepted")
	}
	changed = proto.Clone(out.Operates).(*pb.BattleOperate)
	changed.SkillOpts[0].RoundTime = proto.Float64(.05)
	if _, err = s.ComputeFrozenBattle(t.Context(), input, changed); err == nil {
		t.Fatal("off-frame operation accepted")
	}
	input.WinConditions = []*pb.BattleWinCondition{{}}
	transported, err := s.ComputeFrozenBattle(t.Context(), input, nil)
	if err != nil || !proto.Equal(transported.Operates, out.Operates) || transported.Winner != out.Winner || transported.RandomDraws != out.RandomDraws {
		t.Fatal("original unused victory transport altered default rules", err)
	}
}

func TestP6OriginalRoundRecoveryShieldCoverAndDebuffTrigger(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6Input(t, s, p6Actor("hero_047", "a", "047_1", 1, 1000), p6Actor("hero_047", "d", "047_1", 1, 1000), 12)
	e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	a := e.players[0].actors[0]
	a.hp = 200
	heal := map[string]string{"Id": "round-recover", "Factor": "recoverBySelfRound", "Args1": "10", "TotalRound": "2"}
	if err := e.loadBuff(a, a, heal, 1); err != nil {
		t.Fatal(err)
	}
	// Source attack is 500 at load, so a later attack change cannot alter the frozen heal.
	a.base["att"] = 1000
	e.round, e.camp = 1, 0
	if err := e.beginRound(); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, a.hp, 250)
	shield := map[string]string{"Id": "covered-shield", "Group": "shield-group", "Factor": "shield", "Args1": "100", "TotalRound": "2", "ShouldCover": "1"}
	if err := e.loadBuff(a, a, shield, 4); err != nil {
		t.Fatal(err)
	}
	e.damageHP(e.players[1].actors[0], a, 30, 1)
	if err := e.loadBuff(a, a, shield, 4); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, a.shield("shield"), 100)
	s.Tables = input.Tables
	s.Tables["Buff"] = append(s.Tables["Buff"], map[string]string{"Id": "debuff-reaction", "Factor": "anger", "Args1": "5"})
	a.talents = append(a.talents, combatRoundTrigger{buff: "debuff-reaction", timer: 27, chance: 1})
	before := e.players[0].anger
	if err := e.loadBuff(e.players[1].actors[0], a, map[string]string{"Id": "incoming-debuff", "Factor": "dizzy", "IsDebuff": "1", "TotalRound": "1"}, 4); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, e.players[0].anger, before+5)
	if e.rng.Draws() != 1 {
		t.Fatal("debuff reaction RNG order", e.rng.Draws())
	}
	if !e.conditionTrue(combatRoundTrigger{condition: 1, args: "type:hp,src1:2,type2:maxHp,src2:2,op:4"}, a, nil) {
		t.Fatal("different-property condition rejected")
	}
	if !e.conditionTrue(combatRoundTrigger{condition: 3, args: "type:hero_047,type2:hp,src1:3,src2:1,src2_val:50,op:4"}, a, a) {
		t.Fatal("named camp hero hp condition rejected")
	}
}
