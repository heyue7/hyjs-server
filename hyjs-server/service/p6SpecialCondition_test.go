package service

import (
	"google.golang.org/protobuf/proto"
	pb "hyjs-server/model/protocol"
	"testing"
)

func p6AsunaActor() *pb.BattleHero {
	a := p6Actor("hero_003", "asuna", "003_1,003_2,003_3", 1, 80000)
	a.Prop.Cri = proto.Float64(100)
	return a
}

func TestP6OriginalSpecialConditionPositiveAngerAndIce(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6Input(t, s, p6AsunaActor(), p6Actor("hero_047", "target", "047_1", 1, 80000), 12)
	e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
	if err := e.initialize(); err != nil {
		t.Fatal(err)
	}
	a, d := e.players[0].actors[0], e.players[1].actors[0]
	cast := e.service.table("SkillCast", "SkillCastId", "299")
	condition := combatRoundTrigger{condition: int32(tableNumber(cast, "Buff0TriggerCondition")), args: cast["Buff0ConditionArgs"]}
	d.speState = 1
	if e.conditionTrue(condition, a, d) {
		t.Fatal("normal state matched original float condition")
	}
	d.speState = 2
	if !e.conditionTrue(condition, a, d) {
		t.Fatal("original float condition did not match")
	}
	if err := e.loadBuff(a, a, e.service.table("Buff", "Id", "buff_197"), 12); err != nil {
		t.Fatal(err)
	}
	e.addAnger(a, 10)
	closeCombat(t, e.players[0].anger, 20)
	e.addAnger(a, -10)
	closeCombat(t, e.players[0].anger, 10)
	for _, buff := range append([]*combatRoundBuff(nil), a.buffs...) {
		e.unloadBuff(a, buff)
	}
	if err := e.loadBuff(d, a, e.service.table("Buff", "Id", "buff_385"), 12); err != nil {
		t.Fatal(err)
	}
	e.addAnger(a, 10)
	closeCombat(t, e.players[0].anger, 10)
	if err := e.loadBuff(d, a, e.service.table("Buff", "Id", "buff_464"), 12); err != nil {
		t.Fatal(err)
	}
	e.addAnger(a, 10)
	closeCombat(t, e.players[0].anger, 10)
	e.addAnger(a, -5)
	closeCombat(t, e.players[0].anger, 5)
	for _, buff := range append([]*combatRoundBuff(nil), a.buffs...) {
		e.unloadBuff(a, buff)
	}
	e.addAnger(a, 10)
	closeCombat(t, e.players[0].anger, 15)
	hit, dodge := d.attribute("hit"), d.attribute("dodge")
	if err := e.loadBuff(a, d, e.service.table("Buff", "Id", "buff_152"), 12); err != nil {
		t.Fatal(err)
	}
	closeCombat(t, d.attribute("hit"), hit-20)
	closeCombat(t, d.attribute("dodge"), dodge-20)
	closeCombat(t, e.conditionProperty(d, "ice"), 1)
	if d.hasState("dizzy") || !d.ready(d.skill(0)) {
		t.Fatal("plain original ice turned into a stun")
	}
	for count := 0; count < 2; count++ {
		if err := e.endRound(); err != nil {
			t.Fatal(err)
		}
	}
	closeCombat(t, d.attribute("hit"), hit)
	closeCombat(t, d.attribute("dodge"), dodge)
	closeCombat(t, e.conditionProperty(d, "ice"), 0)
}

func TestP6OriginalAsunaCompleteSkillsConditionalAngerIceAndReplay(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6Input(t, s, p6AsunaActor(), p6Actor("hero_047", "target", "047_1", 1, 80000), 18)
	input.Attacker.Inst.Anger = proto.Float64(300)
	out, err := s.ComputeFrozenBattle(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	skills := map[string]bool{}
	boost, ice := false, false
	for _, event := range out.Events {
		if event.Kind == "release" {
			skills[event.Skill] = true
		}
		if event.Kind == "buff_load" {
			boost = boost || event.Buff == "buff_197"
			ice = ice || event.Buff == "buff_152"
		}
	}
	for _, skill := range []string{"003_1", "003_2", "003_3"} {
		if !skills[skill] {
			t.Fatal("complete original Asuna skill missing", skill)
		}
	}
	if !boost || !ice || out.Authoritative {
		t.Fatal("native conditional effects absent", boost, ice)
	}
	replay, err := s.ComputeFrozenBattle(t.Context(), input, out.Operates)
	if err != nil || !proto.Equal(out.Operates, replay.Operates) || !proto.Equal(out.Attacker, replay.Attacker) || !proto.Equal(out.Defender, replay.Defender) || out.RandomDraws != replay.RandomDraws {
		t.Fatal("complete Asuna replay diverged", err)
	}
}
