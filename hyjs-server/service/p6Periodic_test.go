package service

import (
	"google.golang.org/protobuf/proto"
	pb "hyjs-server/model/protocol"
	"testing"
)

func TestP6OriginalPeriodicStateValuesRecoveryPenaltyShieldsAndExpiration(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6Input(t, s, p6Actor("hero_047", "source", "047_1", 1, 2000), p6Actor("hero_047", "target", "047_1", 1, 2000), 8)
	e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	source, target := e.players[0].actors[0], e.players[1].actors[0]
	target.base["pdef"], target.base["mdef"] = 100, 100
	for _, id := range []string{"buff_111", "buff_149"} {
		if err := e.loadBuff(source, target, e.service.table("Buff", "Id", id), 12); err != nil {
			t.Fatal(err)
		}
	}
	closeCombat(t, target.attribute("att"), 510)
	closeCombat(t, target.attribute("pdef"), 97)
	closeCombat(t, target.attribute("mdef"), 97)
	closeCombat(t, target.buffs[0].value, 300)
	closeCombat(t, target.buffs[1].value, 300)
	target.hp = 1000
	e.changeHP(source, target, 100, "skill_heal")
	closeCombat(t, target.hp, 1095)
	// Only matching typed shields are spent; the original type-specific shield precedes common.
	for _, row := range []map[string]string{
		{"Id": "magic-shield", "Factor": "mshield", "Args1": "100", "TotalRound": "2"},
		{"Id": "common-shield", "Factor": "shield", "Args1": "700", "TotalRound": "2"},
		{"Id": "physical-shield", "Factor": "pshield", "Args1": "900", "TotalRound": "2"},
	} {
		if err := e.loadBuff(source, target, row, 12); err != nil {
			t.Fatal(err)
		}
	}
	source.base["att"] = 9999
	e.round, e.camp = 2, 1
	if err := e.beginRound(); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, target.hp, 1095)
	closeCombat(t, target.shield("mshield"), 0)
	closeCombat(t, target.shield("shield"), 200)
	closeCombat(t, target.shield("pshield"), 900)
	if e.rng.Draws() != 0 {
		t.Fatal("periodic processing consumed random", e.rng.Draws())
	}
	if err := e.endRound(); err != nil {
		t.Fatal(err)
	}
	if err := e.endRound(); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, target.attribute("att"), 500)
	closeCombat(t, target.attribute("pdef"), 100)
	closeCombat(t, target.attribute("minusAddperhp"), 0)
}

func TestP6OriginalFirePoisonWholeFightAndOperationReplay(t *testing.T) {
	s := p6OriginalTables(t)
	actor := p6Actor("hero_008", "burn-and-poison", "008_1", 1, 12000)
	actor.TalentList = []string{"tal_291"}
	input := p6Input(t, s, actor, p6Actor("hero_047", "enemy", "047_1", 1, 12000), 12)
	out, err := s.ComputeFrozenBattle(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	fire, poison := 0, 0
	for _, event := range out.Events {
		if event.Kind == "periodic_damage" {
			switch event.Buff {
			case "buff_149":
				fire++
			case "buff_111":
				poison++
			}
		}
	}
	if fire < 1 || poison < 1 || out.Authoritative {
		t.Fatal("original skill/talent periodic mechanics missing", fire, poison)
	}
	replay, err := s.ComputeFrozenBattle(t.Context(), input, out.Operates)
	if err != nil || !proto.Equal(out.Operates, replay.Operates) || !proto.Equal(out.Defender, replay.Defender) || out.RandomDraws != replay.RandomDraws {
		t.Fatal("periodic full replay changed", err)
	}
}

func TestP6PeriodicLethalAndNegativeNativeValue(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6Input(t, s, p6Actor("hero_047", "a", "047_1", 1, 1000), p6Actor("hero_047", "d", "047_1", 1, 1000), 4)
	e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	d.hp = 200
	d.base["antiPoison"] = 50000
	if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_111"), 12); err != nil {
		t.Fatal(err)
	}
	// Original does not clamp the frozen value: resistance above potency reverses AddHp.
	e.round, e.camp = 2, 1
	if err := e.beginRound(); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.hp, 390)
	e.unloadBuff(d, d.buffs[0])
	d.hp = 200
	if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_149"), 12); err != nil {
		t.Fatal(err)
	}
	if err := e.beginRound(); err != nil {
		t.Fatal(err)
	}
	if d.hp != 0 || d.state != 4 || len(e.commands) != 0 {
		t.Fatal("periodic death issued a skill", d.hp, d.state, len(e.commands))
	}
}
