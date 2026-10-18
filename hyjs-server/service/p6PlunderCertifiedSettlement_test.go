package service

import (
	"encoding/json"
	"fmt"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"os"
	"testing"
)

func TestP6PlunderOriginalCertifiedCompleteStructuralScope(t *testing.T) {
	s := &GameService{}
	for index := 0; index < 11; index++ {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			raw, err := os.ReadFile(fmt.Sprintf("../../analysis/p11/actual-original-plunder3-inputs-v156/%03d.frozen-input.json", index))
			if err != nil {
				t.Fatal(err)
			}
			var input model.FrozenBattleInput
			if err = json.Unmarshal(raw, &input); err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(input)
			if err = validateP6PlunderScope(&input); err != nil {
				t.Fatal("independent structural type3 scope", err)
			}
			out, err := s.ComputeP6PlunderSettlement(t.Context(), &input)
			if err != nil || out == nil || !out.Outcome.Authoritative || out.Outcome.CapabilityHash != constUtil.P6PlunderNativeProof {
				t.Fatal("certified type3 rejected", err)
			}
			if !p6KnownPlunderSettlementProof(&input, out.Outcome.CapabilityHash) || out.Outcome.CapabilityHash == constUtil.P6ActualArenaNativeProof {
				t.Fatal("borrowed Arena authority")
			}
			after, _ := json.Marshal(input)
			if string(before) != string(after) {
				t.Fatal("source actors/skills/Max20 changed")
			}
			bad, err := cloneP6CalculationInput(&input)
			if err != nil {
				t.Fatal(err)
			}
			bad.MaxRound = 40
			if _, err = s.ComputeP6PlunderSettlement(t.Context(), bad); err == nil {
				t.Fatal("original fixed Max20 widened")
			}
			for _, mode := range []int32{2, 11, 17, 20} {
				bad.BattleType = mode
				if _, err = s.ComputeP6PlunderSettlement(t.Context(), bad); err == nil {
					t.Fatal("other wrapper accepted")
				}
			}
			bad, err = cloneP6CalculationInput(&input)
			if err != nil {
				t.Fatal(err)
			}
			bad.Attacker.Formation[0].SkillOrder = nil
			if _, err = s.ComputeP6PlunderSettlement(t.Context(), bad); err == nil {
				t.Fatal("original three skills stripped")
			}
		})
	}
}
