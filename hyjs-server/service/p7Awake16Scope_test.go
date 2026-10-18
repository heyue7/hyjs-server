package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	"testing"
)

func p7Awake16OriginalFixture(t *testing.T) (*GameService, *model.FrozenBattleInput) {
	t.Helper()
	s := p6OriginalTables(t)
	wire := p7AwakeNativeTypedInput(t, p7AwakeNativeCases(t)[0])
	input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: wire.Attacker, Monster: wire.Defender2, Seed: wire.GetRandSeed(), MaxRound: 40, BattleType: 16, AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1)})
	if err != nil {
		t.Fatal(err)
	}
	return s, input
}
func TestP7Awake16IndependentTerminalSourceColdReceipt(t *testing.T) {
	s, input := p7Awake16OriginalFixture(t)
	before, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := P7Awake16ProofForInput(input)
	if err != nil || proof == "" {
		t.Fatal("independent proof", err)
	}
	out, err := s.ComputeP7Awake16ProtocolFinalHP(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Outcome.Authoritative || out.Outcome.CapabilityHash != proof || len(out.FinalActors) != 4 || out.OriginalOperates == nil {
		t.Fatal("complete independent terminal authority")
	}
	if err = ValidateP7Awake16CertifiedReceipt(input, out); err != nil {
		t.Fatal(err)
	}
	saved, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"hp", "guid", "initial-source", "proof", "record-total"} {
		var restored model.CombatFinalHPOutcome
		if json.Unmarshal(saved, &restored) != nil {
			t.Fatal("cold decode")
		}
		switch mode {
		case "hp":
			restored.FinalActors[0].CurrentHP++
		case "guid":
			restored.FinalActors[0].GUID += "foreign"
		case "initial-source":
			restored.FinalActors[0].InitialActorSHA256 = ""
		case "proof":
			restored.Outcome.CapabilityHash = "foreign"
		case "record-total":
			restored.OriginalOperates.AttackerTotalLeftHp = proto.Float64(restored.OriginalOperates.GetAttackerTotalLeftHp() + 1)
		}
		if ValidateP7Awake16CertifiedReceipt(input, &restored) == nil {
			t.Fatal("tampered receipt accepted", mode)
		}
	}
	after, _ := json.Marshal(input)
	if string(after) != string(before) {
		t.Fatal("adapter mutated original frozen input")
	}
	if _, err = s.ComputeOriginalP6ProtocolFinalHP(t.Context(), input, nil); err == nil {
		t.Fatal("generic original protocol gate expanded")
	}
}
func TestP7Awake16ScopeRejectsUnprovedMechanisms(t *testing.T) {
	_, input := p7Awake16OriginalFixture(t)
	for _, mode := range []string{"other-mode", "wounded", "anger", "talent", "weapon", "awake", "npc", "factor", "manual", "negative-seed", "noninteger-panel"} {
		view, err := cloneP6CalculationInput(input)
		if err != nil {
			t.Fatal(err)
		}
		switch mode {
		case "other-mode":
			view.BattleType = 18
		case "wounded":
			view.Attacker.Formation[0].Inst.CurHp = proto.Float64(view.Attacker.Formation[0].Inst.GetCurHp() - 1)
		case "anger":
			view.Attacker.Inst.Anger = proto.Float64(1)
		case "talent":
			view.Attacker.Formation[0].TalentList = []string{"tal_3165"}
		case "weapon":
			view.Attacker.Formation[0].WeaponId = proto.String("foreign")
		case "awake":
			view.Attacker.Formation[0].AwakeStage = proto.Int32(1)
		case "npc":
			view.Monster.Formation[0].Prop.Att = proto.Float64(view.Monster.Formation[0].Prop.GetAtt() + 1)
		case "factor":
			view.AttackerDamageFactor = proto.Float64(2.5)
		case "manual":
			view.DiagnosticManualAutoStartRoundTime = proto.Float64(1)
		case "negative-seed":
			view.Seed = -1
		case "noninteger-panel":
			view.Attacker.Formation[0].Prop.Att = proto.Float64(700.5)
		}
		if _, err = P7Awake16ProofForInput(view); err == nil {
			t.Fatal("unproved mechanism admitted", mode)
		}
	}
}
