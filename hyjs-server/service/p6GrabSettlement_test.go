package service

import (
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

func TestP6GrabPendingIndependentSourceAndNoAuthority(t *testing.T) {
	s := p6OriginalTables(t)
	raw, err := readTestAnalysis75("../../analysis/p6/original-limit20-strong-inputs-v180/000.frozen-input-no-tables.json")
	if err != nil {
		t.Fatal(err)
	}
	var pair model.FrozenBattleInput
	if json.Unmarshal(raw, &pair) != nil {
		t.Fatal("actual player pair")
	}
	source := &pb.SCGrab{AttPlayer: pair.Attacker, DefPlayer: pair.Defender, RandSeed: proto.Int64(pair.Seed), FightId: proto.String("readonly-wrapper19-candidate"), MaxRoundNum: proto.Int32(40)}
	before, _ := proto.MarshalOptions{Deterministic: true}.Marshal(source)
	input, err := s.FreezeP6GrabInput(source)
	if err != nil {
		t.Fatal(err)
	}
	if input.BattleType != 19 || input.MaxRound != 40 || input.DefenderFirst || !proto.Equal(input.Attacker, source.AttPlayer) || !proto.Equal(input.Defender, source.DefPlayer) {
		t.Fatal("original19 projection")
	}
	if err = validateP6GrabScope(input); err != nil {
		t.Fatal("candidate structural shape", err)
	}
	if constUtil.P6GrabNativeProof == "" {
		out, err := s.ComputeP6GrabSettlement(t.Context(), input)
		if err == nil || out != nil {
			t.Fatal("unobserved wrapper accepted winner")
		}
	}
	for _, proof := range []string{"", constUtil.P6StarterNativeProof, constUtil.P6ActualPartyNativeProof, constUtil.P6ActualArenaNativeProof} {
		if p6KnownGrabSettlementProof(input, proof) {
			t.Fatal("foreign certificate accepted")
		}
	}
	source.WinPlayerId = proto.Int64(source.AttPlayer.GetPlayerId())
	if out, err := s.FreezeP6GrabInput(source); err == nil || out != nil {
		t.Fatal("preset winner source accepted")
	}
	source.WinPlayerId = nil
	source.MaxRoundNum = nil
	fallback, err := s.FreezeP6GrabInput(source)
	if err != nil || fallback.MaxRound != 40 {
		t.Fatal("original max fallback", err)
	}
	source.MaxRoundNum = proto.Int32(40)
	after, _ := proto.MarshalOptions{Deterministic: true}.Marshal(source)
	if string(before) != string(after) {
		t.Fatal("source mutated")
	}
	input.BattleType = 2
	if validateP6GrabScope(input) == nil {
		t.Fatal("Arena interpreted as Grab")
	}
}

// Public mode19 is independent of source IDs and sampled seeds. Structural
// admission rejects all unobserved wrappers and effects before settlement.
func TestP6GrabCertifiedStructureAndTerminal(t *testing.T) {
	s := p6OriginalTables(t)
	for _, index := range []int{1, 3} {
		raw, err := readTestAnalysis75(fmt.Sprintf("../../analysis/p6/original-grab19-candidate-inputs-v183/%03d.SCGrab.pb", index))
		if err != nil {
			t.Fatal(err)
		}
		typed := &pb.SCGrab{}
		if proto.Unmarshal(raw, typed) != nil {
			t.Fatal("original typed source")
		}
		input, err := s.FreezeP6GrabInput(typed)
		if err != nil {
			t.Fatal(err)
		}
		if err = validateP6GrabScope(input); err != nil {
			t.Fatal("original structural family", index, err)
		}
		before, _ := json.Marshal(input)
		for _, kind := range []string{"mode", "rounds", "initiative", "zero-seed", "injured", "unknown-skill", "fractional", "view", "manual", "factor"} {
			changed, err := cloneP6CalculationInput(input)
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "mode":
				changed.BattleType = 20
			case "rounds":
				changed.MaxRound = 20
			case "initiative":
				changed.Attacker.Inst.FirstOrder = proto.Int32(0)
				changed.Defender.Inst.FirstOrder = proto.Int32(1)
				changed.DefenderFirst = true
			case "zero-seed":
				changed.Seed = 0
			case "injured":
				changed.Attacker.Formation[0].Inst.CurHp = proto.Float64(1)
			case "unknown-skill":
				changed.Attacker.Formation[0].SkillOrder = proto.String("unobserved")
			case "fractional":
				changed.Attacker.Formation[0].Prop.Att = proto.Float64(1.5)
			case "view":
				changed.IsAttToDef = true
			case "manual":
				changed.DiagnosticManualAutoStartRoundTime = proto.Float64(1)
			case "factor":
				changed.AttackerDamageFactor = proto.Float64(2)
			}
			if validateP6GrabScope(changed) == nil || p6KnownGrabSettlementProof(changed, constUtil.P6GrabNativeProof) {
				t.Fatal("unobserved scope accepted", index, kind)
			}
		}
		if constUtil.P6GrabNativeProof != "" {
			out, err := s.ComputeP6GrabSettlement(t.Context(), input)
			if err != nil || out == nil || out.Outcome == nil || !out.Outcome.Authoritative || !p6KnownGrabSettlementProof(input, out.Outcome.CapabilityHash) {
				t.Fatal("independent original19 settlement", index, err)
			}
			if err = validateP6GrabTerminal(input, out); err != nil {
				t.Fatal(err)
			}
			for _, kind := range []string{"winner", "HP", "source", "engine"} {
				encoded, _ := json.Marshal(out)
				var changed model.CombatFinalHPOutcome
				if json.Unmarshal(encoded, &changed) != nil {
					t.Fatal("terminal copy")
				}
				switch kind {
				case "winner":
					changed.Outcome.Winner = 1 - changed.Outcome.Winner
				case "HP":
					changed.FinalActors[0].CurrentHP += 1
				case "source":
					changed.CalculationSourceSHA256 = "foreign"
				case "engine":
					changed.EngineVersion = "foreign"
				}
				if validateP6GrabTerminal(input, &changed) == nil {
					t.Fatal("tampered terminal accepted", index, kind)
				}
			}
		}
		after, _ := json.Marshal(input)
		if string(before) != string(after) {
			t.Fatal("source mutated")
		}
	}
}
