package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

func p6ActualKFCanonical(t *testing.T) (*GameService, *model.FrozenBattleInput) {
	t.Helper()
	s := p6OriginalTables(t)
	raw, err := readTestAnalysis75("../../analysis/p14/final-game-actual-native/actual-Game1-KFReplayData.pb")
	if err != nil || p6Digest(raw) != "18473d1c95d92d51850578f69a8fa6273039a6f77d55a0ebd6f64abf699c502c" {
		t.Fatal("original actual Game1 source", err)
	}
	source := &pb.KFReplayData{}
	if proto.Unmarshal(raw, source) != nil {
		t.Fatal("original source")
	}
	input, err := s.FreezeP6KFReplayRealmInput(source, int64(source.PlayerList[0].GetServerId()), source.PlayerList[0].GetPlayerId())
	if err != nil {
		t.Fatal(err)
	}
	return s, input
}

func TestP6KFCanonicalPendingProofAndUnknownBoundary(t *testing.T) {
	s, input := p6ActualKFCanonical(t)
	before, _ := json.Marshal(input)
	if err := validateP6KFCanonicalScope(input); err != nil {
		t.Fatal("canonical source0 shape rejected", err)
	}
	if constUtil.P6KFCanonicalNativeProof == "" {
		out, err := s.ComputeP6KFCanonicalSettlement(t.Context(), input)
		if err == nil || out != nil {
			t.Fatal("pending KF evidence applied winner")
		}
	}
	for _, proof := range []string{"", constUtil.P6CrossStarterNativeProof, constUtil.P6StarterNativeProof, "unrelated-ordinary-17"} {
		if p6KnownKFCanonicalSettlementProof(input, proof) {
			t.Fatal("foreign proof accepted", proof)
		}
	}
	for _, variant := range []string{"right-view", "no-rule", "numeric-viewer", "wrong-realm", "ordinary17", "order", "source-order", "old-engine", "wrong-seed", "max", "winner", "extra-skill"} {
		copy, err := cloneP6CalculationInput(input)
		if err != nil {
			t.Fatal(err)
		}
		switch variant {
		case "right-view":
			copy.IsAttToDef = true
			copy.KFReplayViewer.ServerID = int64(copy.KFReplay.PlayerList[1].GetServerId())
		case "no-rule":
			copy.KFReplayViewer.CompatibilityRule = ""
		case "numeric-viewer":
			copy.KFReplayViewer.ServerID = 0
			copy.KFReplayViewer.CompatibilityRule = ""
		case "wrong-realm":
			copy.KFReplayViewer.ServerID = 999
		case "ordinary17":
			copy.KFReplay = nil
			copy.KFReplayViewer = nil
		case "order":
			copy.DefenderFirst = true
		case "source-order":
			copy.KFReplay.PlayerList[0], copy.KFReplay.PlayerList[1] = copy.KFReplay.PlayerList[1], copy.KFReplay.PlayerList[0]
		case "old-engine":
			copy.EngineVersion = "apk-old"
		case "wrong-seed":
			copy.Seed++
		case "max":
			copy.MaxRound = 2
		case "winner":
			copy.KFReplay.WinPlayerId = proto.Int64(1)
		case "extra-skill":
			copy.Attacker.Formation[0].SkillOrder = proto.String("047_1,047_2,unknown")
		}
		if validateP6KFCanonicalScope(copy) == nil {
			t.Fatal("unknown source admitted", variant)
		}
	}
	if realm, pid, err := P6KFCanonicalWinner(input, &model.CombatFinalHPOutcome{Outcome: &model.CombatOutcome{Authoritative: true, CapabilityHash: constUtil.P6CrossStarterNativeProof, Winner: 0}}); err == nil || realm != 0 || pid != 0 {
		t.Fatal("ambiguous numeric ID inferred winner realm")
	}
	after, _ := json.Marshal(input)
	if string(before) != string(after) {
		t.Fatal("source mutated")
	}
}

// The public calculation returns one source0 joint winner even when both
// original player IDs are equal. Terminal changes cannot forge that receipt.
func TestP6KFCanonicalCertifiedJointWinner(t *testing.T) {
	s, input := p6ActualKFCanonical(t)
	before, _ := json.Marshal(input)
	out, err := s.ComputeP6KFCanonicalSettlement(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Outcome.Authoritative || !p6KnownKFCanonicalSettlementProof(input, out.Outcome.CapabilityHash) || out.Outcome.FinalRound != 31 || out.Outcome.Winner != 0 {
		t.Fatal("certified original terminal", out.Outcome)
	}
	realm, pid, err := P6KFCanonicalWinner(input, out)
	if err != nil || realm != int64(input.KFReplay.PlayerList[0].GetServerId()) || pid != input.KFReplay.PlayerList[0].GetPlayerId() {
		t.Fatal("joint winner", realm, pid, err)
	}
	for _, kind := range []string{"winner", "hp", "source", "proof"} {
		raw, _ := json.Marshal(out)
		var changed model.CombatFinalHPOutcome
		if err = json.Unmarshal(raw, &changed); err != nil {
			t.Fatal(err)
		}
		switch kind {
		case "winner":
			changed.Outcome.Winner = 1
		case "hp":
			changed.FinalActors[0].CurrentHP++
		case "source":
			changed.CalculationSourceSHA256 = "other"
		case "proof":
			changed.Outcome.CapabilityHash = constUtil.P6CrossStarterNativeProof
		}
		if _, _, err = P6KFCanonicalWinner(input, &changed); err == nil {
			t.Fatal("altered receipt accepted", kind)
		}
	}
	after, _ := json.Marshal(input)
	if string(before) != string(after) {
		t.Fatal("source mutated")
	}
}
