package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	"testing"
)

func TestP12FieldPVPScopeCurrentHPBothInitiativesAndColdReceipt(t *testing.T) {
	s := p6OriginalTables(t)
	for _, row := range p12FieldPVPNativeCases(t) {
		t.Run(row.Name, func(t *testing.T) {
			wire := p12FieldPVPNativeTypedInput(t, row)
			input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: wire.Attacker, Defender: wire.Defender1, Seed: wire.GetRandSeed(), MaxRound: 40, BattleType: 5, DefenderFirst: wire.Attacker.GetInst().GetFirstOrder() == 0 && wire.Defender1.GetInst().GetFirstOrder() == 1, AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1)})
			if err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(input)
			out, err := s.ComputeP12FieldPVPProtocolFinalHP(t.Context(), input)
			if err != nil {
				t.Fatal(err)
			}
			if err = ValidateP12FieldPVPCertifiedReceipt(input, out); err != nil {
				t.Fatal(err)
			}
			if !out.Outcome.Authoritative || len(out.FinalActors) != 2 || len(out.OriginalOperates.SkillOpts) != 0 {
				t.Fatal("original field terminal denominator/record")
			}
			raw, err := json.Marshal(out)
			if err != nil {
				t.Fatal(err)
			}
			for _, mode := range []string{"hp", "uid", "source", "proof"} {
				var cold model.CombatFinalHPOutcome
				if json.Unmarshal(raw, &cold) != nil {
					t.Fatal("cold decode")
				}
				switch mode {
				case "hp":
					cold.FinalActors[0].CurrentHP++
				case "uid":
					cold.FinalActors[0].UID = "foreign"
				case "source":
					cold.CalculationSourceSHA256 = ""
				case "proof":
					cold.Outcome.CapabilityHash = "other-mode-proof"
				}
				if ValidateP12FieldPVPCertifiedReceipt(input, &cold) == nil {
					t.Fatal("tampered field receipt accepted", mode)
				}
			}
			after, _ := json.Marshal(input)
			if string(before) != string(after) {
				t.Fatal("eligibility normalization changed actual HP/AP/initiative")
			}
			if _, err = s.ComputeOriginalP6ProtocolFinalHP(t.Context(), input, nil); err == nil {
				t.Fatal("generic mode5 authority expanded")
			}
		})
	}
}
func TestP12FieldPVPScopeRejectsForeignModesAndEffects(t *testing.T) {
	s := p6OriginalTables(t)
	wire := p12FieldPVPNativeTypedInput(t, p12FieldPVPNativeCases(t)[0])
	input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: wire.Attacker, Defender: wire.Defender1, Seed: 7, MaxRound: 40, BattleType: 5, AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1)})
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"type14", "dead", "overflow-ap", "talent", "factor", "manual", "unproved-player", "foreign-tables"} {
		view, err := cloneP6CalculationInput(input)
		if err != nil {
			t.Fatal(err)
		}
		switch mode {
		case "type14":
			view.BattleType = 14
		case "dead":
			view.Defender.Formation[0].Inst.CurHp = proto.Float64(0)
		case "overflow-ap":
			view.Attacker.Inst.Anger = proto.Float64(301)
		case "talent":
			view.Attacker.Formation[0].TalentList = []string{"tal_3165"}
		case "factor":
			view.DefenderDamageFactor = proto.Float64(0)
		case "manual":
			view.DiagnosticManualAutoStartRoundTime = proto.Float64(1)
		case "unproved-player":
			view.Attacker.Formation[0].Id = proto.String("hero_001")
		case "foreign-tables":
			view.Tables["Hero"][0]["id"] = "foreign"
		}
		if _, err = p12FieldPVPLegacyProofForInput(view); err == nil {
			t.Fatal("unproved field family admitted", mode)
		}
	}
}
