package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

func p6WeakReleaseInput(t *testing.T, s *GameService) *model.FrozenBattleInput {
	a := p6Actor("hero_001", "multi-hit-source", "001_1", 1, 2000)
	a.Inst.CurHp = proto.Float64(1000)
	input := p6Input(t, s, a, p6Actor("hero_047", "held-target", "047_1", 1, 1), 1)
	input.Defender.Formation = append(input.Defender.Formation, p6Actor("hero_047", "next-target", "047_1", 2, 2000))
	return input
}

func TestP6OriginalFirstTargetWeakReleaseAngerLifestealAndReplay(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6WeakReleaseInput(t, s)
	out, err := s.ComputeFrozenBattle(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	releases, heals, dead := 0, 0, 0
	for _, event := range out.Events {
		if event.Kind == "release" && event.Skill == "001_1" {
			releases++
			if event.Target != "1_held-target" {
				t.Fatal("FIRST re-targeted after lethal first hit", event)
			}
		}
		if event.Kind == "lifesteal" && event.Actor == "0_multi-hit-source" {
			heals++
		}
		if event.Kind == "dead" && event.Actor == "1_held-target" {
			dead++
			if releases != 3 {
				t.Fatal("held target finalized before last released cast", releases)
			}
		}
	}
	if releases != 3 || heals != 3 || dead != 1 || out.Defender.Formation[1].Inst.GetCurHp() != 2000 {
		t.Fatal("original weak-death release flow changed", releases, heals, dead)
	}
	// The original third cast sets float even on a noncritical weak-death recipient: 4×1.3 AP.
	closeCombat(t, out.Attacker.Inst.GetAnger(), 15.2)
	replay, err := s.ComputeFrozenBattle(t.Context(), input, out.Operates)
	if err != nil || !proto.Equal(out.Operates, replay.Operates) || !proto.Equal(out.Attacker, replay.Attacker) || !proto.Equal(out.Defender, replay.Defender) || out.RandomDraws != replay.RandomDraws {
		t.Fatal("weak-release full replay diverged", err)
	}
}

func TestP6OriginalWeakDeathCastSelectionAndHPMutationBoundary(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6WeakReleaseInput(t, s)
	e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	a, d, other := e.players[0].actors[0], e.players[1].actors[0], e.players[1].actors[1]
	if err := e.loadBuff(d, d, map[string]string{"Id": "weak-shield", "Factor": "shield", "Args1": "10", "TotalRound": "1"}, 4); err != nil {
		t.Fatal(err)
	}
	d.hp, d.state = 0, 3
	initial, err := e.selectTargets(a, 3, nil, nil)
	if err != nil || len(initial) != 1 || initial[0] != other {
		t.Fatal("new skill selected weak-dead target", err)
	}
	released, err := e.selectTargetsWithWeak(a, 10, d, nil, true)
	if err != nil || len(released) != 2 {
		t.Fatal("released area cast excluded weak death", err)
	}
	held, err := e.selectTargetsWithWeak(a, 3, d, nil, true)
	if err != nil || len(held) != 1 || held[0] != d {
		t.Fatal("FIRST did not hold original target", err)
	}
	same, err := e.selectTargetsWithWeak(a, 2, d, []*combatRoundActor{d, other}, true)
	if err != nil || len(same) != 1 || same[0] != d {
		t.Fatal("SAME_AS_TARGET expanded the prior area set", err)
	}
	e.damageHP(a, d, 100, 1)
	e.changeHP(a, d, -100, "buff_damage")
	e.changeHP(a, d, 100, "buff_heal")
	closeCombat(t, d.hp, 0)
	closeCombat(t, d.shield("shield"), 10)
	// The original invisible condition permits the same held reference during release.
	d.hp, d.state = 1, 0
	if err := e.loadBuff(d, d, map[string]string{"Id": "held-invisible", "Factor": "invisible", "TotalRound": "1"}, 4); err != nil {
		t.Fatal(err)
	}
	initial, err = e.selectTargets(a, 3, nil, nil)
	if err != nil || len(initial) != 1 || initial[0] != other {
		t.Fatal("new skill selected invisible enemy", err)
	}
	released, err = e.selectTargetsWithWeak(a, 10, d, nil, true)
	if err != nil || len(released) != 2 {
		t.Fatal("release excluded its own invisible reference", err)
	}
	d.state = 4
	released, err = e.selectTargetsWithWeak(a, 10, d, nil, true)
	if err != nil || len(released) != 1 || released[0] != other {
		t.Fatal("released area cast included final death", err)
	}
}
