package service

import (
	"encoding/json"
	"fmt"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
)

func TestP6LegacyPartyActualP5StructurePendingProofAndUnknownBranches(t *testing.T) {
	s := &GameService{}
	for index := 0; index < 8; index++ {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			raw, err := readTestAnalysis75(fmt.Sprintf("../../analysis/p6/actual-demo-readonly-branches-inputs-v140/%03d.frozen-input.json", index))
			if err != nil {
				t.Fatal(err)
			}
			var input model.FrozenBattleInput
			if err = json.Unmarshal(raw, &input); err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(input)
			if err = validateP6LegacyPartyScope(&input); err != nil {
				t.Fatal("actual retained P5 party", err)
			}
			diagnostic, err := s.ComputeOriginalP6ProtocolFinalHP(t.Context(), &input, nil)
			if err != nil || diagnostic.Outcome.Authoritative {
				t.Fatal("actual diagnostic calculation", err)
			}
			if err = validateP6LegacyPartyTerminal(&input, diagnostic); err != nil {
				t.Fatal("original observed short normal branch", err)
			}
			out, err := s.ComputeP6LegacyPartySettlement(t.Context(), &input)
			if constUtil.P6LegacyPartyNativeProof == "" {
				if err == nil || out != nil {
					t.Fatal("unproven actual party opened")
				}
			} else if validateP6LegacyPartyTerminal(&input, diagnostic) == nil && (err != nil || !out.Outcome.Authoritative) {
				t.Fatal("certified supported branch rejected", err)
			}
			after, _ := json.Marshal(input)
			if string(before) != string(after) {
				t.Fatal("retained actual skill/talent/gear/panel/GUID changed")
			}
			// Full calculations keep the ultimates; a future execution must deny the
			// transaction rather than removing the original skill or faking a result.
			event := diagnostic.Outcome.Events[0]
			originalEvents := diagnostic.Outcome.Events
			event.Skill = "001_3"
			event.Kind = "skill_begin"
			diagnostic.Outcome.Events = append(append(originalEvents[:0:0], originalEvents...), event)
			if validateP6LegacyPartyTerminal(&input, diagnostic) == nil {
				t.Fatal("unobserved ultimate accepted")
			}
			event.Skill = ""
			event.Buff = "buff_1"
			diagnostic.Outcome.Events = append(append(originalEvents[:0:0], originalEvents...), event)
			if validateP6LegacyPartyTerminal(&input, diagnostic) == nil {
				t.Fatal("unobserved talent effect accepted")
			}
			for _, change := range []func(*model.FrozenBattleInput){
				func(v *model.FrozenBattleInput) { v.Attacker.Formation[0].SkillOrder = proto.String("001_1,001_2") },
				func(v *model.FrozenBattleInput) { v.Attacker.Formation[1].Pose = proto.Int32(6) },
				func(v *model.FrozenBattleInput) { v.Attacker.Formation[1].TalentList = []string{"tal_2"} },
				func(v *model.FrozenBattleInput) { v.Attacker.Inst.ExtBattleAdds = []int32{1} },
				func(v *model.FrozenBattleInput) { v.Attacker.Formation[0].Prop.Perhp = proto.Float64(1) },
			} {
				copy, err := cloneP6CalculationInput(&input)
				if err != nil {
					t.Fatal(err)
				}
				change(copy)
				if validateP6LegacyPartyScope(copy) == nil {
					t.Fatal("unknown actor/skill/pose/talent/factor admitted")
				}
			}
		})
	}
}
