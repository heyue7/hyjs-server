package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"reflect"
	"testing"
)

func p6SameDamageEngine(t *testing.T) *combatRoundEngine {
	e := p6PropertyTransferEngine(t)
	for i := int32(2); i <= 3; i++ {
		e.input.Defender.Formation = append(e.input.Defender.Formation, p6Actor("hero_047", string(rune('a'+i)), "047_1,047_2,047_3", i, 80000))
	}
	e = &combatRoundEngine{service: e.service, input: e.input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	return e
}
func TestP6OriginalSameDamageCachesTargetIgnoresVisibilityDefenseConsumesTypedShield(t *testing.T) {
	for _, id := range []string{"buff_311", "buff_743"} {
		e := p6SameDamageEngine(t)
		a, d := e.players[0].actors[0], e.players[1].actors[0]
		if err := e.loadBuff(a, a, e.service.table("Buff", "Id", id), 5); err != nil {
			t.Fatal(err)
		}
		for _, x := range e.players[1].actors[1:] {
			x.base["pdef"] = 100000
			x.base["damageReduce"] = 100
			for _, f := range []string{"invincible", "invisible", "shield", "pshield", "mshield"} {
				if err := e.loadBuff(x, x, map[string]string{"Id": f, "Factor": f, "Args1": "1000", "TotalRound": "4"}, 4); err != nil {
					t.Fatal(err)
				}
			}
		}
		skill := a.skills[0]
		skill.target = d
		if err := e.originalSameDamage(skill, d, 100, 1); err != nil {
			t.Fatal(err)
		}
		selected := skill.sameDamTargets[0]
		x := tableNumber(e.service.table("Buff", "Id", id), "Args1")
		closeCombat(t, selected.hp, 80000)
		if e.rng.Draws() != 1 || selected == d {
			t.Fatal("native random other target selection", e.rng.Draws())
		}
		closeCombat(t, selected.shield("pshield"), 1000-x)
		closeCombat(t, selected.shield("shield"), 1000)
		closeCombat(t, selected.shield("mshield"), 1000)
		if err := e.originalSameDamage(skill, e.players[1].actors[1], 200, 1); err != nil {
			t.Fatal(err)
		}
		closeCombat(t, selected.hp, 80000)
		closeCombat(t, selected.shield("pshield"), 1000-3*x)
		if e.rng.Draws() != 1 || len(skill.sameDamTargets) != 1 {
			t.Fatal("native skill target cache redrew")
		}
		selected.state = 4
		selected.hp = 0
		if err := e.originalSameDamage(skill, d, 100, 1); err != nil || e.rng.Draws() != 1 || skill.sameDamTargets[0] != selected {
			t.Fatal("native dead cached target was rerolled", err)
		}
	}
}
func TestP6OriginalSameDamageFallbackMinimumAndSuppression(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	row := e.service.table("Buff", "Id", "buff_311")
	row["CanBeCleared"] = "1"
	if err := e.loadBuff(a, a, row, 5); err != nil {
		t.Fatal(err)
	}
	skill := a.skills[0]
	if err := e.originalSameDamage(skill, d, .5, 1); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.hp, 79999)
	if e.rng.Draws() != 0 || len(skill.sameDamTargets) != 1 || skill.sameDamTargets[0] != d {
		t.Fatal("native single target fallback consumed random")
	}
	suppressed, err := e.suppressBuffsOnce(a)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.originalSameDamage(skill, d, 100, 1); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.hp, 79999)
	if err = e.restoreBuffsOnce(a, suppressed); err != nil {
		t.Fatal(err)
	}
	if err = e.originalSameDamage(skill, d, 100, 1); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.hp, 79924)
	e.unloadBuff(a, a.buffs[0])
	closeCombat(t, a.attribute("sameDamForRandom"), 0)
}
func TestP6OriginalSameDamageFullReleaseLastResetAndNoDrawWithoutFactor(t *testing.T) {
	e := p6SameDamageEngine(t)
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	skill := a.skills[0]
	if err := e.originalSameDamage(skill, d, 100, 1); err != nil || e.rng.Draws() != 0 {
		t.Fatal("missing native factor consumes RNG", err)
	}
	if err := e.loadBuff(a, a, e.service.table("Buff", "Id", "buff_311"), 5); err != nil {
		t.Fatal(err)
	}
	e.beginSkill(skill)
	for i, release := range skill.releases {
		if !release.isCast {
			continue
		}
		skill.releaseIndex = i
		if err := e.dynamicRelease(skill, release.cast); err != nil {
			t.Fatal(err)
		}
		if i == len(skill.releases)-1 {
			if len(skill.sameDamTargets) != 0 {
				t.Fatal("LAST kept random target")
			}
		} else if len(skill.sameDamTargets) != 1 {
			t.Fatal("intermediate release lost target cache")
		}
	}
	// Exact native state3 branch clears even an intermediate release.
	skill.sameDamTargets = []*combatRoundActor{d}
	a.state = 3
	skill.releaseIndex = 0
	if err := e.dynamicRelease(skill, skill.casts[0]); err != nil {
		t.Fatal(err)
	}
	if len(skill.sameDamTargets) != 0 {
		t.Fatal("weak source retained cache")
	}
	e.service.table("Buff", "Id", "buff_311")["BuffClass"] = "2"
	if err := e.validateDynamicTrigger(combatRoundTrigger{buff: "buff_311", timer: 5}); err == nil {
		t.Fatal("wrong same-damage factory accepted")
	}
}
func TestP6OriginalSameDamageExtraDeathNoKillerAndLastAttackImmunity(t *testing.T) {
	e := p6SameDamageEngine(t)
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	a.talents = []combatRoundTrigger{{buff: "buff_196", timer: 18, chance: 1}}
	if err := e.loadBuff(a, a, e.service.table("Buff", "Id", "buff_311"), 5); err != nil {
		t.Fatal(err)
	}
	skill := a.skills[0]
	if err := e.originalSameDamage(skill, d, 100, 1); err != nil {
		t.Fatal(err)
	}
	extra := skill.sameDamTargets[0]
	extra.hp = 1
	if err := e.originalSameDamage(skill, d, 100, 1); err != nil {
		t.Fatal(err)
	}
	if extra.state != 4 || a.hasState("dieAfterLastAttack") {
		t.Fatal("random victim incorrectly triggered primary Kill talent")
	}
	// No alternative target: random damage can still kill the primary skill reference.
	skill.target = d
	skill.sameDamTargets = []*combatRoundActor{d}
	d.hp = 1
	if err := e.originalSameDamage(skill, d, 100, 1); err != nil {
		t.Fatal(err)
	}
	if d.state != 4 || !a.hasState("dieAfterLastAttack") {
		t.Fatal("fallback primary lost original Kill reference")
	}
	protected := e.players[1].actors[1]
	protected.state = 0
	protected.hp = .01
	if err := e.loadBuff(protected, protected, e.service.table("Buff", "Id", "buff_196"), 24); err != nil {
		t.Fatal(err)
	}
	skill.sameDamTargets = []*combatRoundActor{protected}
	if err := e.originalSameDamage(skill, d, 100, 1); err != nil {
		t.Fatal(err)
	}
	if protected.hp != .01 || protected.state != 0 {
		t.Fatal("extra damage bypassed native last-attack AddHp guard")
	}
}
func p6SameDamageWholeInputs(t *testing.T, s *GameService) []*model.FrozenBattleInput {
	var out []*model.FrozenBattleInput
	for _, x := range []struct{ hero, skills, talent string }{{"hero_004", "004_1,004_2,004_3", "tal_247"}, {"hero_068", "068_1,068_2,068_3", "tal_405"}} {
		for _, count := range []int{1, 3} {
			a := p6Actor(x.hero, "same-source", x.skills, 1, 80000)
			a.TalentList = []string{x.talent}
			f := p6Input(t, s, a, p6Actor("hero_047", "same-enemy", "047_1,047_2,047_3", 1, 80000), 18)
			for i := 2; i <= count; i++ {
				f.Defender.Formation = append(f.Defender.Formation, p6Actor("hero_047", string(rune('a'+i)), "047_1,047_2,047_3", int32(i), 80000))
			}
			f.Attacker.Inst.Anger = proto.Float64(100)
			f.Defender.Inst.Anger = proto.Float64(100)
			frozen, err := s.FreezeBattleInput(*f)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, frozen)
		}
	}
	return out
}
func TestP6OriginalSameDamageHero004And068SingleAndMultiFullSkillsReplay(t *testing.T) {
	s := p6OriginalTables(t)
	for i, f := range p6SameDamageWholeInputs(t, s) {
		out, err := s.ComputeFrozenBattle(t.Context(), f, nil)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		hits, loads := 0, 0
		for _, v := range out.Events {
			if v.Kind == "release" && v.Actor == "0_same-source" {
				seen[v.Skill] = true
			}
			if v.Kind == "same_damage" {
				hits++
			}
			if v.Kind == "buff_load" && (v.Buff == "buff_311" || v.Buff == "buff_743") {
				loads++
			}
		}
		if len(seen) != 3 || hits == 0 || loads == 0 || out.Authoritative {
			t.Fatal("actual content full 3skill random-damage sequence", i, seen, hits, loads)
		}
		replay, err := s.ComputeFrozenBattle(t.Context(), f, out.Operates)
		if err != nil || !proto.Equal(out.Operates, replay.Operates) || !proto.Equal(out.Attacker, replay.Attacker) || !proto.Equal(out.Defender, replay.Defender) || out.RandomDraws != replay.RandomDraws || !reflect.DeepEqual(out.Events, replay.Events) {
			t.Fatal("native whole same-damage replay", err)
		}
	}
}
