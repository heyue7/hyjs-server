package service

import (
	"encoding/json"
	"fmt"
	"testing"

	"hyjs-server/constUtil"
	"hyjs-server/model"
)

func TestP6ActualPartyUltimateScopeRequiresIndependentOriginalProof(t *testing.T) {
	s := &GameService{}
	for index := 0; index < 4; index++ {
		raw, err := readTestAnalysis75(fmt.Sprintf("../../analysis/p6/actual-demo-ultimate-mirror-inputs-v147/%03d.frozen-input.json", index))
		if err != nil {
			t.Fatal(err)
		}
		var input model.FrozenBattleInput
		if err = json.Unmarshal(raw, &input); err != nil {
			t.Fatal(err)
		}
		before, _ := json.Marshal(input)
		if err = validateP6ActualPartyScope(&input); err != nil {
			t.Fatal("actual full source shape", err)
		}
		result, err := s.ComputeOriginalP6ProtocolFinalHP(t.Context(), &input, nil)
		if err != nil || result.Outcome.Authoritative {
			t.Fatal("diagnostic original ult calculation", err)
		}
		begins := map[string]int{}
		for _, event := range result.Outcome.Events {
			if event.Kind == "skill_begin" {
				begins[event.Skill]++
			}
		}
		required := []string{"001_3", "003_3"}
		if index > 1 {
			required = []string{"025_3", "047_3"}
		}
		for _, id := range required {
			if begins[id] == 0 {
				t.Fatal("probe ultimate was only queued", id)
			}
		}
		if err = validateP6ActualPartyTerminal(&input, result); err != nil {
			t.Fatal("complete candidate terminal", err)
		}
		out, err := s.ComputeP6ActualPartySettlement(t.Context(), &input)
		if constUtil.P6ActualPartyNativeProof == "" && (err == nil || out != nil) {
			t.Fatal("unproven ultimate opened public authority")
		}
		if constUtil.P6ActualPartyNativeProof != "" && (err != nil || out == nil || !out.Outcome.Authoritative || out.Outcome.CapabilityHash != constUtil.P6ActualPartyNativeProof) {
			t.Fatal("certified original full-party ultimate rejected", err)
		}
		event := result.Outcome.Events[0]
		original := result.Outcome.Events
		event.Skill = "052_3"
		result.Outcome.Events = append(append(original[:0:0], original...), event)
		if validateP6ActualPartyTerminal(&input, result) == nil {
			t.Fatal("unobserved executed skill accepted")
		}
		event.Skill = ""
		event.Buff = "buff_1"
		result.Outcome.Events = append(append(original[:0:0], original...), event)
		if validateP6ActualPartyTerminal(&input, result) == nil {
			t.Fatal("unobserved talent effect accepted")
		}
		after, _ := json.Marshal(input)
		if string(before) != string(after) {
			t.Fatal("actual source actor/skill/HP mutated")
		}
	}
}
