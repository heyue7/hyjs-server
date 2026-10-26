package service

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p6ActualTyped17Input(t *testing.T) *model.FrozenBattleInput {
	t.Helper()
	raw, err := readTestAnalysis75("../../analysis/p6/native-local-p14-type17-qualified-input-v119/000.pb")
	if err != nil {
		t.Fatal(err)
	}
	wire := &pb.SCFightPlayback{}
	if err = proto.Unmarshal(raw, wire); err != nil {
		t.Fatal(err)
	}
	input, err := p6OriginalTables(t).FreezeBattleInput(model.FrozenBattleInput{Attacker: wire.Attacker, Defender: wire.Defender1, BattleType: 17, Seed: wire.GetRandSeed(), MaxRound: 40, AttackerDamageFactor: wire.AttackerFactor, DefenderDamageFactor: wire.DefenderFactor})
	if err != nil {
		t.Fatal(err)
	}
	return input
}

func TestP6Typed17AutomaticOriginalPairProtocolKeepsGenericAndBracketClosed(t *testing.T) {
	s := p6OriginalTables(t)
	input := p6ActualTyped17Input(t)
	if result, err := s.ComputeOriginalP6ProtocolFinalHP(t.Context(), input, nil); err == nil || result != nil {
		t.Fatal("generic recorded17 unexpectedly admitted")
	}
	actual, err := s.ComputeOriginalP6Typed17ProtocolFinalHP(t.Context(), input)
	if err != nil || actual == nil || actual.Outcome.Authoritative || actual.OriginalOperates == nil || len(actual.OriginalOperates.SkillOpts) != 0 || actual.OriginalRecordSHA256 != "" {
		t.Fatal("typed automatic audit boundary", err)
	}
	mutations := map[string]func(*model.FrozenBattleInput){
		"genericMode":     func(x *model.FrozenBattleInput) { x.BattleType = 2 },
		"recordedBracket": func(x *model.FrozenBattleInput) { x.KFReplay = &pb.KFReplayData{} },
		"wrongView":       func(x *model.FrozenBattleInput) { x.IsAttToDef = true },
		"wrongFallback":   func(x *model.FrozenBattleInput) { x.DefenderFirst = true },
		"wrongFactor":     func(x *model.FrozenBattleInput) { x.AttackerDamageFactor = proto.Float64(0) },
		"wrongRound":      func(x *model.FrozenBattleInput) { x.MaxRound = 41 },
		"driverOverride":  func(x *model.FrozenBattleInput) { x.DiagnosticAIDriverPolicy = "not-original" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			copy, err := cloneP6CalculationInput(input)
			if err != nil {
				t.Fatal(err)
			}
			mutate(copy)
			if result, err := s.ComputeOriginalP6Typed17ProtocolFinalHP(t.Context(), copy); err == nil || result != nil {
				t.Fatal("unsupported typed scope succeeded")
			}
		})
	}
}
