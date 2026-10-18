package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	"testing"
)

func p12WantedOriginalFixture(t *testing.T) (*GameService, *model.FrozenBattleInput) {
	t.Helper()
	s := p6OriginalTables(t)
	wire := p12WantedNativeTypedInput(t, p12WantedNativeCases(t)[0])
	input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: wire.Attacker, Monster: wire.Defender2, Seed: wire.GetRandSeed(), MaxRound: 20, BattleType: 8, AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1)})
	if err != nil {
		t.Fatal(err)
	}
	return s, input
}
func TestP12WantedIndependentTerminalSourceColdReceipt(t *testing.T) {
	s, input := p12WantedOriginalFixture(t)
	before, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := P12WantedProofForInput(input)
	if err != nil || proof == "" {
		t.Fatal("independent proof", err)
	}
	out, err := s.ComputeP12WantedProtocolFinalHP(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Outcome.Authoritative || out.Outcome.CapabilityHash != proof || len(out.FinalActors) != 6 || out.OriginalOperates == nil {
		t.Fatal("complete independent terminal authority")
	}
	if err = ValidateP12WantedCertifiedReceipt(input, out); err != nil {
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
		if ValidateP12WantedCertifiedReceipt(input, &restored) == nil {
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
func TestP12WantedScopeRejectsUnprovedMechanisms(t *testing.T) {
	_, input := p12WantedOriginalFixture(t)
	for _, mode := range []string{"other-mode", "wounded", "anger", "talent", "weapon", "awake", "npc", "factor", "manual", "all-npc-dead", "duplicate-guid", "tables", "negative-seed", "noninteger-panel"} {
		view, err := cloneP6CalculationInput(input)
		if err != nil {
			t.Fatal(err)
		}
		switch mode {
		case "other-mode":
			view.BattleType = 16
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
			view.AttackerDamageFactor = proto.Float64(3)
		case "manual":
			view.DiagnosticManualAutoStartRoundTime = proto.Float64(1)
		case "all-npc-dead":
			for _, actor := range view.Monster.Formation {
				actor.Inst.CurHp = proto.Float64(0)
			}
		case "duplicate-guid":
			view.Attacker.Formation[1].Guid = proto.String(view.Attacker.Formation[0].GetGuid())
		case "tables":
			delete(view.Tables, "WantedEnemy")
		case "negative-seed":
			view.Seed = -1
		case "noninteger-panel":
			view.Attacker.Formation[0].Prop.Att = proto.Float64(700.5)
		}
		if _, err = p12WantedLegacyProofForInput(view); err == nil {
			t.Fatal("unproved mechanism admitted", mode)
		}
	}
}

// Growth changes and account-owned GUIDs remain inside the proved mechanism family.
func TestP12WantedScopeAdmitsLegitimateGrowthAndAccountIdentity(t *testing.T) {
	_, input := p12WantedOriginalFixture(t)
	input.Attacker.PlayerId = proto.Int64(999001)
	input.Attacker.ServerId = proto.Int32(2)
	for i, actor := range input.Attacker.Formation {
		actor.Guid = proto.String([]string{"owned-player-main", "owned-player-partner"}[i])
		actor.Level = proto.Int32(35 + int32(i))
		actor.Prop.Att = proto.Float64(actor.Prop.GetAtt() + 11)
		actor.Prop.Hp = proto.Float64(actor.Prop.GetHp() + 101)
		actor.Prop.Pdef = proto.Float64(actor.Prop.GetPdef() + 3)
		actor.Prop.Mdef = proto.Float64(actor.Prop.GetMdef() + 5)
		actor.Inst.CurHp = proto.Float64(actor.Prop.GetHp())
	}
	for _, factor := range []float64{1, 2.5} {
		input.AttackerDamageFactor = proto.Float64(factor)
		if _, err := P12WantedProofForInput(input); err != nil {
			t.Fatal("legitimate growth/account identity rejected", err)
		}
	}
}

func TestP12WantedScopeIndependentPartialHPProof(t *testing.T) {
	_, input := p12WantedOriginalFixture(t)
	full, err := P12WantedProofForInput(input)
	if err != nil {
		t.Fatal(err)
	}
	input.Monster.Formation[0].Inst.CurHp = proto.Float64(0)
	input.Monster.Formation[1].Inst.CurHp = proto.Float64(21.125)
	partial, err := P12WantedProofForInput(input)
	if err != nil || partial == full || partial == "" {
		t.Fatal("partial must carry its independent type8 evidence", err)
	}
	input.Monster.Formation[1].SkillOrder = proto.String("foreign")
	if _, err = P12WantedProofForInput(input); err == nil {
		t.Fatal("partial proof altered original NPC skills")
	}
}
