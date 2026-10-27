package service

import (
	"context"
	"errors"

	"hyjs-server/constUtil"
	"hyjs-server/model"
)

// validateP6Arena047Scope reuses the exact closed047 actor/mechanism conditions.
// Only validation sees a shallow mode11 view; calculation always retains the
// actual original mode2 input and its own independently certified typed driver.
func validateP6Arena047Scope(input *model.FrozenBattleInput) error {
	if input == nil {
		return errors.New(constUtil.CombatInvalid)
	}
	if input.BattleType != 2 || input.EngineVersion != constUtil.P6ArenaSettlementEngine || input.DefenderFirst || input.Attacker.GetInst().GetFirstOrder() != 1 || input.Defender.GetInst().GetFirstOrder() != 0 {
		return errors.New(constUtil.CombatUnsupported)
	}
	actorConditions := *input
	actorConditions.BattleType = 11
	return validateP6Starter047Scope(&actorConditions)
}

// ComputeP6ArenaSettlement will admit only the independent original mode2 scope.
// It never rewrites the calculation mode or promotes the old mode11 certificate.
func (s *GameService) ComputeP6ArenaSettlement(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	if err := validateP6Arena047Scope(input); err != nil {
		return nil, err
	}
	if constUtil.P6ArenaNativeProof == "" {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	frozen, err := cloneP6CalculationInput(input)
	if err != nil {
		return nil, err
	}
	if err = validateP6Arena047Scope(frozen); err != nil {
		return nil, err
	}
	result, err := s.ComputeOriginalP6ProtocolFinalHP(ctx, frozen, nil)
	if err != nil {
		return nil, err
	}
	if err = validateP6Starter047Terminal(frozen, result); err != nil {
		return nil, err
	}
	result.Outcome.Authoritative = true
	result.Outcome.CapabilityHash = constUtil.P6ArenaNativeProof
	return result, nil
}

func p6KnownArenaSettlementProof(input *model.FrozenBattleInput, proof string) bool {
	return input != nil && input.BattleType == 2 && input.EngineVersion == constUtil.P6ArenaSettlementEngine && input.ClientVersion == constUtil.P6OriginalClientHash && input.ConfigurationHash == constUtil.P6LinearPhysicalTables && proof != "" && (proof == constUtil.P6ArenaNativeProof || proof == constUtil.P6ArenaParentProof)
}
