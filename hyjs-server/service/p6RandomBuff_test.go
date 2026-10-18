package service

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p6RandomBuffInput(t *testing.T, s *GameService) *model.FrozenBattleInput {
	input := p6Input(t, s, p6Actor("hero_013", "random-caster", "013_1,013_2,013_3", 1, 80000), p6Actor("hero_047", "random-first", "047_1,047_2,047_3", 1, 80000), 18)
	input.Attacker.Inst.Anger = proto.Float64(100)
	input.Defender.Formation = append(input.Defender.Formation, p6Actor("hero_047", "random-second", "047_1", 2, 80000), p6Actor("hero_047", "random-third", "047_1", 3, 80000))
	return input
}

func TestP6OriginalRandomBuffCloneWeightsRangeAndChildImmunity(t *testing.T) {
	s := p6OriginalTables(t)
	makeEngine := func(seed int64) *combatRoundEngine {
		input := p6RandomBuffInput(t, s)
		e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input, rng: NewCombatRandom(seed), operate: &pb.BattleOperate{}}
		if err := e.initialize(); err != nil {
			t.Fatal(err)
		}
		return e
	}
	for _, tc := range []struct {
		seed  int64
		index float64
		child string
	}{{0, 10, "buff_122"}, {2432, 14, "buff_134"}} {
		e := makeEngine(tc.seed)
		a, d := e.players[0].actors[0], e.players[1].actors[0]
		if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_335"), 12); err != nil {
			t.Fatal(err)
		}
		if e.rng.Draws() != 1 || len(d.buffs) != 1 || d.buffs[0].row["Id"] != tc.child || d.buffs[0].rounds != int32(tableNumber(d.buffs[0].row, "TotalRound")) {
			t.Fatal("random Clone kept wrapper or changed child lifetime", tc.seed)
		}
		if e.events[0].Kind != "random_buff_choice" || e.events[0].Value != tc.index || e.events[0].Buff != tc.child {
			t.Fatal("original duplicate weighting or exclusive Count-1 bound changed", e.events[0])
		}
	}
	for _, params := range []string{"buff_111", "buff_111,buff_149"} {
		e := makeEngine(2432)
		a, d := e.players[0].actors[0], e.players[1].actors[0]
		if err := e.loadBuff(a, d, map[string]string{"Id": "original-range-probe", "BuffClass": "4", "Factor": "randomBuff", "Param": params}, 12); err != nil || e.rng.Draws() != 1 || len(d.buffs) != 1 || d.buffs[0].row["Id"] != "buff_111" {
			t.Fatal("one/two-entry original range picked final entry or skipped RNG", params, err)
		}
	}
	e := makeEngine(0)
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	d.immuneBuff[e.service.table("Buff", "Id", "buff_111")["Group"]] = true
	if err := e.applyTrigger(a, d, combatRoundTrigger{buff: "buff_335", timer: 12, selectStrategy: 2, chance: 1}, nil); err != nil || e.rng.Draws() != 2 || len(d.buffs) != 0 {
		t.Fatal("selected child immunity changed original chance/Clone draws", err, e.rng.Draws())
	}
}

func TestP6OriginalHero013FullRandomSkillsWholeBattleAndReplay(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6RandomBuffInput(t, s)
	out, err := s.ComputeFrozenBattle(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	seen, choices := map[string]bool{}, 0
	for _, event := range out.Events {
		if event.Kind == "release" && event.Actor == "0_random-caster" {
			seen[event.Skill] = true
		}
		if event.Kind == "random_buff_choice" {
			choices++
			if event.Value < 0 || event.Value >= 15 || event.Buff == "buff_335" {
				t.Fatal("original wrapper range or clone identity changed", event)
			}
		}
		if event.Kind == "buff_load" && event.Buff == "buff_335" {
			t.Fatal("random wrapper leaked into actor collection", event)
		}
	}
	if len(seen) != 3 || choices < 3 || out.Authoritative {
		t.Fatal("complete original Hero013 dependencies remained open", seen, choices)
	}
	replayed, err := s.ComputeFrozenBattle(t.Context(), input, out.Operates)
	if err != nil || !proto.Equal(out.Operates, replayed.Operates) || !proto.Equal(out.Attacker, replayed.Attacker) || !proto.Equal(out.Defender, replayed.Defender) || out.RandomDraws != replayed.RandomDraws {
		t.Fatal("original random child whole replay diverged", err)
	}
}

func TestP6RandomBuffRejectsUnknownChildCyclesAndFactoryMismatch(t *testing.T) {
	for _, variant := range []struct{ field, value string }{{"Param", ""}, {"Param", "buff_111,unknown-child"}, {"Param", "buff_335"}, {"BuffClass", "0"}} {
		s := p6OriginalTables(t)
		s.table("Buff", "Id", "buff_335")[variant.field] = variant.value
		input := p6RandomBuffInput(t, s)
		if _, err := s.ComputeFrozenBattle(t.Context(), input, nil); err == nil {
			t.Fatal("unknown random child/cycle/factory silently accepted", variant)
		}
	}
}
