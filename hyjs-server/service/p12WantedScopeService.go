package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"strings"
)

// validateP12WantedScope binds the original type8 normal001003 mechanism family.
// Legitimate actor GUIDs, levels and integer growth panels remain account-owned.
func validateP12WantedScope(input *model.FrozenBattleInput) error {
	reject := func() error { return errors.New(constUtil.CombatUnsupported) }
	if input == nil || input.BattleType != 8 || input.MaxRound != 20 || input.Attacker == nil || input.Attacker.Inst == nil || input.Monster == nil || input.Monster.Inst == nil || input.Defender != nil || input.KFReplay != nil || input.KFReplayViewer != nil || len(input.WinConditions) != 0 || input.DefenderFirst || input.IsAttToDef || input.PVECompatibilityRule != "" || input.DiagnosticAIDriverPolicy != "" || input.DiagnosticManualAutoStartRoundTime != nil {
		return reject()
	}
	if input.EngineVersion != constUtil.P6LinearPhysicalEngine || input.ClientVersion != constUtil.P6OriginalClientHash || input.ConfigurationHash != constUtil.P12WantedConfigurationHash || input.Seed < 0 || input.Seed > math.MaxInt32 {
		return reject()
	}
	if input.AttackerDamageFactor == nil || (*input.AttackerDamageFactor != 1 && *input.AttackerDamageFactor != 2.5) || input.DefenderDamageFactor == nil || *input.DefenderDamageFactor != 1 {
		return reject()
	}
	party := input.Attacker
	if party.GetPlayerId() <= 0 || party.GetServerId() <= 0 || len(party.Formation) != 2 || party.Inst.GetAnger() != 0 || party.Inst.GetFirstOrder() != 0 || len(party.Inst.ExtBattleAdds) != 0 || p6ScopedUnknown(party) || p6ScopedUnknown(party.Inst) {
		return reject()
	}
	seen := map[string]bool{}
	talents := []string{}
	for index, actor := range party.Formation {
		id := []string{"001", "003"}[index]
		pose := []int32{3, 7}[index]
		if actor == nil || actor.Prop == nil || actor.Inst == nil || actor.GetId() != "hero_"+id || actor.GetSkillOrder() != id+"_1,"+id+"_2,"+id+"_3" || actor.GetPose() != pose || actor.GetBreachedLevel() != 1 || actor.GetBloodNumber() != 1 || actor.GetSkillCastChance() != 100 || actor.GetLevel() < 1 || actor.GetLevel() > 100 || actor.GetAwakeStage() != 0 || actor.GetAwakenSkill() != "" || actor.GetWeaponId() != "" || len(actor.TalentList) != len(talents) || p6ScopedUnknown(actor) || p6ScopedUnknown(actor.Prop) || p6ScopedUnknown(actor.Inst) {
			return reject()
		}
		for index, talent := range talents {
			if actor.TalentList[index] != talent {
				return reject()
			}
		}
		if actor.GetGuid() == "" || len(actor.GetGuid()) > 128 || strings.Contains(actor.GetGuid(), "_") || seen[actor.GetGuid()] {
			return reject()
		}
		seen[actor.GetGuid()] = true
		p := actor.Prop
		for _, value := range []float64{p.GetAtt(), p.GetHp(), p.GetPdef(), p.GetMdef()} {
			if !combatFinite(value) || value < 0 || value > constUtil.P6LinearPhysicalPanelLimit || math.Trunc(value) != value {
				return reject()
			}
		}
		hit, critical := 95.0, 10.0
		if p.GetAtt() < 1 || p.GetHp() < 1 || actor.Inst.GetCurHp() != p.GetHp() || p.GetPeratt() != 0 || p.GetPerhp() != 0 || p.GetPerpdef() != 0 || p.GetPermdef() != 0 || p.GetDamageAdd() != 0 || p.GetDamageReduce() != 0 || p.GetHit() != hit || p.GetDodge() != 5 || p.GetCri() != critical || p.GetAntiCri() != 0 || p.GetCriFactor() != 50 {
			return reject()
		}
	}
	// Original WantedID1 retains all four NPC panels, poses, alternate skills and talents.
	if len(input.Monster.Formation) != 4 || input.Monster.Inst.GetAnger() != 0 || p6ScopedUnknown(input.Monster) || p6ScopedUnknown(input.Monster.Inst) {
		return reject()
	}
	expected := []string{
		"b08a58e43afd108996b4bfd1aa964742f712eb54cb7c402c363d04dc529d47de",
		"fe4b615600ea0dc06baccd0b7168f39e50751a13b93f1dab24c0242c9b4f62e3",
		"6f0a21e225e89a0fce49f9b951eeb4bbd44815e26883f9a1f540225968dca963",
		"fbc37eb5b74a85d84e043644049ef87494fa4fcf68b6c075a32e6a94f889ecc0",
	}
	livingNPC := false
	for index, actor := range input.Monster.Formation {
		if actor == nil || actor.Prop == nil || actor.Inst == nil || !combatFinite(actor.Inst.GetCurHp()) || actor.Inst.GetCurHp() < 0 || actor.Inst.GetCurHp() > actor.Prop.GetHp() {
			return reject()
		}
		livingNPC = livingNPC || actor.Inst.GetCurHp() > 0
		clone := proto.Clone(actor).(*pb.BattleHero)
		clone.Inst.CurHp = proto.Float64(clone.Prop.GetHp())
		raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(clone)
		if err != nil || p6Digest(raw) != expected[index] {
			return reject()
		}
	}
	if !livingNPC {
		return reject()
	}
	raw, err := json.Marshal(input.Tables)
	if err != nil || p6Digest(raw) != constUtil.P12WantedConfigurationHash {
		return reject()
	}
	return nil
}

func validateP12WantedTerminal(input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome) error {
	reject := func() error { return errors.New(constUtil.P6ReplayConflict) }
	if input == nil || out == nil || out.Outcome == nil || out.OriginalOperates == nil || out.Outcome.Operates == nil || out.OriginalRecordSHA256 != "" || out.Schema != constUtil.P6FinalActorHPSchema || out.EngineVersion != input.EngineVersion || out.ClientVersion != input.ClientVersion || out.ConfigurationHash != input.ConfigurationHash || out.OriginalOperates.AttackerTransOpt != nil || out.OriginalOperates.DefenderTransOpt != nil {
		return reject()
	}
	sources := map[string]*pb.BattleHero{}
	for camp, formation := range [][]*pb.BattleHero{input.Attacker.GetFormation(), input.Monster.GetFormation()} {
		for _, actor := range formation {
			if actor == nil || actor.Prop == nil || actor.Inst == nil {
				return reject()
			}
			sources[fmt.Sprintf("%d/%s", camp, actor.GetGuid())] = actor
		}
	}
	if len(sources) != 6 || len(out.FinalActors) != len(sources) {
		return reject()
	}
	seen := map[string]bool{}
	current := [2]float64{}
	for _, actor := range out.FinalActors {
		key := fmt.Sprintf("%d/%s", actor.Camp, actor.GUID)
		source := sources[key]
		if source == nil || seen[key] || actor.Camp < 0 || actor.Camp > 1 || actor.UID != fmt.Sprintf("%d_%s", actor.Camp, source.GetGuid()) || actor.HeroID != source.GetId() || actor.Pose != source.GetPose() || actor.RuntimePose != 0 || actor.BloodNumber != source.GetBloodNumber() || actor.MaxHP != source.Prop.GetHp() || !combatFinite(actor.CurrentHP) || actor.CurrentHP < 0 || actor.CurrentHP > actor.MaxHP {
			return reject()
		}
		raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(source)
		if err != nil || p6Digest(raw) != actor.InitialActorSHA256 {
			return reject()
		}
		seen[key] = true
		current[actor.Camp] += actor.CurrentHP
	}
	if out.Outcome.FinalRound < 1 || out.Outcome.FinalRound > input.MaxRound+1 || out.OriginalOperates.GetFinalRoundNum() != out.Outcome.FinalRound || out.OriginalOperates.GetAttackerTotalLeftHp() != current[0] || out.OriginalOperates.GetDefenderTotalLeftHp() != current[1] || out.Outcome.Operates.GetAttackerTotalLeftHp() != current[0] || out.Outcome.Operates.GetDefenderTotalLeftHp() != current[1] {
		return reject()
	}
	winner := int32(1)
	if current[1] == 0 && current[0] > 0 {
		winner = 0
	} else if !(current[0] == 0 && current[1] > 0 || current[0] > 0 && current[1] > 0 && out.Outcome.FinalRound == input.MaxRound+1) {
		return reject()
	}
	if out.Outcome.Winner != winner {
		return reject()
	}
	view := *input
	view.Tables = nil
	raw, err := json.Marshal(&view)
	if err != nil || p6Digest(raw) != out.CalculationSourceSHA256 {
		return reject()
	}
	return nil
}

// P12WantedProofForInput admits the validated original automatic family only.
func p12WantedLegacyProofForInput(input *model.FrozenBattleInput) (string, error) {
	if err := validateP12WantedScope(input); err != nil {
		return "", err
	}
	proof := constUtil.P12WantedNativeProof
	for _, actor := range input.Monster.Formation {
		if actor.Inst.GetCurHp() != actor.Prop.GetHp() {
			proof = constUtil.P12WantedPartialHPProof
		}
	}
	if len(proof) != 64 {
		return "", errors.New(constUtil.CombatUnsupported)
	}
	return proof, nil
}

// ComputeP12WantedProtocolFinalHP does not mutate the caller's frozen input.
func (s *GameService) ComputeP12WantedProtocolFinalHP(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	proof, err := P12WantedProofForInput(input)
	if err != nil {
		return nil, err
	}
	if proof == constUtil.P12WantedMechanismProof || proof == constUtil.P12WantedCatalogMechanismProof {
		return s.computeP12MechanismFinalHP(ctx, input, 8)
	}
	out, err := s.computeFrozenBattleFinalHP(ctx, input, nil, true)
	if err != nil {
		return nil, err
	}
	if err = validateP12WantedTerminal(input, out); err != nil {
		return nil, err
	}
	out.Outcome.Authoritative = true
	out.Outcome.CapabilityHash = proof
	return out, nil
}

// ValidateP12WantedCertifiedReceipt binds cold receipts to the same source and proof.
func ValidateP12WantedCertifiedReceipt(input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome) error {
	if out != nil && out.Outcome != nil && (out.Outcome.CapabilityHash == constUtil.P12WantedMechanismProof || out.Outcome.CapabilityHash == constUtil.P12WantedCatalogMechanismProof) {
		return p12MechanismTerminal(input, out, 8)
	}

	proof, err := p12WantedLegacyProofForInput(input)
	if err != nil {
		return err
	}
	if out == nil || out.Outcome == nil || !out.Outcome.Authoritative || out.Outcome.CapabilityHash != proof {
		return errors.New(constUtil.P6ReplayConflict)
	}
	return validateP12WantedTerminal(input, out)
}

// New inputs retain the old native family when eligible; otherwise use the
// separately named local mechanism compatibility family for this exact mode.
func P12WantedProofForInput(input *model.FrozenBattleInput) (string, error) {
	if p12WantedCatalogMarker(input) {
		if err := p12MechanismScope(input, 8); err != nil {
			return "", err
		}
		return constUtil.P12WantedCatalogMechanismProof, nil
	}
	if proof, err := p12WantedLegacyProofForInput(input); err == nil {
		return proof, nil
	}
	if err := p12MechanismScope(input, 8); err != nil {
		return "", err
	}
	return constUtil.P12WantedMechanismProof, nil
}
