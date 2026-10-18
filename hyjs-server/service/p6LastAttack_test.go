package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	"reflect"
	"testing"
)

func TestP6OriginalLastAttackRepeatedLethalShieldsAndOwnRoundEnd(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	d.talents = []combatRoundTrigger{{buff: "buff_196", timer: 24, chance: 1}}
	d.hp = 20
	if err := e.loadBuff(d, d, e.service.table("Buff", "Id", "buff_157"), 4); err != nil {
		t.Fatal(err)
	}
	shield := d.shield("shield")
	e.damageHP(a, d, shield+21, 1)
	if e.failure != nil || d.hp != .01 || d.state != 0 || !d.hasState("dieAfterLastAttack") || d.shield("shield") != 0 {
		t.Fatal("original lethal protection/shield order", e.failure, d.hp, d.state)
	}
	e.changeHP(a, d, -1, "damage")
	if d.hp != .01 || d.state != 0 || len(d.buffs) != 1 || e.rng.Draws() != 1 {
		t.Fatal("native AddHp must stop all subsequent changes during last attack", d.hp, len(d.buffs), e.rng.Draws())
	}
	if err := e.loadBuff(d, d, e.service.table("Buff", "Id", "buff_157"), 4); err != nil {
		t.Fatal(err)
	}
	shield = d.shield("shield")
	e.damageHP(a, d, 10000, 1)
	e.changeHP(a, d, 10000, "buff_heal")
	if d.hp != .01 || d.shield("shield") != shield || e.rng.Draws() != 1 {
		t.Fatal("protected actor mutated HP/shield or retriggered WillDie")
	}
	e.camp = 0
	if err := e.endRound(); err != nil {
		t.Fatal(err)
	}
	if d.state != 0 || d.hp != .01 || !d.hasState("dieAfterLastAttack") {
		t.Fatal("enemy round end killed last attacker")
	}
	d.current = d.skills[0]
	d.current.state = 2
	d.current.first = true
	d.waiting = []*combatRoundSkill{d.skills[1]}
	e.pendingUltimate = d.current
	e.ultimateRunning = true
	e.camp = 1
	if err := e.endRound(); err != nil {
		t.Fatal(err)
	}
	if d.hp != 0 || d.state != 4 || len(d.buffs) != 0 || d.current != nil || len(d.waiting) != 0 || e.pendingUltimate != nil || e.ultimateRunning {
		t.Fatal("direct own-round DoDead did not clear pending state", d.hp, d.state)
	}
	for _, skill := range d.skills {
		if skill.state != 4 {
			t.Fatal("last attack left skill enabled")
		}
	}
}
func TestP6OriginalLastAttackForcedDeathReviveAndNoKillerTalent(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	// Direct lifecycle probe. This cross-hero talent combination has no content claim.
	d.talents = []combatRoundTrigger{{buff: "buff_196", timer: 24, chance: 1}, {buff: "buff_282", timer: 16, chance: 1}}
	// Use the exact revival row referenced by native talent282.
	d.talents[1].buff = e.service.table("Talent", "Id", "tal_282")["Buff1"]
	a.talents = []combatRoundTrigger{{buff: "buff_196", timer: 18, chance: 1}}
	d.hp = 1
	e.damageHP(a, d, 10, 1)
	e.camp = 1
	if err := e.endRound(); err != nil {
		t.Fatal(err)
	}
	if !d.revived || d.state != 0 || d.hp != 8000 || d.hasState("dieAfterLastAttack") || a.hasState("dieAfterLastAttack") {
		t.Fatal("forced death revival/killer lifecycle", d.hp, d.state, d.revived)
	}
	for _, skill := range d.skills {
		if skill.state != 4 {
			t.Fatal("revival reenabled old skill")
		}
	}
}
func p6LastAttackWholeInputs(t *testing.T, s *GameService) []*model.FrozenBattleInput {
	var out []*model.FrozenBattleInput
	for _, selfFirst := range []bool{true, false} {
		a := p6Actor("hero_004", "last-attacker", "004_1,004_2,004_3", 1, 80000)
		a.Inst.CurHp = proto.Float64(40)
		a.TalentList = []string{"tal_245"}
		b := p6Actor("hero_047", "last-enemy", "047_1,047_2,047_3", 1, 80000)
		var f *model.FrozenBattleInput
		if selfFirst {
			f = p6Input(t, s, a, b, 8)
		} else {
			f = p6Input(t, s, b, a, 8)
		}
		f.Attacker.Inst.Anger = proto.Float64(100)
		f.Defender.Inst.Anger = proto.Float64(100)
		out = append(out, f)
	}
	return out
}
func TestP6OriginalLastAttackHero004FullSkillsAndReplay(t *testing.T) {
	s := p6OriginalTables(t)
	for i, f := range p6LastAttackWholeInputs(t, s) {
		out, err := s.ComputeFrozenBattle(t.Context(), f, nil)
		if err != nil {
			t.Fatal(err)
		}
		protection, endRound := 0, int32(0)
		seen := map[string]bool{}
		for _, v := range out.Events {
			if v.Kind == "last_attack_protected" {
				protection++
			}
			if v.Kind == "last_attack_round_death" {
				endRound = v.Round
			}
			if v.Kind == "release" && (v.Skill == "004_1" || v.Skill == "004_2" || v.Skill == "004_3") {
				seen[v.Skill] = true
			}
		}
		if protection == 0 || endRound == 0 || len(seen) != 3 || out.Authoritative {
			t.Fatal("actual hero004 full last attack sequence", i, protection, endRound, seen)
		}
		replay, err := s.ComputeFrozenBattle(t.Context(), f, out.Operates)
		if err != nil || !proto.Equal(out.Operates, replay.Operates) || !proto.Equal(out.Attacker, replay.Attacker) || !proto.Equal(out.Defender, replay.Defender) || out.RandomDraws != replay.RandomDraws || !reflect.DeepEqual(out.Events, replay.Events) {
			t.Fatal("last attack full replay", err)
		}
	}
}
func TestP6OriginalLastAttackRejectsWrongFactoryAndTrigger(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	for _, v := range []combatRoundTrigger{{buff: "buff_196", timer: 5}, {buff: "buff_196", timer: 24, selectStrategy: 2}} {
		if err := e.validateDynamicTrigger(v); err == nil {
			t.Fatal("unsupported last attack trigger accepted")
		}
	}
	e.service.table("Buff", "Id", "buff_196")["BuffClass"] = "0"
	if err := e.validateDynamicTrigger(combatRoundTrigger{buff: "buff_196", timer: 24}); err == nil {
		t.Fatal("wrong native state factory accepted")
	}
}
