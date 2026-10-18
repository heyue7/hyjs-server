package service

import (
	"context"
	"encoding/json"
	"errors"

	"hyjs-server/constUtil"
	"hyjs-server/model"
)

// Every original mode has an independent local compatibility proof. Empty
// pending proofs keep this draft closed; no Rush/old actor proof is borrowed.
func p11GenericMechanismProof(input *model.FrozenBattleInput) (string, error) {
	reject := func() (string, error) { return "", errors.New(constUtil.CombatUnsupported) }
	if input == nil || input.Attacker == nil || input.Defender == nil || input.Monster != nil || input.Attacker.Inst == nil || input.Defender.Inst == nil || input.KFReplay != nil || input.KFReplayViewer != nil || input.IsAttToDef || len(input.WinConditions) != 0 || input.PVECompatibilityRule != "" || input.DiagnosticAIDriverPolicy != "" || input.DiagnosticManualAutoStartRoundTime != nil || input.EngineVersion != constUtil.P6EngineVersion || input.ClientVersion != constUtil.P6OriginalClientHash || input.ConfigurationHash != constUtil.P6LinearPhysicalTables {
		return reject()
	}
	maximum := int32(40)
	proof := ""
	switch input.BattleType {
	case 2:
		proof = constUtil.P11GenericArenaCompatibilityProof
	case 3:
		maximum = 20
		proof = constUtil.P11GenericPlunderCompatibilityProof
	case 11:
		proof = constUtil.P11GenericFriendCompatibilityProof
	case 19:
		proof = constUtil.P11GenericGrabCompatibilityProof
	case 20:
		proof = constUtil.P11GenericLimitCompatibilityProof
	default:
		return reject()
	}
	if input.MaxRound != maximum || len(proof) != 64 {
		return reject()
	}
	// Retain the public wrapper's actual initiative. Friend11 permits either
	// legitimate side first; the other current producers set attacker1/defender0.
	if input.BattleType != 11 && (input.DefenderFirst || input.Attacker.Inst.GetFirstOrder() != 1 || input.Defender.Inst.GetFirstOrder() != 0) {
		return reject()
	}
	if err := validateFrozenP6(input); err != nil {
		return "", err
	}
	return proof, nil
}

// This calculation retains the original wrapper, MaxRound, source actors,
// numeric panels and AI. It never reads a client winner or writes business data.
func (s *GameService) ComputeP11GenericMechanismSettlement(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	proof, err := p11GenericMechanismProof(input)
	if err != nil {
		return nil, err
	}
	if _, err = s.ValidateP6GenericMechanisms(ctx, input); err != nil {
		return nil, err
	}
	var out *model.CombatFinalHPOutcome
	switch input.BattleType {
	case 19:
		out, err = s.ComputeOriginalP6GrabProtocolFinalHP(ctx, input)
	case 20:
		out, err = s.ComputeOriginalP6LimitProtocolFinalHP(ctx, input)
	default:
		out, _, err = s.ComputeP6GenericMechanismBattle(ctx, input, nil)
	}
	if err != nil {
		return nil, err
	}
	out.Outcome.Authoritative = true
	out.Outcome.CapabilityHash = proof
	if err = validateP11GenericMechanismTerminal(input, out); err != nil {
		return nil, err
	}
	return out, nil
}

// Cold receipts select this family by its mode-specific proof, then validate
// the exact frozen source and full actual terminal, retaining all old readers.
func validateP11GenericMechanismTerminal(input *model.FrozenBattleInput, out *model.CombatFinalHPOutcome) error {
	proof, err := p11GenericMechanismProof(input)
	if err != nil {
		return err
	}
	if out == nil || out.Outcome == nil || !out.Outcome.Authoritative || out.Outcome.CapabilityHash != proof || out.Schema != constUtil.P6FinalActorHPSchema || out.EngineVersion != input.EngineVersion || out.ClientVersion != input.ClientVersion || out.ConfigurationHash != input.ConfigurationHash || out.OriginalRecordSHA256 != "" || out.OriginalOperates == nil || len(out.OriginalOperates.SkillOpts) != 0 || out.OriginalOperates.AttackerTransOpt != nil || out.OriginalOperates.DefenderTransOpt != nil {
		return errors.New(constUtil.P6ReplayConflict)
	}
	for _, actor := range out.FinalActors {
		if actor.RuntimePose != 0 {
			return errors.New(constUtil.P6ReplayConflict)
		}
	}
	if err = validateP6GenericMechanismTerminal(input, out); err != nil {
		return err
	}
	closure := *input
	closure.Tables = nil
	raw, err := json.Marshal(&closure)
	if err != nil || p6Digest(raw) != out.CalculationSourceSHA256 {
		return errors.New(constUtil.P6ReplayConflict)
	}
	return nil
}

// p11KnownGenericMechanismProof selects only the independently named current mode.
func p11KnownGenericMechanismProof(input *model.FrozenBattleInput, proof string) bool {
	expected, err := p11GenericMechanismProof(input)
	return err == nil && proof != "" && expected == proof
}

// p11SettlementScope keeps legacy admission separate for historical receipts.
func p11SettlementScope(input *model.FrozenBattleInput, proof string, legacy func(*model.FrozenBattleInput) error) error {
	if p11KnownGenericMechanismProof(input, proof) {
		return nil
	}
	return legacy(input)
}
