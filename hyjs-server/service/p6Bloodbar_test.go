package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"reflect"
	"testing"
)

// p6OriginalMultiBarInput uses the original Task52 boss row, including total HP and skills.
func p6OriginalMultiBarInput(t *testing.T, s *GameService) *model.FrozenBattleInput {
	t.Helper()
	enemy, err := s.actor(s.table("CopyEnemy", "CopyEnemyId", "133"), "original-boss", 3, true)
	if err != nil {
		t.Fatal(err)
	}
	if enemy.GetBloodNumber() != 2 || enemy.Prop.GetHp() != 1104 {
		t.Fatal("original multi-bar enemy changed", enemy)
	}
	attacker := p6Actor("hero_047", "player", "047_1,047_2,047_3", 1, 40000)
	input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: &pb.BattlePlayer{PlayerId: proto.Int64(42), Formation: []*pb.BattleHero{attacker}}, Monster: &pb.BattleMonster{Formation: []*pb.BattleHero{enemy}}, Seed: 0, MaxRound: 40, BattleType: 1})
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func TestP6OriginalMultiBloodbarTotalHPWholeBattleAndReplay(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6OriginalMultiBarInput(t, s)
	out, err := s.ComputeFrozenBattle(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Winner != 0 || out.Authoritative || out.Monster.Formation[0].GetBloodNumber() != 2 || out.Monster.Formation[0].Inst.GetCurHp() != 0 {
		t.Fatal("multi-bar whole battle changed", out.Winner)
	}
	weak, dead, crossed := 0, 0, false
	for _, event := range out.Events {
		if event.Actor == "1_original-boss" && event.Kind == "weak_dead" {
			weak++
		}
		if event.Actor == "1_original-boss" && event.Kind == "dead" {
			dead++
		}
		if event.Target == "1_original-boss" && event.Kind == "damage" && event.TargetHP > 0 && event.TargetHP < 552 {
			crossed = true
		}
	}
	if weak != 1 || dead != 1 || !crossed {
		t.Fatal("visual bar boundary created wrong lifetime", weak, dead, crossed)
	}
	replay, err := s.ComputeFrozenBattle(t.Context(), input, out.Operates)
	if err != nil || !proto.Equal(out.Operates, replay.Operates) || !proto.Equal(out.Monster, replay.Monster) || !reflect.DeepEqual(out.Events, replay.Events) || out.RandomDraws != replay.RandomDraws {
		t.Fatal("multi-bar replay diverged", err)
	}
	// Only display metadata differs: changing the raw bar count cannot multiply HP or alter RNG.
	for _, bars := range []int32{0, 1, 3, 5, 6} {
		enemy := proto.Clone(input.Monster).(*pb.BattleMonster)
		enemy.Formation[0].BloodNumber = proto.Int32(bars)
		variant, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: input.Attacker, Monster: enemy, Seed: 0, MaxRound: 40, BattleType: 1})
		if err != nil {
			t.Fatal(err)
		}
		other, err := s.ComputeFrozenBattle(t.Context(), variant, nil)
		if err != nil || other.Winner != out.Winner || !proto.Equal(out.Operates, other.Operates) || other.RandomDraws != out.RandomDraws || !reflect.DeepEqual(out.Events, other.Events) || other.Monster.Formation[0].GetBloodNumber() != bars {
			t.Fatal("bar display count altered original computation", bars, err)
		}
	}
}
