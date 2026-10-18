package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"reflect"
	"testing"
)

func p6HPChangeEngine(t *testing.T) *combatRoundEngine {
	s := p6OriginalTables(t)
	f := p6Input(t, s, p6Actor("hero_047", "injured", "047_1,047_2,047_3", 1, 1000), p6Actor("hero_047", "attacker", "047_1,047_2,047_3", 1, 80000), 18)
	healer := p6Actor("hero_009", "healer", "009_1,009_2,009_3", 2, 80000)
	healer.TalentList = []string{"tal_256"}
	f.Attacker.Formation = append(f.Attacker.Formation, healer)
	e := &combatRoundEngine{service: &GameService{Tables: f.Tables}, input: f, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	e.camp = 1
	e.players[0].actors[1].base["att"] = 100
	return e
}

func TestP6OriginalHPChangeThresholdRepeatedHealAndEnemyRound(t *testing.T) {
	e := p6HPChangeEngine(t)
	target, source := e.players[0].actors[0], e.players[1].actors[0]
	e.damageHP(source, target, 399, 1)
	closeCombat(t, target.hp, 601)
	closeCombat(t, target.hpChangeInRound, -399)
	if e.rng.Draws() != 0 {
		t.Fatal("below forty-percent boundary triggered talent")
	}
	e.damageHP(source, target, 1, 1)
	closeCombat(t, target.hp, 670)
	closeCombat(t, target.hpChangeInRound, -330)
	if e.rng.Draws() != 1 {
		t.Fatal("direct recipient has only native activation RNG", e.rng.Draws())
	}
	e.damageHP(source, target, 70, 1)
	closeCombat(t, target.hp, 600)
	closeCombat(t, target.hpChangeInRound, -400)
	if e.rng.Draws() != 2 {
		t.Fatal("covered one-round marker must still draw activation RNG")
	}
	// The original ShouldCover marker suppresses the same coefficient until it is removed.
	e.unloadBuff(target, target.buffs[0])
	e.damageHP(source, target, 1, 1)
	closeCombat(t, target.hp, 669)
	closeCombat(t, target.hpChangeInRound, -331)
	e.camp = 0
	e.damageHP(source, target, 100, 1)
	closeCombat(t, target.hp, 569)
	closeCombat(t, target.hpChangeInRound, -431)
	if e.rng.Draws() != 3 || e.failure != nil {
		t.Fatal("own round fired EnemyDamage40percent", e.failure)
	}
	if err := e.beginRound(); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, target.hpChangeInRound, 0)
}

func TestP6OriginalHPChangeShieldLethalForbiddenAndResetBoundaries(t *testing.T) {
	e := p6HPChangeEngine(t)
	target, source := e.players[0].actors[0], e.players[1].actors[0]
	if err := e.loadBuff(target, target, map[string]string{"Id": "test-hp-shield", "Factor": "pshield", "Args1": "500", "TotalRound": "4"}, 1); err != nil {
		t.Fatal(err)
	}
	e.damageHP(source, target, 500, 1)
	closeCombat(t, target.hpChangeInRound, 0)
	if e.rng.Draws() != 0 {
		t.Fatal("fully shielded damage triggered HP-change talent")
	}
	if err := e.loadBuff(target, target, map[string]string{"Id": "forbid", "Factor": "forbidAddHp", "TotalRound": "4"}, 1); err != nil {
		t.Fatal(err)
	}
	e.damageHP(source, target, 400, 1)
	closeCombat(t, target.hp, 600)
	closeCombat(t, target.hpChangeInRound, -400)
	if e.rng.Draws() != 1 {
		t.Fatal("forbidden recovery must activate but cannot mutate HP or recurse")
	}
	e.damageHP(source, target, 600, 1)
	closeCombat(t, target.hpChangeInRound, -400)
	if target.state != 4 || e.rng.Draws() != 1 {
		t.Fatal("lethal MinusHP must skip forty-percent recovery", target.state, e.rng.Draws())
	}
	// Original self-revival ResetRound clears both accumulators before positive recovery.
	if err := e.loadBuff(target, target, e.service.table("Buff", "Id", "buff_277"), 16); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, target.hpChangeInRound, target.attribute("maxHp")*.1)
}

func TestP6OriginalRecoverSourcePhysicalFormulaAndNominalOverheal(t *testing.T) {
	e := p6HPChangeEngine(t)
	target, owner := e.players[0].actors[0], e.players[0].actors[1]
	e.camp = 0
	owner.attackType = 2
	owner.base["att"], owner.base["patt"], owner.base["matt"] = 100, 10, 5000
	owner.base["treatAdd"], owner.base["cure"], target.base["heal"] = 20, 3, 7
	target.base["minusAddperhp"] = -50
	target.hp = 990
	if err := e.loadBuff(owner, target, e.service.table("Buff", "Id", "buff_418"), 19); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, target.hp, 1000)
	closeCombat(t, target.hpChangeInRound, 51.2)
	if err := e.loadBuff(owner, target, e.service.table("Buff", "Id", "buff_829"), 19); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, target.hpChangeInRound, 115.6)
	// Explicit reactivation boundary probe keeps the original heal coefficient and changes only
	// clearability so the native UnEffect/Effect path can be exercised in isolation.
	target.buffs[0].row["CanBeCleared"] = "1"
	suppressed, err := e.suppressBuffsOnce(target)
	if err != nil {
		t.Fatal(err)
	}
	owner.base["att"] = 200
	if err = e.restoreBuffsOnce(target, suppressed); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, target.hpChangeInRound, 234)
}

func TestP6OriginalHPChangeDirectRecipientOrderAndEnemyHealTimer(t *testing.T) {
	e := p6HPChangeEngine(t)
	target, owner, enemy := e.players[0].actors[0], e.players[0].actors[1], e.players[1].actors[0]
	// Probe the native direct-recipient path independently of the stored selector2.
	owner.talents[0].selectStrategy = 6
	owner.state, owner.hp = 3, 0
	target.talents = append([]combatRoundTrigger(nil), owner.talents...)
	e.damageHP(enemy, target, 400, 1)
	closeCombat(t, target.hp, 670)
	if e.rng.Draws() != 1 {
		t.Fatal("weak healer excluded, recipient healed itself, or random selection consumed RNG")
	}
	owner.state = 4
	e.damageHP(enemy, target, 70, 1)
	closeCombat(t, target.hp, 600)
	if e.rng.Draws() != 1 {
		t.Fatal("dead healer still loaded recovery")
	}
	// No original table binds timer34; this explicitly synthetic consumer tests its call boundary.
	e.service.Tables["Buff"] = append(e.service.Tables["Buff"], map[string]string{"Id": "p6_timer34_probe", "Factor": "att", "Args1": "1", "TotalRound": "4"})
	enemy.talents = []combatRoundTrigger{{buff: "p6_timer34_probe", timer: 34, chance: 1}}
	e.camp = 0
	e.changeHP(owner, target, 100, "test_heal")
	if e.rng.Draws() != 2 || enemy.attribute("att") != enemy.base["att"]+1 {
		t.Fatal("positive healing failed EnemyAddHp", e.rng.Draws())
	}
	enemy.state = 3
	e.changeHP(owner, target, 100, "test_heal")
	if e.rng.Draws() != 2 {
		t.Fatal("EnemyAddHp fired weak enemy")
	}
}

func p6HPChangeWholeInputs(t *testing.T, s *GameService) []*model.FrozenBattleInput {
	var inputs []*model.FrozenBattleInput
	for _, x := range []struct{ hero, skills, talent string }{{"hero_009", "009_1,009_2,009_3", "tal_256"}, {"hero_079", "079_1,079_2,079_3", "tal_3191"}} {
		for _, camp := range []int32{0, 1} {
			healer := p6Actor(x.hero, "native-emergency", x.skills, 2, 80000)
			healer.TalentList = []string{x.talent}
			ally := p6Actor("hero_047", "injured-mate", "047_1,047_2,047_3", 1, 6000)
			enemy := p6Actor("hero_047", "enemy", "047_1,047_2,047_3", 1, 80000)
			enemy.Prop.Att = proto.Float64(1600)
			f := p6Input(t, s, ally, enemy, 18)
			if camp == 1 {
				f.Attacker, f.Defender = f.Defender, f.Attacker
				f.Defender.Formation = append(f.Defender.Formation, healer)
			} else {
				f.Attacker.Formation = append(f.Attacker.Formation, healer)
			}
			f.Attacker.Inst.Anger, f.Defender.Inst.Anger = proto.Float64(100), proto.Float64(100)
			inputs = append(inputs, f)
		}
	}
	return inputs
}

func TestP6OriginalHPChangeActualTwoHeroesFullSkillsAndReplay(t *testing.T) {
	for _, f := range p6HPChangeWholeInputs(t, p6OriginalTables(t)) {
		s := &GameService{Tables: f.Tables}
		out, err := s.ComputeFrozenBattle(t.Context(), f, nil)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, ev := range out.Events {
			found = found || ev.Kind == "buff_load" && (ev.Buff == "buff_418" || ev.Buff == "buff_829")
		}
		if !found || out.Authoritative {
			t.Fatal("actual mapped emergency recovery did not trigger")
		}
		replay, err := s.ComputeFrozenBattle(t.Context(), f, out.Operates)
		if err != nil || !proto.Equal(out.Operates, replay.Operates) || !proto.Equal(out.Attacker, replay.Attacker) || !proto.Equal(out.Defender, replay.Defender) || out.RandomDraws != replay.RandomDraws || !reflect.DeepEqual(out.Events, replay.Events) {
			t.Fatal("actual emergency recovery full-event replay", err)
		}
	}
}

// Original recover with exactly -100% treatAdd has no HP export or shield debit.
// Keep CLR multiplication rounding explicit so ARM64 fusion cannot turn zero into damage.
func TestP6OriginalRecoveryZeroTreatFactorDoesNotSpendShield(t *testing.T) {
	e := p6HPChangeEngine(t)
	target, source := e.players[0].actors[0], e.players[0].actors[1]
	source.base["att"], source.base["patt"], source.base["treatAdd"], source.base["cure"] = 1823.9, 0, -100, 0
	target.base["heal"] = 0
	target.hp = 400
	if err := e.loadBuff(target, target, map[string]string{"Id": "zero-treat-shield", "Factor": "shield", "Args1": "50", "TotalRound": "1"}, 4); err != nil {
		t.Fatal(err)
	}
	before := len(e.events)
	if err := e.loadBuff(source, target, e.service.table("Buff", "Id", "buff_146"), 4); err != nil {
		t.Fatal(err)
	}
	if target.hp != 400 || target.shield("shield") != 50 {
		t.Fatal("zero recovery mutated HP/shield", target.hp, target.shield("shield"))
	}
	for _, event := range e.events[before:] {
		if event.Kind == "buff_hp_export" || event.Kind == "damage" || event.Kind == "buff_heal" {
			t.Fatal("zero recovery exported HP", event)
		}
	}
}
