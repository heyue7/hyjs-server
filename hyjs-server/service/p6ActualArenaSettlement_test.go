package service

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"hyjs-server/constUtil"
	"hyjs-server/model"
)

func TestP6ActualArenaWholePartyIndependentOriginalScope(t *testing.T) {
	s := &GameService{}
	for index := 0; index < 8; index++ {
		raw, err := os.ReadFile(fmt.Sprintf("../../analysis/p6/actual-full-party-arena2-inputs-v151/%03d.frozen-input.json", index))
		if err != nil {
			t.Fatal(err)
		}
		var input model.FrozenBattleInput
		if err = json.Unmarshal(raw, &input); err != nil {
			t.Fatal(err)
		}
		before, _ := json.Marshal(input)
		if err = validateP6ActualArenaScope(&input); err != nil {
			t.Fatal("whole actual party shape", err)
		}
		diagnostic, err := s.ComputeOriginalP6ProtocolFinalHP(t.Context(), &input, nil)
		if err != nil || diagnostic.Outcome.Authoritative {
			t.Fatal("type2 complete diagnostic", err)
		}
		if err = validateP6ActualPartyTerminal(&input, diagnostic); err != nil {
			t.Fatal("complete known terminal", err)
		}
		out, err := s.ComputeP6ActualArenaSettlement(t.Context(), &input)
		if constUtil.P6ActualArenaNativeProof == "" {
			if err == nil || out != nil {
				t.Fatal("Arena borrowed Friend certificate")
			}
		} else if err != nil || out == nil || !out.Outcome.Authoritative || out.Outcome.CapabilityHash != constUtil.P6ActualArenaNativeProof {
			t.Fatal("certified Arena party rejected", err)
		}
		after, _ := json.Marshal(input)
		if string(before) != string(after) {
			t.Fatal("original type/actors/skills altered")
		}
		view, err := cloneP6CalculationInput(&input)
		if err != nil {
			t.Fatal(err)
		}
		view.BattleType = 11
		if validateP6ActualArenaScope(view) == nil {
			t.Fatal("Friend wrapper accepted as Arena")
		}
	}
}
