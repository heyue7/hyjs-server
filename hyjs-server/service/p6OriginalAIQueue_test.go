package service

import (
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

// Original PlayerAI.Update removes Done entries at 0018..0056 before actor
// updates. A SkillEnd after that cursor cannot close its queue in the same Tick.
func TestP6OriginalAIQueueDoneNormalAndUltimateCleanupReturn(t *testing.T) {
	e := &combatRoundEngine{service: &GameService{Tables: map[string][]map[string]string{}}, input: &model.FrozenBattleInput{Tables: map[string][]map[string]string{}}, rng: NewCombatRandom(42), players: [2]*combatRoundPlayer{{}, {}}}
	e.aiDealingNormals = []*combatRoundSkill{{state: 4}}
	if e.canFinishRound() {
		t.Fatal("finished actor bypassed original still-enqueued normal")
	}
	if err := e.step(); err != nil || !e.canFinishRound() || len(e.aiDealingNormals) != 0 {
		t.Fatal("next AI.Update did not remove Done normal", err)
	}
	e.aiUltimateCleanup = true
	if e.canFinishRound() {
		t.Fatal("ultimate cleanup bypassed original return")
	}
	if err := e.step(); err != nil || e.canFinishRound() || !e.aiCleanupReturned {
		t.Fatal("00ab cleanup Update ended round in same Tick", err)
	}
	if err := e.step(); err != nil || !e.canFinishRound() {
		t.Fatal("following original AI.Update did not allow FinishRound", err)
	}
	e.supplied = &pb.BattleOperate{}
	e.aiDealingNormals = []*combatRoundSkill{{state: 4}}
	e.aiUltimateCleanup = true
	if e.canFinishRound() {
		t.Fatal("supplied replay bypassed original normal completion queue")
	}
}
