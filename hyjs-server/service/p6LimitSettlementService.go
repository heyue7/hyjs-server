package service

import (
	"context"
	"encoding/json"
	"errors"
	"hyjs-server/constUtil"
	"hyjs-server/model"
)

// Keep the original Limit20 wrapper independent from Arena/Friend/ordinary17.
// Empty Limit proof never applies business outcomes, fees or scores.
func (s *GameService) ComputeOriginalP6LimitProtocolFinalHP(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	if ctx == nil || input == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	if input.BattleType != 20 || input.Attacker == nil || input.Defender == nil || input.Monster != nil || input.KFReplay != nil || input.KFReplayViewer != nil || input.IsAttToDef || len(input.WinConditions) != 0 || input.PVECompatibilityRule != "" || input.DiagnosticAIDriverPolicy != "" || input.DiagnosticManualAutoStartRoundTime != nil {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	return s.computeFrozenBattleFinalHP(ctx, input, nil, true)
}

func validateP6LimitScope(input *model.FrozenBattleInput) error {
	if input == nil {
		return errors.New(constUtil.CombatInvalid)
	}
	if input.BattleType != 20 || input.KFReplayViewer != nil || input.DiagnosticManualAutoStartRoundTime != nil {
		return errors.New(constUtil.CombatUnsupported)
	}
	view, err := cloneP6CalculationInput(input)
	if err != nil {
		return err
	}
	view.BattleType = 11
	if validateP6Starter047Scope(view) == nil {
		return nil
	}
	// The new independent full-party wrapper is observed with raw1/0/full40.
	// Existing single047 semantics and historical capability stay unchanged.
	if input.Seed <= 0 || input.DefenderFirst || input.Attacker == nil || input.Defender == nil || input.Attacker.GetInst().GetFirstOrder() != 1 || input.Defender.GetInst().GetFirstOrder() != 0 {
		return errors.New(constUtil.CombatUnsupported)
	}
	return validateP6ActualPartyScope(view)
}
func (s *GameService) ComputeP6LimitSettlement(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	if ctx == nil || input == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	frozen, err := cloneP6CalculationInput(input)
	if err != nil {
		return nil, err
	}
	if err = validateP6LimitScope(frozen); err != nil {
		return s.ComputeP11GenericMechanismSettlement(ctx, frozen)
	}
	proof := p6LimitInputProof(frozen)
	if proof == "" {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	result, err := s.ComputeOriginalP6LimitProtocolFinalHP(ctx, frozen)
	if err != nil {
		return nil, err
	}
	if err = validateP6LimitTerminal(frozen, result); err != nil {
		return nil, err
	}
	result.Outcome.Authoritative = true
	result.Outcome.CapabilityHash = proof
	return result, nil
}
func p6KnownLimitSettlementProof(input *model.FrozenBattleInput, proof string) bool {
	if input != nil && input.BattleType == 20 && p11KnownGenericMechanismProof(input, proof) {
		return true
	}
	return proof != "" && validateP6LimitScope(input) == nil && proof == p6LimitInputProof(input)
}
func validateP6LimitTerminal(input *model.FrozenBattleInput, result *model.CombatFinalHPOutcome) error {
	if result != nil && result.Outcome != nil && p11KnownGenericMechanismProof(input, result.Outcome.CapabilityHash) {
		return validateP11GenericMechanismTerminal(input, result)
	}
	if err := validateP6LimitScope(input); err != nil {
		return err
	}
	var terminalError error
	if len(input.Attacker.Formation) == 1 {
		terminalError = validateP6Starter047Terminal(input, result)
	} else {
		terminalError = validateP6ActualPartyTerminal(input, result)
	}
	if terminalError != nil {
		return terminalError
	}
	if result.Schema != constUtil.P6FinalActorHPSchema || result.EngineVersion != input.EngineVersion || result.ClientVersion != input.ClientVersion || result.ConfigurationHash != input.ConfigurationHash {
		return errors.New(constUtil.P6ReplayConflict)
	}
	closure := *input
	closure.Tables = nil
	raw, err := json.Marshal(&closure)
	if err != nil || p6Digest(raw) != result.CalculationSourceSHA256 {
		return errors.New(constUtil.P6ReplayConflict)
	}
	return nil
}

// Select by already validated structural family, preserving accepted047 receipts.
// Empty new proof cannot promote a complete party using the historical proof.
func p6LimitInputProof(input *model.FrozenBattleInput) string {
	if input == nil || input.Attacker == nil || input.Defender == nil {
		return ""
	}
	if len(input.Attacker.Formation) == 1 && len(input.Defender.Formation) == 1 {
		return constUtil.P6LimitNativeProof
	}
	if len(input.Attacker.Formation) == 2 && len(input.Defender.Formation) == 2 {
		return constUtil.P6LimitPartyNativeProof
	}
	return ""
}
