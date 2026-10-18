package service

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	pb "hyjs-server/model/protocol"
	"reflect"
	"testing"
)

func p6AwakenActor() *pb.BattleHero {
	a := p6Actor("hero_006", "awake", "006_1,006_2,006_3", 1, 40000)
	a.AwakeStage = proto.Int32(2)
	a.AwakenSkill = proto.String("0006_1,2")
	a.TalentList = []string{"tal_1089"}
	a.Prop.Cri = proto.Float64(20)
	a.Prop.Dodge = proto.Float64(15)
	return a
}

func TestP6OriginalAwakenAllThreeSlotsAndFrozenPanel(t *testing.T) {
	s := p6OriginalTables(t)
	for position, expected := range [][]string{
		{"0006_1", "006_1", "006_3", "006_2"},
		{"006_1", "0006_1", "006_3", "006_2"},
		{"006_1", "006_2", "006_3", "0006_1"},
	} {
		a := p6AwakenActor()
		a.AwakenSkill = proto.String(fmt.Sprintf("0006_1,%d", position))
		input := p6Input(t, s, a, p6Actor("hero_047", "target", "047_1", 1, 40000), 12)
		e := &combatRoundEngine{service: &GameService{Tables: input.Tables}, input: input, rng: NewCombatRandom(0), operate: &pb.BattleOperate{}}
		if err := e.initialize(); err != nil {
			t.Fatal(position, err)
		}
		actor := e.players[0].actors[0]
		var actual []string
		for _, skill := range actor.skills {
			actual = append(actual, skill.id)
		}
		if !reflect.DeepEqual(actual, expected) || actor.skill(2).id != "006_3" {
			t.Fatal("native awake insertion changed ultimate", position, actual)
		}
		closeCombat(t, actor.attribute("att"), input.Attacker.Formation[0].Prop.GetAtt())
		closeCombat(t, actor.attribute("cri"), 20)
		closeCombat(t, actor.attribute("dodge"), 15)
		out, err := s.ComputeFrozenBattle(t.Context(), input, nil)
		if err != nil {
			t.Fatal("awake whole battle", position, err)
		}
		found := false
		for _, event := range out.Events {
			found = found || event.Kind == "release" && event.Skill == "0006_1"
		}
		if !found || out.Authoritative {
			t.Fatal("actual awake cast not executed", position)
		}
		replayed, err := s.ComputeFrozenBattle(t.Context(), input, out.Operates)
		if err != nil || !proto.Equal(out.Operates, replayed.Operates) || !proto.Equal(out.Attacker, replayed.Attacker) || out.RandomDraws != replayed.RandomDraws {
			t.Fatal("awake replay", position, err)
		}
	}
}

func TestP6AwakenRejectsBareDuplicateForeignAndMissingTalent(t *testing.T) {
	for _, mutation := range []func(*pb.BattleHero){
		func(a *pb.BattleHero) { a.AwakenSkill = proto.String("0006_1") },
		func(a *pb.BattleHero) { a.AwakenSkill = proto.String("0006_1,3") },
		func(a *pb.BattleHero) { a.AwakenSkill = proto.String("0001_1,2") },
		func(a *pb.BattleHero) { a.SkillOrder = proto.String("006_1,006_2,006_3,0006_1") },
		func(a *pb.BattleHero) { a.TalentList = nil },
		func(a *pb.BattleHero) { a.AwakeStage = proto.Int32(1) },
	} {
		s := p6OriginalTables(t)
		a := p6AwakenActor()
		mutation(a)
		input := p6Input(t, s, a, p6Actor("hero_047", "target", "047_1", 1, 40000), 12)
		if _, err := s.ComputeFrozenBattle(t.Context(), input, nil); err == nil {
			t.Fatal("malformed native awaken input accepted", a)
		}
	}
}
