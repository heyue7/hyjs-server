package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	"reflect"
	"testing"
)

func TestP6OriginalShareHighestCurrentHPTiesSnapshotAndAnyUnloadClearsAll(t *testing.T) {
	e := p6SameDamageEngine(t)
	source, target := e.players[0].actors[0], e.players[1].actors[0]
	m1, m2 := e.players[1].actors[1], e.players[1].actors[2]
	m1.base["maxHp"] = 90000
	m1.hp = 2000
	m2.hp = 2000
	for _, id := range []string{"buff_206", "buff_523"} {
		if err := e.loadBuff(source, target, e.service.table("Buff", "Id", id), 7); err != nil {
			t.Fatal(err)
		}
	}
	if target.shareFactor != .3 || len(target.shareActors) != 2 || target.shareActors[0] != m1 || target.shareActors[1] != m2 || e.rng.Draws() != 0 {
		t.Fatal("native current HP/tie overwrite snapshot")
	}
	m2.hp = 1
	if len(target.shareActors) != 2 {
		t.Fatal("native share snapshot recomputed on HP change")
	}
	e.unloadBuff(target, target.buffs[0])
	if target.shareFactor != 0 || len(target.shareActors) != 0 || len(target.buffs) != 1 {
		t.Fatal("one native UnEffect must clear whole attribute despite surviving identity")
	}
	e.originalShareEffect(target, e.service.table("Buff", "Id", "buff_206"))
	if len(target.shareActors) != 1 || target.shareActors[0] != m1 {
		t.Fatal("native Effect failed to reread highest current HP")
	}
	e.service.table("Buff", "Id", "buff_523")["CanBeCleared"] = "1"
	e.originalShareEffect(target, e.service.table("Buff", "Id", "buff_523"))
	suppressed, err := e.suppressBuffsOnce(target)
	if err != nil {
		t.Fatal(err)
	}
	if target.shareFactor != 0 || len(target.shareActors) != 0 {
		t.Fatal("native suppression did not clear share fields")
	}
	m2.hp = 3000
	if err = e.restoreBuffsOnce(target, suppressed); err != nil {
		t.Fatal(err)
	}
	if len(target.shareActors) != 1 || target.shareActors[0] != m2 || target.shareFactor != .3 {
		t.Fatal("restore did not reselect current high HP")
	}
}
func TestP6OriginalShareRecipientIndependentFormulaIgnoresCritArmorImmunityConsumesTypedShield(t *testing.T) {
	e := p6SameDamageEngine(t)
	source, target := e.players[0].actors[0], e.players[1].actors[0]
	m1, m2 := e.players[1].actors[1], e.players[1].actors[2]
	m1.base["pdef"], m1.base["damageReduce"] = 100, 10
	m2.base["pdef"], m2.base["damageReduce"] = 200, 20
	for _, actor := range []*combatRoundActor{source, m1, m2} {
		for _, f := range []string{"shield", "pshield", "mshield", "invincible"} {
			if err := e.loadBuff(actor, actor, map[string]string{"Id": f, "Factor": f, "Args1": "9999", "TotalRound": "4"}, 4); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := e.loadBuff(source, source, map[string]string{"Id": "immune-def", "Factor": "immuneDef", "TotalRound": "4"}, 4); err != nil {
		t.Fatal(err)
	}
	e.originalShareEffect(target, e.service.table("Buff", "Id", "buff_206"))
	skill := source.skills[0]
	skill.target = target
	// FSkill.Do has already selected this collection before the first release.
	skill.targets = []*combatRoundActor{target}
	rng := NewCombatRandom(0)
	d1 := (1000 - 100) * .5 / 2 * .9 * (1 + rng.Value()*.2 - .1)
	d2 := (1000 - 200) * .5 / 2 * .8 * (1 + rng.Value()*.2 - .1)
	if err := e.originalSharedDamage(skill, target, 1, 1, 1000, 0); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, m1.hp, 80000)
	closeCombat(t, m2.hp, 80000)
	if e.rng.Draws() != 2 {
		t.Fatal("native recipient variance draws", e.rng.Draws())
	}
	closeCombat(t, m1.shield("pshield"), 9999-d1)
	closeCombat(t, m2.shield("pshield"), 9999-d2)
	for _, actor := range []*combatRoundActor{m1, m2} {
		closeCombat(t, actor.shield("shield"), 9999)
		closeCombat(t, actor.shield("mshield"), 9999)
	}
	m1.state = 4
	m1.hp = 0
	m1.base["dodge"] = source.base["hit"] + 10
	draws := e.rng.Draws()
	if err := e.originalSharedDamage(skill, target, 1, 1, 1000, 0); err != nil {
		t.Fatal(err)
	}
	if e.rng.Draws() != draws+3 || m1.hp != 0 {
		t.Fatal("dead cached share must still consume variance/dodge, HP mutation forbidden")
	}
}
func TestP6OriginalShareFullReleaseRNGOrderPrimaryHalfAndNoMateFallback(t *testing.T) {
	e := p6SameDamageEngine(t)
	source, target := e.players[0].actors[0], e.players[1].actors[0]
	m1, m2 := e.players[1].actors[1], e.players[1].actors[2]
	source.base["cri"] = 0
	target.base["antiCri"] = 0
	for _, actor := range []*combatRoundActor{target, m1, m2} {
		actor.base["pdef"] = 100
		actor.base["dodge"] = 0
		actor.base["damageReduce"] = 0
	}
	source.base["att"] = 1000
	if err := e.loadBuff(source, target, e.service.table("Buff", "Id", "buff_206"), 7); err != nil {
		t.Fatal(err)
	}
	skill := source.skills[0]
	skill.target = target
	cast := skill.casts[0]
	// Prime the original FSkill.Do collection before this first release.
	skill.targets = []*combatRoundActor{target}
	rng := NewCombatRandom(0)
	rng.Value()
	rng.Value()
	factor := tableNumber(cast, "ExportFactor") * .01
	s1 := 900 * factor * .25 * (1 + rng.Value()*.2 - .1)
	s2 := 900 * factor * .25 * (1 + rng.Value()*.2 - .1)
	primary := 900 * factor * .5 * (1 + rng.Value()*.2 - .1)
	rng.Value()
	if err := e.dynamicRelease(skill, cast); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, m1.hp, 80000-s1)
	closeCombat(t, m2.hp, 80000-s2)
	closeCombat(t, target.hp, 80000-primary)
	if e.rng.Draws() != rng.Draws() || target.shareFactor != 0 || len(target.shareActors) != 0 {
		t.Fatal("native full share RNG order or onceHit lifecycle", e.rng.Draws(), rng.Draws())
	}
	one := p6PropertyTransferEngine(t)
	one.originalShareEffect(one.players[1].actors[0], one.service.table("Buff", "Id", "buff_206"))
	if len(one.players[1].actors[0].shareActors) != 0 {
		t.Fatal("native singleton has recipient")
	}
	one.service.table("Buff", "Id", "buff_206")["BuffClass"] = "1"
	if err := one.validateDynamicTrigger(combatRoundTrigger{buff: "buff_206", timer: 7}); err == nil {
		t.Fatal("wrong share factory accepted")
	}
}
func TestP6OriginalShareMagicDefenseAndDodgeExtraDrawBeforePrimary(t *testing.T) {
	e := p6SameDamageEngine(t)
	source, target := e.players[0].actors[0], e.players[1].actors[0]
	m1, m2 := e.players[1].actors[1], e.players[1].actors[2]
	m1.base["pdef"], m2.base["pdef"] = 100000, 100000
	m1.base["mdef"], m2.base["mdef"] = 50, 150
	m1.base["mdamageReduce"], m2.base["mdamageReduce"] = 10, 20
	m1.base["dodge"] = source.attribute("hit") + 30
	e.originalShareEffect(target, e.service.table("Buff", "Id", "buff_523"))
	skill := source.skills[0]
	skill.target = target
	rng := NewCombatRandom(0)
	d1 := (1000 - 50) * .3 / 2 * .9 * (1 + rng.Value()*.2 - .1) * (1 - rng.Value()*.3*2)
	d2 := (1000 - 150) * .3 / 2 * .8 * (1 + rng.Value()*.2 - .1)
	if err := e.originalSharedDamage(skill, target, 2, 1, 1000, 0); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, m1.hp, 80000-d1)
	closeCombat(t, m2.hp, 80000-d2)
	if e.rng.Draws() != 3 {
		t.Fatal("magical share variance/dodge order")
	}
}
func p6ShareWholeInputs(t *testing.T, s *GameService) []*model.FrozenBattleInput {
	var out []*model.FrozenBattleInput
	for _, x := range []struct{ hero, skills, talent string }{{"hero_022", "022_1,022_2,022_3", "tal_301"}, {"hero_052", "052_1,052_2,052_3", "tal_372"}} {
		for _, mates := range []int{0, 2} {
			actor := p6Actor(x.hero, "share-owner", x.skills, 1, 80000)
			actor.TalentList = []string{x.talent}
			f := p6Input(t, s, p6Actor("hero_047", "share-attacker", "047_1,047_2,047_3", 1, 80000), actor, 16)
			for i := 2; i <= mates+1; i++ {
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
func TestP6OriginalShareActualHero022And052CompleteSkillsSingleAndTieTeamsReplay(t *testing.T) {
	s := p6OriginalTables(t)
	for i, f := range p6ShareWholeInputs(t, s) {
		out, err := s.ComputeFrozenBattle(t.Context(), f, nil)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		shares, loads := 0, 0
		for _, v := range out.Events {
			if v.Kind == "release" && v.Actor == "1_share-owner" {
				seen[v.Skill] = true
			}
			if v.Kind == "share_damage" {
				shares++
			}
			if v.Kind == "buff_load" && (v.Buff == "buff_206" || v.Buff == "buff_523") {
				loads++
			}
		}
		if len(seen) != 3 || loads == 0 || i%2 == 1 && shares == 0 || i%2 == 0 && shares != 0 || out.Authoritative {
			t.Fatal("actual full share configuration", i, seen, shares, loads)
		}
		replay, err := s.ComputeFrozenBattle(t.Context(), f, out.Operates)
		if err != nil || !proto.Equal(out.Operates, replay.Operates) || !proto.Equal(out.Attacker, replay.Attacker) || !proto.Equal(out.Defender, replay.Defender) || out.RandomDraws != replay.RandomDraws || !reflect.DeepEqual(out.Events, replay.Events) {
			t.Fatal("native complete share replay", i, err)
		}
	}
}
