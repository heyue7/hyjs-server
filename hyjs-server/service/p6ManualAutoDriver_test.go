package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"testing"
)

func p6ActualManualAutoInput(t *testing.T) *model.FrozenBattleInput {
	t.Helper()
	raw, err := readTestAnalysis75("../../analysis/p6/actual-phone-rush-manual-auto-input-v161/000.frozen-input.json")
	if err != nil || p6Digest(raw) != "647f1129df6efd57f95dba3ed8fe31135d5750d76ebe592cc34e222e0e83f9d0" {
		t.Fatal("manual source SHA", err)
	}
	var input model.FrozenBattleInput
	if err = json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	return &input
}

// Compare the actual UI End with a fresh live-player calculation, without using
// the supplied record as engine instructions or rewriting its first clock.
func TestP6ManualAutoActualEndAndFrozenIsolation(t *testing.T) {
	input := p6ActualManualAutoInput(t)
	raw, err := readTestAnalysis75("../../analysis/p7/actual-scoped-phone-v149-20261015/p7-d187f5f2045cd967d55b3f9c4da55383-CSRushFightEnd-1792070372964.pb")
	if err != nil {
		t.Fatal(err)
	}
	end := &pb.CSRushFightEnd{}
	if err = proto.Unmarshal(raw, end); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(input)
	out, err := (&GameService{}).ComputeOriginalP6ProtocolFinalHP(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := proto.MarshalOptions{Deterministic: true}.Marshal(end.GetOptRecord())
	if err != nil || p6Digest(actual) != "543aa85e039f658632ee18c741625b2c4d3ad6e8cba14d24c23d63b7cc21214a" {
		t.Fatal("actual End SHA", err)
	}
	got, err := proto.MarshalOptions{Deterministic: true}.Marshal(out.OriginalOperates)
	if err != nil || string(actual) != string(got) || out.Outcome.Authoritative {
		t.Fatal("actual complete End mismatch", err)
	}
	after, _ := json.Marshal(input)
	if string(before) != string(after) {
		t.Fatal("source changed")
	}
	clone, err := cloneP6CalculationInput(input)
	if err != nil {
		t.Fatal(err)
	}
	*clone.DiagnosticManualAutoStartRoundTime = 0
	if *input.DiagnosticManualAutoStartRoundTime != 30.100000000000158 {
		t.Fatal("calculation delay pointer aliased")
	}
	s := &GameService{Tables: input.Tables}
	frozen, err := s.FreezeBattleInput(*input)
	if err != nil {
		t.Fatal(err)
	}
	*frozen.DiagnosticManualAutoStartRoundTime = 0
	if *input.DiagnosticManualAutoStartRoundTime != 30.100000000000158 {
		t.Fatal("capture delay pointer aliased")
	}
}

func TestP6ManualAutoDriverRejectsUnboundControls(t *testing.T) {
	input := p6ActualManualAutoInput(t)
	if err := ValidateP6AIDriverPolicy(input); err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{"missing-policy", "missing-delay", "nan", "inf", "negative", "past-deadline", "pvp", "no-monster", "view", "defender-first", "compat", "unknown-policy"} {
		copy, err := cloneP6CalculationInput(input)
		if err != nil {
			t.Fatal(err)
		}
		switch variant {
		case "missing-policy":
			copy.DiagnosticAIDriverPolicy = ""
		case "missing-delay":
			copy.DiagnosticManualAutoStartRoundTime = nil
		case "nan":
			copy.DiagnosticManualAutoStartRoundTime = proto.Float64(math.NaN())
		case "inf":
			copy.DiagnosticManualAutoStartRoundTime = proto.Float64(math.Inf(1))
		case "negative":
			copy.DiagnosticManualAutoStartRoundTime = proto.Float64(-0.1)
		case "past-deadline":
			copy.DiagnosticManualAutoStartRoundTime = proto.Float64(110.1)
		case "pvp":
			copy.BattleType = 11
		case "no-monster":
			copy.Monster = nil
		case "view":
			copy.IsAttToDef = true
		case "defender-first":
			copy.DefenderFirst = true
		case "compat":
			copy.PVECompatibilityRule = constUtil.P6LocalPVENoTargetFinish
		case "unknown-policy":
			copy.DiagnosticAIDriverPolicy = "unverified"
		}
		if ValidateP6AIDriverPolicy(copy) == nil {
			t.Fatal("unbound driver accepted", variant)
		}
	}
}
