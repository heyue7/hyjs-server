package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"testing"
)

func TestP6WantedOriginalAlternateSkillsAreEnemyAndModeSpecific(t *testing.T) {
	s := p6OriginalTables(t)
	row := p12WantedNativeCases(t)[0]
	wire := p12WantedNativeTypedInput(t, row)
	input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: wire.Attacker, Monster: wire.Defender2, Seed: wire.GetRandSeed(), MaxRound: 20, BattleType: 8, AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1)})
	if err != nil {
		t.Fatal(err)
	}
	if len(input.Tables["WantedEnemy"]) == 0 || input.ConfigurationHash == constUtil.P6LinearPhysicalTables {
		t.Fatal("original Wanted source omitted from exact frozen configuration")
	}
	out, err := s.computeFrozenBattleFinalHP(t.Context(), input, nil, true)
	if err != nil || out.Outcome.Authoritative {
		t.Fatal("original Wanted skill source is unavailable or globally authorized", err)
	}
	if _, err = s.ComputeOriginalP6ProtocolFinalHP(t.Context(), input, nil); err == nil {
		t.Fatal("generic original mode8 gate expanded")
	}
	foreign, err := cloneP6CalculationInput(input)
	if err != nil {
		t.Fatal(err)
	}
	foreign.BattleType = 16
	if _, err = s.computeFrozenBattleFinalHP(t.Context(), foreign, nil, true); err == nil {
		t.Fatal("Wanted alternate NPC skills leaked to another mode")
	}
	// Reading the new source table must leave established mode16 table closure unchanged.
	awake := p7AwakeNativeTypedInput(t, p7AwakeNativeCases(t)[0])
	old, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: awake.Attacker, Monster: awake.Defender2, Seed: 7, MaxRound: 40, BattleType: 16, AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1)})
	if err != nil || old.ConfigurationHash != constUtil.P6LinearPhysicalTables || len(old.Tables["WantedEnemy"]) != 0 {
		t.Fatal("unrelated original table closure drift", err)
	}
}
