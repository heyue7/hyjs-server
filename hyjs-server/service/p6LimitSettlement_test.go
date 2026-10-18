package service

import (
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"os"
	"testing"
)

func TestP6LimitPendingIndependentProof(t *testing.T) {
	s := p6OriginalTables(t)
	raw, err := os.ReadFile("../../analysis/p6/original-limit20-inputs-v172/000.frozen-input-no-tables.json")
	if err != nil {
		t.Fatal(err)
	}
	var source model.FrozenBattleInput
	if json.Unmarshal(raw, &source) != nil {
		t.Fatal("actual current Limit query source")
	}
	source.Tables = s.Tables
	input, err := s.FreezeBattleInput(source)
	if err != nil {
		t.Fatal(err)
	}
	if err = validateP6LimitScope(input); err != nil {
		t.Fatal("actual source scope", err)
	}
	if constUtil.P6LimitNativeProof == "" {
		out, err := s.ComputeP6LimitSettlement(t.Context(), input)
		if err == nil || out != nil {
			t.Fatal("empty Limit proof admitted winner")
		}
	}
	for _, proof := range []string{"", constUtil.P6StarterNativeProof, constUtil.P6CrossStarterNativeProof} {
		if p6KnownLimitSettlementProof(input, proof) {
			t.Fatal("foreign mode capability admitted")
		}
	}
	input.BattleType = 2
	if validateP6LimitScope(input) == nil {
		t.Fatal("Arena reclassified Limit")
	}
}

// The same public type20 family supports actual FindMatch strong/weak sources
// and validates their full original terminal before any consumer applies cost.
func TestP6LimitCertifiedActualStrongWeakAndUnknown(t *testing.T) {
	s := p6OriginalTables(t)
	for index, winner := range []int32{0, 1} {
		raw, err := os.ReadFile(fmt.Sprintf("../../analysis/p6/original-limit20-strong-inputs-v180/%03d.frozen-input-no-tables.json", index))
		if err != nil {
			t.Fatal(err)
		}
		var source model.FrozenBattleInput
		if json.Unmarshal(raw, &source) != nil {
			t.Fatal("actual frozen source")
		}
		source.Tables = s.Tables
		input, err := s.FreezeBattleInput(source)
		if err != nil {
			t.Fatal(err)
		}
		before, _ := json.Marshal(input)
		out, err := s.ComputeP6LimitSettlement(t.Context(), input)
		if err != nil || out == nil {
			t.Fatal("public Limit20", index, err)
		}
		if !out.Outcome.Authoritative || out.Outcome.Winner != winner || !p6KnownLimitSettlementProof(input, out.Outcome.CapabilityHash) {
			t.Fatal("independent Limit20 capability", index)
		}
		if out.Outcome.FinalRound != int32(15+index) {
			t.Fatal("actual original terminal round", out.Outcome.FinalRound)
		}
		if err = validateP6LimitTerminal(input, out); err != nil {
			t.Fatal(err)
		}
		for _, kind := range []string{"arena", "other-skill", "fractional-panel", "injured", "awake", "manual", "view", "factor"} {
			changed, err := cloneP6CalculationInput(input)
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "arena":
				changed.BattleType = 2
			case "other-skill":
				changed.Attacker.Formation[0].SkillOrder = proto.String("047_1,047_2,unknown")
			case "fractional-panel":
				changed.Attacker.Formation[0].Prop.Att = proto.Float64(175.5)
			case "injured":
				changed.Attacker.Formation[0].Inst.CurHp = proto.Float64(1)
			case "awake":
				changed.Attacker.Formation[0].AwakeStage = proto.Int32(1)
			case "manual":
				changed.DiagnosticManualAutoStartRoundTime = proto.Float64(1)
			case "view":
				changed.IsAttToDef = true
			case "factor":
				changed.AttackerDamageFactor = proto.Float64(2)
			}
			if validateP6LimitScope(changed) == nil || p6KnownLimitSettlementProof(changed, out.Outcome.CapabilityHash) {
				t.Fatal("unknown input certified", kind)
			}
		}
		after, _ := json.Marshal(input)
		if string(before) != string(after) {
			t.Fatal("source mutated")
		}
	}
}
