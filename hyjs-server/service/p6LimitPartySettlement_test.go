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

// The new original full-party mode20 capability must leave every already
// accepted single047 proof and frozen terminal readable under its old family.
func TestP6LimitPartyIndependentProofPreservesHistorical047(t *testing.T) {
	s := p6OriginalTables(t)
	raw, err := os.ReadFile("../../analysis/p6/original-limit20-strong-inputs-v180/000.frozen-input-no-tables.json")
	if err != nil {
		t.Fatal(err)
	}
	var old model.FrozenBattleInput
	if json.Unmarshal(raw, &old) != nil {
		t.Fatal("actual historical source")
	}
	old.Tables = s.Tables
	frozen, err := s.FreezeBattleInput(old)
	if err != nil || validateP6LimitScope(frozen) != nil || !p6KnownLimitSettlementProof(frozen, constUtil.P6LimitNativeProof) {
		t.Fatal("historical047 capability changed", err)
	}
	raw, err = os.ReadFile("../../analysis/p6/original-full-party-limit20-inputs-v187/000.SCLimitPvpFightResult.pb")
	if err != nil {
		t.Fatal(err)
	}
	typed := &pb.SCLimitPvpFightResult{}
	if proto.Unmarshal(raw, typed) != nil || len(typed.PlayerList) != 2 {
		t.Fatal("actual full original party source")
	}
	input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: typed.PlayerList[0], Defender: typed.PlayerList[1], Seed: typed.GetRandSeed(), MaxRound: typed.GetMaxRoundNum(), BattleType: 20, AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1)})
	if err != nil || validateP6LimitScope(input) != nil {
		t.Fatal("new independent20 shape", err)
	}
	if p6KnownLimitSettlementProof(input, constUtil.P6LimitNativeProof) {
		t.Fatal("newparty borrowed historical047 proof")
	}
	if constUtil.P6LimitPartyNativeProof == "" {
		out, err := s.ComputeP6LimitSettlement(t.Context(), input)
		if err == nil || out != nil {
			t.Fatal("unobserved party received authority")
		}
	} else {
		out, err := s.ComputeP6LimitSettlement(t.Context(), input)
		if err != nil || out == nil || out.Outcome == nil || !out.Outcome.Authoritative || !p6KnownLimitSettlementProof(input, out.Outcome.CapabilityHash) || out.Outcome.CapabilityHash != constUtil.P6LimitPartyNativeProof || validateP6LimitTerminal(input, out) != nil {
			t.Fatal("independentparty terminal", err)
		}
		if p6KnownLimitSettlementProof(frozen, constUtil.P6LimitPartyNativeProof) {
			t.Fatal("newproof rewrote historical047 capability")
		}
	}
	for _, kind := range []string{"mode", "short", "opposite", "missing-ultimate", "mixed-size", "other-talent", "other-buff", "fractional", "hurt", "manual"} {
		changed, err := cloneP6CalculationInput(input)
		if err != nil {
			t.Fatal(err)
		}
		switch kind {
		case "mode":
			changed.BattleType = 19
		case "short":
			changed.MaxRound = 5
		case "opposite":
			changed.Attacker.Inst.FirstOrder = proto.Int32(0)
			changed.Defender.Inst.FirstOrder = proto.Int32(1)
			changed.DefenderFirst = true
		case "missing-ultimate":
			changed.Attacker.Formation[0].SkillOrder = proto.String("001_1,001_2")
		case "mixed-size":
			changed.Defender.Formation = changed.Defender.Formation[:1]
		case "other-talent":
			changed.Attacker.Formation[0].TalentList = []string{"unobserved"}
		case "other-buff":
			changed.Attacker.Formation[0].Prop.DamageAdd = proto.Float64(1)
		case "fractional":
			changed.Attacker.Formation[0].Prop.Att = proto.Float64(1.5)
		case "hurt":
			changed.Attacker.Formation[0].Inst.CurHp = proto.Float64(1)
		case "manual":
			changed.DiagnosticManualAutoStartRoundTime = proto.Float64(1)
		}
		if validateP6LimitScope(changed) == nil || p6KnownLimitSettlementProof(changed, constUtil.P6LimitPartyNativeProof) {
			t.Fatal("unobserved fullparty shape admitted", kind)
		}
	}
}

// Public results keep independent typed20 evidence and the exact original full
// actor/skill source. Engine-only mirrors do not become real match candidates.
func TestP6LimitPartyCertifiedActualDirectionsAndTerminal(t *testing.T) {
	s := p6OriginalTables(t)
	for index := 0; index < 2; index++ {
		raw, err := os.ReadFile(fmt.Sprintf("../../analysis/p6/original-full-party-limit20-inputs-v187/%03d.SCLimitPvpFightResult.pb", index))
		if err != nil {
			t.Fatal(err)
		}
		typed := &pb.SCLimitPvpFightResult{}
		if proto.Unmarshal(raw, typed) != nil || len(typed.PlayerList) != 2 {
			t.Fatal("actual direction typed source")
		}
		input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: typed.PlayerList[0], Defender: typed.PlayerList[1], Seed: typed.GetRandSeed(), MaxRound: typed.GetMaxRoundNum(), BattleType: 20, AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1)})
		if err != nil {
			t.Fatal(err)
		}
		before, _ := json.Marshal(input)
		out, err := s.ComputeP6LimitSettlement(t.Context(), input)
		if err != nil || out == nil || out.Outcome == nil || !out.Outcome.Authoritative || out.Outcome.Winner != int32(index) || out.Outcome.FinalRound != int32(3+index) || out.Outcome.CapabilityHash != constUtil.P6LimitPartyNativeProof || !p6KnownLimitSettlementProof(input, out.Outcome.CapabilityHash) || validateP6LimitTerminal(input, out) != nil {
			t.Fatal("independent type20 fullparty result", index, err)
		}
		for _, kind := range []string{"winner", "HP", "source", "engine"} {
			encoded, _ := json.Marshal(out)
			var changed model.CombatFinalHPOutcome
			if json.Unmarshal(encoded, &changed) != nil {
				t.Fatal("copyterminal")
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
			if validateP6LimitTerminal(input, &changed) == nil {
				t.Fatal("changed type20 terminal admitted", kind)
			}
		}
		after, _ := json.Marshal(input)
		if string(before) != string(after) {
			t.Fatal("source mutated")
		}
	}
}
