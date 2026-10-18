package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"reflect"
	"testing"
)

func p6PropertyTransferEngine(t *testing.T) *combatRoundEngine {
	s := p6OriginalTables(t)
	f := p6Input(t, s, p6Actor("hero_047", "transfer-source", "047_1,047_2,047_3", 1, 80000), p6Actor("hero_047", "transfer-target", "047_1,047_2,047_3", 1, 80000), 18)
	e := &combatRoundEngine{service: &GameService{Tables: f.Tables}, input: f, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	return e
}
func TestP6OriginalPropertyTransfersAllEightRowsLifecycle(t *testing.T) {
	for _, tc := range []struct {
		id, key string
		want    float64
	}{
		{"buff_128", "pdef", -50}, {"buff_463", "mdef", -18.75},
		{"buff_144", "cri", 21}, {"buff_190", "cri", 7},
		{"buff_186", "cri", 15}, {"buff_189", "att", 14},
		{"buff_262", "criFactor", 50}, {"buff_458", "mdef", 200},
	} {
		t.Run(tc.id, func(t *testing.T) {
			e := p6PropertyTransferEngine(t)
			source, target := e.players[0].actors[0], e.players[1].actors[0]
			source.base["hit"] = 125
			source.base["mdef"] = 250
			target.base["cri"] = 10
			e.players[0].anger = 5
			e.players[1].anger = 79.9
			e.players[0].combo = 2
			e.players[1].combo = 29
			row := e.service.table("Buff", "Id", tc.id)
			if err := e.validateDynamicTrigger(combatRoundTrigger{buff: tc.id, timer: 4}); err != nil {
				t.Fatal(err)
			}
			before := target.attribute(tc.key)
			if err := e.loadBuff(source, target, row, 4); err != nil {
				t.Fatal(err)
			}
			want := before + tc.want
			if tc.key == "att" || tc.id == "buff_128" || tc.id == "buff_463" {
				want = before * (1 + tc.want*.01)
			}
			closeCombat(t, target.attribute(tc.key), want)
			// Original Update0 has no recomputation when sources/anger/combo change.
			source.base["hit"] = 150
			source.base["mdef"] = 400
			target.base["cri"] = 20
			e.players[1].anger = 100
			e.players[1].combo = 40
			e.time = 1
			e.updateBuffs(target)
			if tc.key == "cri" {
				want += 10
			}
			closeCombat(t, target.attribute(tc.key), want)
			oldValue := target.buffs[0].value
			row["CanBeCleared"] = "1" // Exercise the native general reactivation path; original row unchanged outside frozen input.
			suppressed, err := e.suppressBuffsOnce(target)
			if err != nil {
				t.Fatal(err)
			}
			closeCombat(t, target.attribute(tc.key), target.base[tc.key])
			if err = e.restoreBuffsOnce(target, suppressed); err != nil {
				t.Fatal(err)
			}
			if target.buffs[0].value == oldValue {
				t.Fatal("reactivation failed to reread changed input", oldValue)
			}
			b := target.buffs[0]
			e.unloadBuff(target, b)
			closeCombat(t, target.attribute(tc.key), target.base[tc.key])
			if e.rng.Draws() != 0 {
				t.Fatal("transfer consumed RNG")
			}
		})
	}
}
func TestP6OriginalPropertyTransferThresholdCapAndSelfSource(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	a := e.players[0].actors[0]
	a.base["cri"] = 10
	a.base["mdef"] = 100
	row := e.service.table("Buff", "Id", "buff_144")
	for _, tc := range []struct{ input, want float64 }{{0, 0}, {9.999, 0}, {10, 3}, {19.999, 3}, {20, 6}, {100, 30}} {
		e.players[0].anger = tc.input
		closeCombat(t, e.propertyTransferValue(a, a, row), tc.want)
	}
	combo := e.service.table("Buff", "Id", "buff_186")
	for _, tc := range []struct {
		input int32
		want  float64
	}{{0, 0}, {4, 0}, {5, 3}, {80, 48}, {85, 50}, {100, 50}} {
		e.players[0].combo = tc.input
		closeCombat(t, e.propertyTransferValue(a, a, combo), tc.want)
	}
	// A self-source magic-defense snapshot must remove its own old increment before re-Effect.
	shield := e.service.table("Buff", "Id", "buff_458")
	shield["CanBeCleared"] = "1"
	if err := e.loadBuff(a, a, shield, 5); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, a.attribute("mdef"), 180)
	for i := 0; i < 3; i++ {
		suppressed, err := e.suppressBuffsOnce(a)
		if err != nil {
			t.Fatal(err)
		}
		if err = e.restoreBuffsOnce(a, suppressed); err != nil {
			t.Fatal(err)
		}
		closeCombat(t, a.attribute("mdef"), 180)
	}
	e.unloadBuff(a, a.buffs[0])
	closeCombat(t, a.attribute("mdef"), 100)
	// No implicit lower bound: original negative Args2 and nonzero upper Args3 stay signed.
	probe := map[string]string{"Factor": "angerTocri", "Args1": "10", "Args2": "-3", "Args3": "5"}
	e.players[0].anger = 20
	closeCombat(t, e.propertyTransferValue(a, a, probe), -6)
}
func p6OriginalTransferWholeInputs(t *testing.T, s *GameService) []*model.FrozenBattleInput {
	var result []*model.FrozenBattleInput
	for _, tc := range []struct{ id, skills, talent string }{
		{"hero_007", "007_4,007_5,007_6", ""},
		{"hero_039", "039_1,039_2,039_3", "tal_357"},
		{"hero_053", "053_1,053_2,053_3", "tal_364"},
		{"hero_009", "009_1,009_2,009_3", "tal_258"},
	} {
		a := p6Actor(tc.id, "transfer-"+tc.id, tc.skills, 1, 80000)
		if tc.talent != "" {
			a.TalentList = []string{tc.talent}
		}
		f := p6Input(t, s, a, p6Actor("hero_047", "transfer-enemy", "047_1,047_2,047_3", 1, 80000), 18)
		f.Attacker.Inst.Anger = proto.Float64(100)
		result = append(result, f)
	}
	return result
}
func TestP6OriginalPropertyTransferFourCompleteHeroesReplay(t *testing.T) {
	s := p6OriginalTables(t)
	want := []string{"buff_128", "buff_262", "buff_463", "buff_458"}
	for i, f := range p6OriginalTransferWholeInputs(t, s) {
		t.Run(f.Attacker.Formation[0].GetId(), func(t *testing.T) {
			before, _ := json.Marshal(f)
			out, err := s.ComputeFrozenBattle(t.Context(), f, nil)
			if err != nil {
				t.Fatal(err)
			}
			seen := map[string]bool{}
			loads := 0
			for _, v := range out.Events {
				if v.Kind == "release" && v.Actor == "0_"+f.Attacker.Formation[0].GetGuid() {
					seen[v.Skill] = true
				}
				if v.Kind == "buff_load" && v.Buff == want[i] {
					loads++
				}
			}
			if len(seen) != 3 || loads == 0 || out.Authoritative {
				t.Fatal("original complete skill/transfer missing", seen, loads)
			}
			replay, err := s.ComputeFrozenBattle(t.Context(), f, out.Operates)
			if err != nil || !proto.Equal(out.Operates, replay.Operates) || !proto.Equal(out.Attacker, replay.Attacker) || !proto.Equal(out.Defender, replay.Defender) || out.RandomDraws != replay.RandomDraws || !reflect.DeepEqual(out.Events, replay.Events) {
				t.Fatal("transfer event replay diverged", err)
			}
			after, _ := json.Marshal(f)
			if string(before) != string(after) {
				t.Fatal("frozen original input mutated")
			}
		})
	}
}
func TestP6PropertyTransferFactoryAndDivisorReject(t *testing.T) {
	for _, id := range []string{"buff_128", "buff_144", "buff_186", "buff_189", "buff_190", "buff_262", "buff_458", "buff_463"} {
		e := p6PropertyTransferEngine(t)
		row := e.service.table("Buff", "Id", id)
		row["BuffClass"] = "0"
		if err := e.validateDynamicTrigger(combatRoundTrigger{buff: id, timer: 4}); err == nil {
			t.Fatal("wrong factory accepted", id)
		}
		row["BuffClass"] = "1"
		row["Args1"] = "0"
		err := e.validateDynamicTrigger(combatRoundTrigger{buff: id, timer: 4})
		if p6PropertyTransferDivides(row["Factor"]) && err == nil {
			t.Fatal("zero divisor accepted", id)
		}
		if !p6PropertyTransferDivides(row["Factor"]) && err != nil {
			t.Fatal("zero multiplier incorrectly refused", id, err)
		}
	}
}

func TestP6OriginalPropertyTransferCastRoundDeathAndVariantGuards(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	a, target := e.players[0].actors[0], e.players[1].actors[0]
	e.players[0].anger = 100
	if err := e.loadBuff(a, a, e.service.table("Buff", "Id", "buff_144"), 4); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, a.attribute("cri"), a.base["cri"]+30)
	e.clearOnce(a, true)
	if len(a.buffs) != 1 {
		t.Fatal("before-cast transfer expired at hit")
	}
	e.clearOnce(a, false)
	if len(a.buffs) != 0 {
		t.Fatal("one-cast transfer survived end")
	}
	if err := e.loadBuff(a, target, e.service.table("Buff", "Id", "buff_458"), 5); err != nil {
		t.Fatal(err)
	}
	e.round = 1
	e.camp = 0
	if err := e.endRound(); err != nil {
		t.Fatal(err)
	}
	if len(target.buffs) != 1 {
		t.Fatal("two-round transfer expired early")
	}
	e.round = 2
	e.camp = 1
	if err := e.endRound(); err != nil {
		t.Fatal(err)
	}
	if len(target.buffs) != 0 {
		t.Fatal("two-round transfer failed to expire")
	}
	if err := e.loadBuff(a, target, e.service.table("Buff", "Id", "buff_458"), 5); err != nil {
		t.Fatal(err)
	}
	e.damageHP(a, target, target.hp+1, 1)
	if target.state != 4 || len(target.buffs) != 0 || target.attribute("mdef") != target.base["mdef"] {
		t.Fatal("death retained transfer")
	}
	s := p6OriginalTables(t)
	for _, skills := range []string{"007_4,007_2,007_6", "004_11,004_12,004_13", "047_1,007_5,007_6"} {
		f := p6Input(t, s, p6Actor("hero_007", "variant", skills, 1, 80000), p6Actor("hero_047", "enemy", "047_1,047_2,047_3", 1, 80000), 4)
		if _, err := s.ComputeFrozenBattle(t.Context(), f, nil); err == nil {
			t.Fatal("unmapped or mixed native sequence accepted", skills)
		}
	}
}
