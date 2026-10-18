package service

import (
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

func TestP6OriginalPlaybackEnemyFinishHintKeepsAutomaticPhase(t *testing.T) {
	e := &combatRoundEngine{input: &model.FrozenBattleInput{BattleType: 1}, camp: 1, time: 1}
	// A recorded caller accepts the SkillEnd view hint. An automatic caller
	// waits for its own AI finish notification even after all normal messages.
	e.players = [2]*combatRoundPlayer{{}, {}}
	e.originalPlaybackFinishHint = true
	if e.canFinishRound() {
		t.Fatal("automatic caller accepted the weaker SkillEnd hint")
	}
	e.originalRecordMode = true
	if e.canFinishRound() {
		t.Fatal("nil-record original automatic projection accepted a playback hint")
	}
	e.supplied = &pb.BattleOperate{}
	if !e.canFinishRound() {
		t.Fatal("original playback caller lost the actual SkillEnd callback boundary")
	}
	e.originalPlaybackFinishHint = false
	if e.canFinishRound() {
		t.Fatal("empty normal phase fabricated a SkillEnd callback")
	}
	e.copyEnemyFinishNotified = true
	if !e.canFinishRound() {
		t.Fatal("actual AI notification rejected")
	}
}

func TestP6OriginalPlaybackHintSkipsDisabledNPCNormal(t *testing.T) {
	actor := &combatRoundActor{camp: 1, heroClass: 1, hp: 100}
	chosen := &combatRoundSkill{actor: actor, state: 4, row: map[string]string{}}
	disabled := &combatRoundSkill{actor: actor, state: 0, row: map[string]string{}}
	actor.skills = []*combatRoundSkill{chosen, disabled}
	e := &combatRoundEngine{input: &model.FrozenBattleInput{BattleType: 1}, camp: 1}
	e.players[0] = &combatRoundPlayer{}
	e.players[1] = &combatRoundPlayer{actors: []*combatRoundActor{actor}}
	e.commands = []combatRoundCommand{{actor: actor, skills: []*combatRoundSkill{chosen}}}
	if e.originalHasActiveNormalSkill() {
		t.Fatal("unselected NPC normal was treated as enabled")
	}
	chosen.state = 3
	if !e.originalHasActiveNormalSkill() {
		t.Fatal("active selected normal lost its callback guard")
	}
}
