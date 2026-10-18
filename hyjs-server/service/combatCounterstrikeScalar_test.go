package service

import (
	"math"
	"testing"

	pb "hyjs-server/model/protocol"
)

// 原 3% 减伤被移除后，仍在场的 35% 保留其真实 scalar；反击不能再乘100再除100。
func TestP6CounterstrikeOriginalScalarAfterRealBuffRemoval(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6Input(t, s, p6Actor("hero_047", "attacker", "047_1", 1, 100000),
		p6Actor("hero_047", "retaliator", "047_1", 1, 100000), 6)
	e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input,
		rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	a, owner := e.players[0].actors[0], e.players[1].actors[0]
	owner.base["att"] = 500000
	a.base["pdef"], a.base["pdamageReduce"] = 0, 0
	for _, id := range []string{"buff_627", "buff_517"} {
		if err := e.loadBuff(a, a, e.table("Buff", "Id", id), 4); err != nil {
			t.Fatal(err)
		}
	}
	for _, buff := range a.buffs {
		if buff.row["Id"] == "buff_627" {
			e.unloadBuff(a, buff)
			break
		}
	}
	if math.Float64bits(a.originalFraction("damageReduce")) != math.Float64bits(.35) {
		t.Fatal("original Effect/UnEffect scalar changed", a.originalFraction("damageReduce"))
	}
	if err := e.loadBuff(owner, owner, e.table("Buff", "Id", "buff_259"), 7); err != nil {
		t.Fatal(err)
	}
	if err := e.counterStrike(owner, a, 1000000); err != nil {
		t.Fatal(err)
	}
	// FRelease scalar .35 gives exactly65000; old percent round trip gives64999.99999999999.
	if math.Float64bits(a.hp) != math.Float64bits(35000) {
		t.Fatal("ordinary counterstrike lost original scalar precision", a.hp)
	}
	var count int
	for _, event := range e.events {
		if event.Kind == "counterstrike" {
			count++
			if math.Float64bits(event.Value) != math.Float64bits(65000) {
				t.Fatal("original counterstrike damage export changed", event.Value)
			}
		}
	}
	if count != 1 || e.rng.Draws() != 0 {
		t.Fatal("single retaliation changed action/RNG count", count, e.rng.Draws())
	}
}

func TestP6CounterstrikeScalarKeepsOriginalAttackerReductionAndRealCap(t *testing.T) {
	owner := &combatRoundActor{base: map[string]float64{"att": 500, "patt": 0,
		"counterStrikeDam": 20, "damageAdd": 0}, attackType: 1}
	attacker := &combatRoundActor{base: map[string]float64{"damageReduce": 25}}
	target := &combatRoundActor{base: map[string]float64{"damageReduce": 99,
		"pdef": 0, "pdamageReduce": 0}}
	if got := combatOriginalCounterstrikeDamage(owner, attacker, target, 1000); got != 75 {
		t.Fatal("area selection substituted target for original attacker", got)
	}
	if got := combatOriginalCounterstrikeDamage(owner, attacker, target, 10); got != 2 {
		t.Fatal("original incoming damage cap changed", got)
	}
	owner.counterReal = true
	if got := combatOriginalCounterstrikeDamage(owner, attacker, target, 1000); got != 100 {
		t.Fatal("real retaliation did not bypass defense/reduction", got)
	}
}
