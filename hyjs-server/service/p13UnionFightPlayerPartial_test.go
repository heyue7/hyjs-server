package service

import (
	"bytes"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"testing"
)

func TestP13UFPlayerPartialNewPolicyPreservesLegacyAndActualState(t *testing.T) {
	s := p6OriginalTables(t)
	raw, err := readTestAnalysis75("../../analysis/p6/native-actual-war-wounded14-inputs-v196/000.SCUFFightRecordReplay.pb")
	if err != nil {
		t.Fatal(err)
	}
	typed := &pb.SCUFFightRecordReplay{}
	if proto.Unmarshal(raw, typed) != nil {
		t.Fatal("actual source")
	}
	input, err := s.FreezeP6UFPlayerInput(typed)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.ComputeP6UFPartialPlayerSettlement(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	old := p13UFPlayerBattle172Rules()
	saved, _ := json.Marshal(old)
	if bytes.Contains(saved, []byte("partial_capability_proof")) || p13UFPlayerBattleTerminalForRules(old, input, out) {
		t.Fatal("old offer gained partial authorization")
	}
	rules := *old
	rules.Policy.ID = constUtil.P13UFPlayerPartialPolicyID
	rules.FullHPOnly = false
	rules.PartialCapabilityProof = constUtil.P6UFPartialPlayerNativeProof
	if !p13UFPlayerBattleRulesValid(&rules) || !p13UFPlayerBattleTerminalForRules(&rules, input, out) {
		t.Fatal("independent ID2 source capability")
	}
	before := p13UFPlayerBattle172Frames(typed.PlayerList)
	actualBefore, _ := json.Marshal(before)
	after, dead, killer, _, err := p13UFPlayerBattleApply(before, input, out, &rules)
	if err != nil || len(after) != 2 || dead == 0 || killer == 0 {
		t.Fatal("current-state whole-player terminal", err)
	}
	again, _ := json.Marshal(before)
	if !bytes.Equal(actualBefore, again) {
		t.Fatal("actual beforeHP/AP mutated")
	}
	if _, _, _, _, err = p13UFPlayerBattleApply(before, input, out, old); err == nil {
		t.Fatal("partial applied through old fullHP offer")
	}
	frame := before[0]
	battle := typed.PlayerList[0]
	body, _ := proto.Marshal(battle)
	frameBody, _ := proto.Marshal(frame)
	actor := &db.P13UnionFightFieldPlayer{PlayerID: battle.GetPlayerId(), Camp: int64(frame.GetCamp()), Battle: body, BattleHash: p6Digest(body), Frame: frameBody, FrameHash: p6Digest(frameBody)}
	projected, err := p13UFPlayerBattleSourceForRules(actor, frame, battle, 1, &rules)
	if err != nil || !proto.Equal(projected, battle) {
		t.Fatal("real current HP/AP projection changed", err)
	}
	if _, err = p13UFPlayerBattleSourceForRules(actor, frame, battle, 1, old); err == nil {
		t.Fatal("old source projected partial state")
	}
	for _, branch := range []string{"HP", "AP", "proof", "ID"} {
		changed := rules
		switch branch {
		case "HP":
			bad := proto.Clone(frame).(*pb.InstUFPlayer)
			bad.FightInfo.HpPos[0] = 0
			if _, err = p13UFPlayerBattleSourceForRules(actor, bad, battle, 1, &rules); err == nil {
				t.Fatal("dead projection")
			}
			continue
		case "AP":
			bad := proto.Clone(frame).(*pb.InstUFPlayer)
			bad.FightInfo.Anger = proto.Float64(301)
			if _, err = p13UFPlayerBattleSourceForRules(actor, bad, battle, 1, &rules); err == nil {
				t.Fatal("invalid currentAP")
			}
			continue
		case "proof":
			changed.PartialCapabilityProof = constUtil.P6UFPlayerNativeProof
		case "ID":
			changed.Policy.ID = 1
		}
		if p13UFPlayerBattleRulesValid(&changed) || p13UFPlayerBattleTerminalForRules(&changed, input, out) {
			t.Fatal("foreign partial rule", branch)
		}
	}
	finalOld, _ := json.Marshal(old)
	if !bytes.Equal(saved, finalOld) {
		t.Fatal("old frozen rule bytes changed")
	}
}
