package service

import (
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"
)

func TestP6OriginalSubjectCampZeroBattleEndPriorityAllTypes(t *testing.T) {
	s := p6OriginalTables(t)
	for battleType := int32(0); battleType <= 20; battleType++ {
		for _, hp := range [][2]float64{{0, 0}, {0, 1000}, {1000, 0}} {
			a := p6Actor("hero_047", "subject", "047_1", 1, 1000)
			d := p6Actor("hero_047", "opponent", "047_1", 1, 1000)
			a.Inst.CurHp, d.Inst.CurHp = proto.Float64(hp[0]), proto.Float64(hp[1])
			input := p6Input(t, s, a, d, 1)
			input.BattleType = battleType
			out, err := s.ComputeFrozenBattle(t.Context(), input, nil)
			if err != nil {
				t.Fatal(battleType, hp, err)
			}
			winner := int32(1)
			if hp[1] == 0 {
				winner = 0
			}
			if out.Winner != winner || out.FinalRound != 1 || len(out.Operates.GetSkillOpts()) != 0 || out.Authoritative {
				t.Fatal("original subject0 enemy-dead priority lost", battleType, hp, out.Winner)
			}
		}
	}
}

func TestP6OriginalRoundLimitIsDefenderVictoryAndFullReplay(t *testing.T) {
	s := p6OriginalTables(t)
	for _, first := range []bool{false, true} {
		input := p6Input(t, s, p6Actor("hero_047", "subject", "047_1", 1, 1000000000), p6Actor("hero_047", "opponent", "047_1", 1, 1000000000), 1)
		input.DefenderFirst = first
		if err := SetP6FirstOrder(input.Attacker, input.Defender, first); err != nil {
			t.Fatal(err)
		}
		out, err := s.ComputeFrozenBattle(t.Context(), input, nil)
		if err != nil || out.Winner != 1 || out.FinalRound != input.MaxRound+1 || out.Operates.GetAttackerTotalLeftHp() <= 0 || out.Operates.GetDefenderTotalLeftHp() <= 0 {
			t.Fatal("original max-round subject0 loss became a draw", first, err)
		}
		replay, err := s.ComputeFrozenBattle(t.Context(), input, out.Operates)
		if err != nil || replay.Winner != 1 || !proto.Equal(out.Operates, replay.Operates) || !reflect.DeepEqual(out.Events, replay.Events) || out.RandomDraws != replay.RandomDraws {
			t.Fatal("original round-limit full replay drift", first, err)
		}
	}
}

func TestP6OriginalRoundExhaustionComputedAndObservedReceiptBounds(t *testing.T) {
	s, ctx, id, input := p6Recorder(t)
	input.MaxRound = 1
	for _, player := range []*pb.BattlePlayer{input.Attacker, input.Defender} {
		player.Formation[0].Prop.Hp = proto.Float64(1e9)
		player.Formation[0].Inst.CurHp = proto.Float64(1e9)
	}
	out, err := s.ComputeFrozenBattle(ctx, input, nil)
	if err != nil || out.FinalRound != 2 {
		t.Fatal("original exhausted round boundary", err)
	}
	for _, op := range out.Operates.SkillOpts {
		if op.GetRoundNum() > 1 {
			t.Fatal("skill executed in end-only round")
		}
	}
	for _, name := range []string{"computed-limit", "observed-limit", "invalid-skill-limit", "invalid-final-limit"} {
		if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
			_, captureErr := s.CaptureBattleReplay(ctx, tx, id, "round-exhaustion", name, *input)
			return captureErr
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		_, computeErr := s.ComputeBattleReplay(ctx, tx, id, "round-exhaustion", "computed-limit", out.Operates)
		return computeErr
	}); err != nil {
		t.Fatal("max+1 terminal compute receipt", err)
	}
	if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
		return s.FinishObservedBattleReplay(ctx, tx, id, "round-exhaustion", "observed-limit", out.Operates, false)
	}); err != nil {
		t.Fatal("max+1 terminal observed receipt", err)
	}
	for _, name := range []string{"computed-limit", "observed-limit"} {
		frames, queryErr := s.fightPlayback(ctx, id, &pb.CSFightPlayback{FightId: proto.String(name), ServerId: proto.Int32(1)})
		if queryErr != nil || !proto.Equal(frames[0].(*pb.SCFightPlayback).OptRecord, out.Operates) {
			t.Fatal("terminal round original query wire", queryErr)
		}
	}
	badSkill := proto.Clone(out.Operates).(*pb.BattleOperate)
	badSkill.SkillOpts = append(badSkill.SkillOpts, &pb.BattleSkillOperate{RoundNum: proto.Int32(2), RoundTime: proto.Float64(0), Camp: proto.Int32(1), ActorUid: proto.String(input.Defender.Formation[0].GetGuid()), SkillIndex: proto.Int32(0)})
	badFinal := proto.Clone(out.Operates).(*pb.BattleOperate)
	badFinal.FinalRoundNum = proto.Int32(3)
	for _, sample := range []struct {
		name    string
		operate *pb.BattleOperate
	}{{"invalid-skill-limit", badSkill}, {"invalid-final-limit", badFinal}} {
		if err = p6Tx(ctx, id, func(tx *gorm.DB) error {
			return s.FinishObservedBattleReplay(ctx, tx, id, "round-exhaustion", sample.name, sample.operate, false)
		}); err == nil {
			t.Fatal("invalid observed round accepted", sample.name)
		}
		var row db.P6BattleRecord
		if err = db.GetIns().First(&row, "player_id=? AND source_fight=?", id, sample.name).Error; err != nil || row.Status != constUtil.P6BattleCaptured || len(row.Operates) != 0 {
			t.Fatal("invalid round mutated captured receipt", err)
		}
		if _, err = s.ComputeFrozenBattle(ctx, input, sample.operate); err == nil {
			t.Fatal("invalid computed round accepted", sample.name)
		}
	}
}
