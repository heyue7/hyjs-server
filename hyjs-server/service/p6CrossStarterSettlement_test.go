package service

import (
	"context"
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
)

func TestP6CrossStarterIndependentScopeAndPendingProof(t *testing.T) {
	service := &GameService{}
	for _, name := range []string{"000", "001", "002"} {
		raw, err := readTestAnalysis75("../../analysis/p6/actual-qualified-prayer-type17-inputs-v129/" + name + ".frozen-input.json")
		if err != nil {
			t.Fatal(err)
		}
		var input model.FrozenBattleInput
		if err = json.Unmarshal(raw, &input); err != nil {
			t.Fatal(err)
		}
		before, _ := json.Marshal(input)
		if err = validateP6CrossStarterScope(&input); err != nil {
			t.Fatal("actual qualified source rejected", err)
		}
		out, err := service.ComputeOriginalP6Typed17ProtocolFinalHP(context.Background(), &input)
		if err != nil || out.Outcome.Authoritative || validateP6CrossStarterTerminal(&input, out) != nil {
			t.Fatal("actual diagnostic terminal", err)
		}
		settled, err := service.ComputeP6CrossStarterSettlement(context.Background(), &input)
		if constUtil.P6CrossStarterNativeProof == "" {
			if err == nil || settled != nil {
				t.Fatal("unproven17 family opened")
			}
		} else if err != nil || !settled.Outcome.Authoritative {
			t.Fatal("independent certified17 family rejected", err)
		}
		after, _ := json.Marshal(input)
		if string(before) != string(after) {
			t.Fatal("source wrapper/initiative mutated")
		}
		for _, change := range []func(*model.FrozenBattleInput){func(v *model.FrozenBattleInput) { v.BattleType = 11 }, func(v *model.FrozenBattleInput) { v.Attacker.Inst.FirstOrder = proto.Int32(1) }, func(v *model.FrozenBattleInput) { v.Attacker.Inst.ExtBattleAdds = []int32{1} }, func(v *model.FrozenBattleInput) { v.Defender.PlayerId = v.Attacker.PlayerId }, func(v *model.FrozenBattleInput) { v.Defender.Formation[0].TalentList = []string{"tal_1"} }, func(v *model.FrozenBattleInput) { v.IsAttToDef = true }, func(v *model.FrozenBattleInput) {
			v.Defender.Formation = append(v.Defender.Formation, v.Defender.Formation[0])
		}} {
			copy, err := cloneP6CalculationInput(&input)
			if err != nil {
				t.Fatal(err)
			}
			change(copy)
			if validateP6CrossStarterScope(copy) == nil {
				t.Fatal("unknown source/mode/initiative/extra fields admitted")
			}
		}
	}
}
