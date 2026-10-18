package service

import (
	"math"
	"testing"

	pb "hyjs-server/model/protocol"
)

func TestP6AdditionalAttackOriginalTargetScalarAfterBuffRemoval(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6Input(t, s, p6Actor("hero_047", "source", "047_1", 1, 100000),
		p6Actor("hero_047", "target", "047_1", 1, 100000), 6)
	e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input,
		rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	source, target := e.players[0].actors[0], e.players[1].actors[0]
	source.base["att"] = 50000
	target.base["pdef"], target.base["pdamageReduce"] = 0, 0
	for _, id := range []string{"buff_627", "buff_517"} {
		if err := e.loadBuff(target, target, e.table("Buff", "Id", id), 4); err != nil {
			t.Fatal(err)
		}
	}
	for _, buff := range target.buffs {
		if buff.row["Id"] == "buff_627" {
			e.unloadBuff(target, buff)
			break
		}
	}
	if math.Float64bits(target.originalFraction("damageReduce")) != math.Float64bits(.35) {
		t.Fatal("original target scalar changed", target.originalFraction("damageReduce"))
	}
	if err := e.loadBuff(source, target, e.table("Buff", "Id", "buff_135"), 12); err != nil {
		t.Fatal(err)
	}
	// 原 Args1=200，IL按50000*200*.01*.65得到65000，保原 AddHp 和原class8导出。
	if math.Float64bits(target.hp) != math.Float64bits(35000) {
		t.Fatal("original immediate additional damage lost target scalar", target.hp)
	}
	count := 0
	for _, event := range e.events {
		if event.Kind == "additional_attack_damage" {
			count++
			if math.Float64bits(event.Value) != math.Float64bits(65000) {
				t.Fatal("immediate damage export changed", event.Value)
			}
		}
	}
	if count != 1 || e.rng.Draws() != 0 {
		t.Fatal("immediate damage added an action or RNG draw", count, e.rng.Draws())
	}
}

func TestP6AdditionalAttackScalarPreservesMagicRealAndMinimum(t *testing.T) {
	source := &combatRoundActor{base: map[string]float64{"att": 500, "matt": 100,
		"damageAdd": 25, "damageReduce": 99}, attackType: 2}
	target := &combatRoundActor{base: map[string]float64{"mdef": 100,
		"damageReduce": 25, "mdamageReduce": 25}}
	row := map[string]string{"Factor": "additionAttDam", "Args1": "200"}
	if got := combatOriginalAdditionalAttackDamage(source, target, row); got != 750 {
		t.Fatal("magic defense/typed reduction or target scalar source changed", got)
	}
	row["Factor"] = "additionAttRealDam"
	if got := combatOriginalAdditionalAttackDamage(source, target, row); got != 1500 {
		t.Fatal("real additional damage did not bypass defense/reduction", got)
	}
	row["Factor"] = "additionAttDam"
	target.base["mdef"] = 1000000
	if got := combatOriginalAdditionalAttackDamage(source, target, row); got != 1 {
		t.Fatal("original minimum damage changed", got)
	}
}
