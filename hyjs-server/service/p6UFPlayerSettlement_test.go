package service

import (
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"os"
	"testing"
)

func TestP6UFPlayerPendingIndependentFactorAndSource(t *testing.T) {
	s := p6OriginalTables(t)
	raw, err := os.ReadFile("../../analysis/p6/original-uf14-player-factor1-inputs-v191/001.SCUFFightRecordReplay.pb")
	if err != nil {
		t.Fatal(err)
	}
	typed := &pb.SCUFFightRecordReplay{}
	if proto.Unmarshal(raw, typed) != nil {
		t.Fatal("original typed14 projection")
	}
	before, _ := proto.MarshalOptions{Deterministic: true}.Marshal(typed)
	input, err := s.FreezeP6UFPlayerInput(typed)
	if err != nil || validateP6UFPlayerScope(input) != nil || input.BattleType != 14 || input.IsAttToDef || input.DefenderFirst || input.AttackerDamageFactor == nil || *input.AttackerDamageFactor != 1 || !proto.Equal(input.Attacker, typed.PlayerList[0]) || !proto.Equal(input.Defender, typed.PlayerList[1]) {
		t.Fatal("independent explicit-factor14 source", err)
	}
	if constUtil.P6UFPlayerNativeProof == "" {
		out, err := s.ComputeP6UFPlayerSettlement(t.Context(), input)
		if err == nil || out != nil {
			t.Fatal("unobserved14 accepted")
		}
	}
	for _, proof := range []string{"", constUtil.P6StarterNativeProof, constUtil.P6ActualPartyNativeProof, constUtil.P6GrabNativeProof, constUtil.P6LimitNativeProof, constUtil.P6LimitPartyNativeProof, constUtil.P6KFCanonicalNativeProof} {
		if p6KnownUFPlayerSettlementProof(input, proof) {
			t.Fatal("foreign capability accepted")
		}
	}
	for _, kind := range []string{"old-zero-factor", "partial-injury", "mode", "round", "opposite", "zerozero", "manual", "nil-factor", "fractional", "unknown-skill", "team-adds"} {
		changed, err := cloneP6CalculationInput(input)
		if err != nil {
			t.Fatal(err)
		}
		switch kind {
		case "old-zero-factor":
			changed.AttackerDamageFactor = proto.Float64(0)
		case "partial-injury":
			changed.Attacker.Formation[0].Inst.CurHp = proto.Float64(205.4)
		case "mode":
			changed.BattleType = 19
		case "round":
			changed.MaxRound = 5
		case "opposite":
			changed.Attacker.Inst.FirstOrder = proto.Int32(0)
			changed.Defender.Inst.FirstOrder = proto.Int32(1)
			changed.DefenderFirst = true
		case "zerozero":
			changed.Attacker.Inst.FirstOrder = proto.Int32(0)
		case "manual":
			changed.DiagnosticManualAutoStartRoundTime = proto.Float64(1)
		case "nil-factor":
			changed.AttackerDamageFactor = nil
		case "fractional":
			changed.Attacker.Formation[0].Prop.Att = proto.Float64(175.5)
		case "team-adds":
			changed.Attacker.Inst.ExtBattleAdds = []int32{1}
		case "unknown-skill":
			changed.Attacker.Formation[0].SkillOrder = proto.String("unobserved")
		}
		if validateP6UFPlayerScope(changed) == nil || p6KnownUFPlayerSettlementProof(changed, constUtil.P6UFPlayerNativeProof) {
			t.Fatal("old/unknown14 source admitted", kind)
		}
	}
	typed.WinPlayerId = proto.Int64(0)
	if frozen, err := s.FreezeP6UFPlayerInput(typed); err == nil || frozen != nil {
		t.Fatal("presetwinner pointer accepted")
	}
	typed.WinPlayerId = nil
	typed.RoomId = proto.String("other-map")
	if frozen, err := s.FreezeP6UFPlayerInput(typed); err == nil || frozen != nil {
		t.Fatal("unobserved room admitted")
	}
	typed.RoomId = nil
	after, _ := proto.MarshalOptions{Deterministic: true}.Marshal(typed)
	if string(before) != string(after) {
		t.Fatal("original source mutated")
	}
}

// New ordinary war calculations bind the exact typed pair and complete same-run
// terminal. These mathematical sources do not assert current war eligibility.
func TestP6UFPlayerCertifiedFamiliesAndTerminal(t *testing.T) {
	s := p6OriginalTables(t)
	for _, index := range []int{1, 2, 3, 4} {
		raw, err := os.ReadFile(fmt.Sprintf("../../analysis/p6/original-uf14-player-factor1-inputs-v191/%03d.SCUFFightRecordReplay.pb", index))
		if err != nil {
			t.Fatal(err)
		}
		typed := &pb.SCUFFightRecordReplay{}
		if proto.Unmarshal(raw, typed) != nil {
			t.Fatal("original explicitfactor14 typedpair")
		}
		input, err := s.FreezeP6UFPlayerInput(typed)
		if err != nil {
			t.Fatal(err)
		}
		before, _ := json.Marshal(input)
		out, err := s.ComputeP6UFPlayerSettlement(t.Context(), input)
		expectedWinner := int32(0)
		if index == 2 || index == 4 {
			expectedWinner = 1
		}
		expectedRound := map[int]int32{1: 15, 2: 16, 3: 3, 4: 4}[index]
		if err != nil || out == nil || out.Outcome == nil || !out.Outcome.Authoritative || out.Outcome.Winner != expectedWinner || out.Outcome.FinalRound != expectedRound || out.Outcome.CapabilityHash != constUtil.P6UFPlayerNativeProof || !p6KnownUFPlayerSettlementProof(input, out.Outcome.CapabilityHash) || validateP6UFPlayerTerminal(input, out) != nil {
			t.Fatal("independent ordinary14 capability", index, err)
		}
		for _, kind := range []string{"winner", "HP", "source", "engine"} {
			encoded, _ := json.Marshal(out)
			var changed model.CombatFinalHPOutcome
			if json.Unmarshal(encoded, &changed) != nil {
				t.Fatal("copy terminal")
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
			if validateP6UFPlayerTerminal(input, &changed) == nil {
				t.Fatal("tampered ordinary14 terminal accepted", index, kind)
			}
		}
		after, _ := json.Marshal(input)
		if string(before) != string(after) {
			t.Fatal("source mutated")
		}
	}
}
