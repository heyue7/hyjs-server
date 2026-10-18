package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	"reflect"
	"testing"
)

func TestP6OriginalAllDebuffClearabilityAndPersistentMarker(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	for _, row := range []map[string]string{
		{"Id": "clearable-negative1", "Factor": "peratt", "Args1": "-10", "IsDebuff": "1", "CanBeCleared": "1", "TotalRound": "4"},
		{"Id": "clearable-negative2", "Factor": "perdef", "Args1": "-10", "IsDebuff": "1", "CanBeCleared": "1", "TotalRound": "4"},
		{"Id": "retained-negative", "Factor": "peratt", "Args1": "-5", "IsDebuff": "1", "TotalRound": "4"},
		{"Id": "retained-positive", "Factor": "att", "Args1": "20", "CanBeCleared": "1", "TotalRound": "4"},
	} {
		if err := e.loadBuff(a, d, row, 7); err != nil {
			t.Fatal(err)
		}
	}
	clear := e.service.table("Buff", "Id", "buff_578")
	if err := e.loadBuff(d, d, clear, 7); err != nil {
		t.Fatal(err)
	}
	if len(d.buffs) != 3 {
		t.Fatal("clearAllDebuff failed original clearable-only snapshot", len(d.buffs))
	}
	for _, b := range d.buffs {
		if b.row["Id"] == "clearable-negative1" || b.row["Id"] == "clearable-negative2" {
			t.Fatal("eligible debuff survived")
		}
	}
	marker := d.buffs[2]
	if marker.row["Id"] != "buff_578" || marker.rounds != 100 {
		t.Fatal("native dispel marker identity/lifetime missing")
	}
	if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_111"), 7); err != nil {
		t.Fatal(err)
	}
	if err := e.loadBuff(d, d, clear, 7); err != nil {
		t.Fatal(err)
	}
	if len(d.buffs) != 4 || d.buffs[2] != marker {
		t.Fatal("equal cover reexecuted native clearAllDebuff")
	}
	// Actual buff580 cannot remove actual578: its CanBeCleared is false in the original row.
	if err := e.loadBuff(d, d, e.service.table("Buff", "Id", "buff_580"), 2); err != nil {
		t.Fatal(err)
	}
	if len(d.buffs) != 5 || d.buffs[2] != marker {
		t.Fatal("exact-ID dispel bypassed original nonclearable flag")
	}
	if e.rng.Draws() != 0 {
		t.Fatal("all/specific dispel unexpectedly consumed RNG")
	}
}
func TestP6OriginalSpecificDispelExactIdsMutatingIterationAndMissing(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	a := e.players[0].actors[0]
	for i := 0; i < 4; i++ {
		if err := e.loadBuff(a, a, map[string]string{"Id": "same-original-id", "Group": "shared-group", "Factor": "att", "Args1": "10", "CanBeCleared": "1", "TotalRound": "5"}, 4); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.loadBuff(a, a, map[string]string{"Id": "other-in-group", "Group": "shared-group", "Factor": "att", "Args1": "10", "CanBeCleared": "1", "TotalRound": "5"}, 4); err != nil {
		t.Fatal(err)
	}
	row := map[string]string{"Factor": "clearSomeBuffs", "Param": ",missing,same-original-id,"}
	e.originalDispelEffect(a, row)
	if len(a.buffs) != 3 {
		t.Fatal("native mutating collection must skip adjacent identity", len(a.buffs))
	}
	closeCombat(t, a.attribute("att"), a.base["att"]+30)
	row["Param"] = "same-original-id,same-original-id"
	e.originalDispelEffect(a, row)
	if len(a.buffs) != 1 || a.buffs[0].row["Id"] != "other-in-group" {
		t.Fatal("repeated exact-ID native passes or group boundary failed")
	}
	if e.rng.Draws() != 0 {
		t.Fatal("specific-ID native dispel consumed RNG")
	}
}
func TestP6OriginalDispelReeffectAndHPConditionalBranches(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	a := e.players[0].actors[0]
	a.talents = nil
	tal := e.service.table("Talent", "Id", "tal_382")
	for _, tc := range []struct {
		hp   float64
		slot int
		want bool
	}{{39999, 1, true}, {40000, 1, true}, {40001, 1, false}, {39999, 2, false}, {40000, 2, false}, {40001, 2, true}} {
		a.hp = tc.hp
		key := "Buff1"
		if tc.slot == 2 {
			key = "Buff2"
		}
		trigger := combatRoundTrigger{condition: 2, args: tal[key+"ConditionArgs"]}
		if e.conditionTrue(trigger, a, nil) != tc.want {
			t.Fatal("original50percent branch changed", tc)
		}
	}
	// Re-Effect of a clearable dispel probe executes its effect again, without replacing lifetime.
	row := map[string]string{}
	for k, v := range e.service.table("Buff", "Id", "buff_578") {
		row[k] = v
	}
	row["CanBeCleared"] = "1"
	if err := e.loadBuff(a, a, row, 7); err != nil {
		t.Fatal(err)
	}
	marker := a.buffs[0]
	suppressed, err := e.suppressBuffsOnce(a)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.loadBuff(a, a, e.service.table("Buff", "Id", "buff_111"), 7); err != nil {
		t.Fatal(err)
	}
	if err = e.restoreBuffsOnce(a, suppressed); err != nil {
		t.Fatal(err)
	}
	if len(a.buffs) != 1 || a.buffs[0] != marker || marker.rounds != 100 {
		t.Fatal("native dispel reEffect identity/lifetime/cleanup failed")
	}
}
func p6DispelWholeInputs(t *testing.T, s *GameService) []*model.FrozenBattleInput {
	var result []*model.FrozenBattleInput
	for _, hp := range []float64{32000, 64000} {
		a := p6Actor("hero_062", "dispel-caster", "062_1,062_2,062_3", 1, 80000)
		a.Inst.CurHp = proto.Float64(hp)
		a.TalentList = []string{"tal_382"}
		d := p6Actor("hero_008", "dispel-poison-source", "008_1,008_2,008_3", 1, 80000)
		d.TalentList = []string{"tal_291"}
		f := p6Input(t, s, a, d, 18)
		f.Attacker.Inst.Anger = proto.Float64(100)
		result = append(result, f)
	}
	return result
}
func TestP6OriginalDispelFullSkillsBothHPBranchesReplay(t *testing.T) {
	s := p6OriginalTables(t)
	for i, f := range p6DispelWholeInputs(t, s) {
		out, err := s.ComputeFrozenBattle(t.Context(), f, nil)
		if err != nil {
			t.Fatal(err)
		}
		expected := "buff_578"
		if i == 1 {
			expected = "buff_580"
		}
		loads := 0
		seen := map[string]bool{}
		unloadedPoison := false
		for _, v := range out.Events {
			if v.Kind == "buff_load" && v.Buff == expected {
				loads++
			}
			if v.Kind == "release" && v.Actor == "0_"+f.Attacker.Formation[0].GetGuid() {
				seen[v.Skill] = true
			}
			if v.Kind == "buff_unload" && v.Buff == "buff_111" {
				unloadedPoison = true
			}
		}
		if loads == 0 || len(seen) != 3 || out.Authoritative {
			t.Fatal("complete original dispel HP branch missing", i, loads, seen)
		}
		if i == 0 && !unloadedPoison {
			t.Fatal("low-HP full sequence never cleared original poison")
		}
		replay, err := s.ComputeFrozenBattle(t.Context(), f, out.Operates)
		if err != nil || !proto.Equal(out.Operates, replay.Operates) || !proto.Equal(out.Attacker, replay.Attacker) || !proto.Equal(out.Defender, replay.Defender) || out.RandomDraws != replay.RandomDraws || !reflect.DeepEqual(out.Events, replay.Events) {
			t.Fatal("original dispel replay diverged", err)
		}
	}
	for _, id := range []string{"buff_578", "buff_580"} {
		e := p6PropertyTransferEngine(t)
		e.service.table("Buff", "Id", id)["BuffClass"] = "0"
		if err := e.validateDynamicTrigger(combatRoundTrigger{buff: id, timer: 7}); err == nil {
			t.Fatal("wrong dispel factory accepted", id)
		}
	}
}
