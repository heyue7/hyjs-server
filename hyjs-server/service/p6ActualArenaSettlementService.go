package service

import (
	"context"
	"errors"

	"hyjs-server/constUtil"
	"hyjs-server/model"
)

// The original Arena callback is a separately verified mode. The private
// validation view shares only recovered complete-party actor/mechanism checks;
// original type2, initiative, source hash and calculation remain unchanged.
func validateP6ActualArenaScope(input *model.FrozenBattleInput) error {
	if input == nil {
		return errors.New(constUtil.CombatInvalid)
	}
	if input.BattleType != 2 || input.DefenderFirst || input.Attacker.GetInst().GetFirstOrder() != 1 || input.Defender.GetInst().GetFirstOrder() != 0 {
		return errors.New(constUtil.CombatUnsupported)
	}
	view, err := cloneP6CalculationInput(input)
	if err != nil {
		return err
	}
	view.BattleType = 11
	return validateP6ActualPartyScope(view)
}

// ComputeP6ActualArenaSettlement cannot borrow a Friend11 proof. Empty Arena
// proof rejects every matching source; old diagnostic captures never promote.
func (s *GameService) ComputeP6ActualArenaSettlement(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	if ctx == nil || input == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	frozen, err := cloneP6CalculationInput(input)
	if err != nil {
		return nil, err
	}
	if err = validateP6ActualArenaScope(frozen); err != nil {
		return nil, err
	}
	if constUtil.P6ActualArenaNativeProof == "" {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	result, err := s.ComputeOriginalP6ProtocolFinalHP(ctx, frozen, nil)
	if err != nil {
		return nil, err
	}
	if err = validateP6ActualPartyTerminal(frozen, result); err != nil {
		return nil, err
	}
	result.Outcome.Authoritative = true
	result.Outcome.CapabilityHash = constUtil.P6ActualArenaNativeProof
	return result, nil
}

func p6KnownActualArenaSettlementProof(input *model.FrozenBattleInput, proof string) bool {
	return input != nil && input.BattleType == 2 && input.EngineVersion == constUtil.P6LegacyPartySettlementEngine && input.ClientVersion == constUtil.P6OriginalClientHash && input.ConfigurationHash == constUtil.P6LinearPhysicalTables && proof != "" && proof == constUtil.P6ActualArenaNativeProof
}
