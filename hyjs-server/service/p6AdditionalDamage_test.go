package service

import (
	"google.golang.org/protobuf/proto"
	pb "hyjs-server/model/protocol"
	"testing"
)

func p6KiritoAwakenActor() *pb.BattleHero {
	a := p6Actor("hero_001", "kirito", "001_1,001_2,001_3", 1, 80000)
	a.AwakeStage = proto.Int32(2)
	a.AwakenSkill = proto.String("0001_1,2")
	// Isolated original talent combinations exercise both awakening conditions.
	a.TalentList = []string{"tal_1087", "tal_384", "tal_257"}
	return a
}

func TestP6OriginalImmediateAdditionalDamageAndStateConditions(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6Input(t, s, p6Actor("hero_047", "source", "047_1", 1, 10000), p6Actor("hero_047", "target", "047_1", 1, 10000), 6)
	e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	a.attackType = 2
	d.base["mdef"], d.base["mdamageReduce"], d.base["damageReduce"] = 100, 10, 10
	a.base["damageAdd"] = 20
	if err := e.loadBuff(d, d, map[string]string{"Id": "source-magic-shield", "Factor": "mshield", "Args1": "300", "TotalRound": "1"}, 4); err != nil {
		t.Fatal(err)
	}
	if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_709"), 12); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.hp, 10000)
	closeCombat(t, d.shield("mshield"), 100)
	if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_135"), 12); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.hp, 9300)
	if e.rng.Draws() != 0 {
		t.Fatal("immediate additional damage drew randomness")
	}
	for _, id := range []string{"buff_149", "buff_111"} {
		if err := e.loadBuff(a, d, e.service.table("Buff", "Id", id), 12); err != nil {
			t.Fatal(err)
		}
	}
	closeCombat(t, e.conditionProperty(d, "fireDam"), 1)
	closeCombat(t, e.conditionProperty(d, "poisonDam"), 1)
	// Native real factor ignores both defense and reduction, while retaining source damage type.
	d.base["mdef"], d.base["damageReduce"], d.base["mdamageReduce"] = 10000, 100, 100
	before := d.hp
	if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_199"), 12); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.hp, before-120)
	d.state = 3
	before = d.hp
	if err := e.additionalAttackDamage(a, d, e.service.table("Buff", "Id", "buff_135")); err != nil {
		t.Fatal(err)
	}
	if err := e.bleedingHit(a, d); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.hp, before)
}

func TestP6OriginalKiritoAwakenFullSkillsAndConditionalReplay(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6Input(t, s, p6KiritoAwakenActor(), p6Actor("hero_047", "target", "047_1", 1, 80000), 18)
	input.Attacker.Inst.Anger = proto.Float64(300)
	out, err := s.ComputeFrozenBattle(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	skills := map[string]bool{}
	additional, sourceHP := false, false
	for _, event := range out.Events {
		if event.Kind == "release" {
			skills[event.Skill] = true
		}
		additional = additional || event.Kind == "additional_attack_damage"
		sourceHP = sourceHP || event.Kind == "source_maxhp_damage"
	}
	for _, skill := range []string{"001_1", "001_2", "001_3", "0001_1"} {
		if !skills[skill] {
			t.Fatal("complete original skill missing", skill)
		}
	}
	if !additional || !sourceHP || out.Authoritative {
		t.Fatal("original conditional damage did not run", additional, sourceHP)
	}
	replay, err := s.ComputeFrozenBattle(t.Context(), input, out.Operates)
	if err != nil || !proto.Equal(out.Operates, replay.Operates) || !proto.Equal(out.Attacker, replay.Attacker) || !proto.Equal(out.Defender, replay.Defender) || out.RandomDraws != replay.RandomDraws {
		t.Fatal("conditional awake replay diverged", err)
	}
}
