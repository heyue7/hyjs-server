package service

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// validateP6CrossStarterScope limits the first independent typed17 family to
// the actual qualified source's original0/0 fallback. Its validation-only view
// reuses closed047 actor fields; the calculation retains the original wrapper.
func validateP6CrossStarterScope(input *model.FrozenBattleInput) error {
	reject := func() error { return errors.New(constUtil.CombatUnsupported) }
	if input == nil {
		return errors.New(constUtil.CombatInvalid)
	}
	if input.BattleType != 17 || input.Attacker == nil || input.Defender == nil || input.DefenderFirst || input.Attacker.GetPlayerId() == input.Defender.GetPlayerId() {
		return reject()
	}
	for _, player := range []*pb.BattlePlayer{input.Attacker, input.Defender} {
		if player.Inst == nil || player.Inst.GetFirstOrder() != 0 || len(player.Inst.ExtBattleAdds) != 0 {
			return reject()
		}
	}
	view, err := cloneP6CalculationInput(input)
	if err != nil {
		return err
	}
	view.BattleType = 11
	view.Attacker.Inst.FirstOrder = proto.Int32(1)
	view.Defender.Inst.FirstOrder = proto.Int32(0)
	return validateP6Starter047Scope(view)
}

// ComputeP6CrossStarterSettlement is an independent typed17 capability gate.
// An empty compiled original proof rejects every input. Old signup/audit data
// cannot be promoted; a consumer must capture a new match under current sources.
func (s *GameService) ComputeP6CrossStarterSettlement(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	if ctx == nil || input == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	frozen, err := cloneP6CalculationInput(input)
	if err != nil {
		return nil, err
	}
	if err = validateP6CrossStarterScope(frozen); err != nil {
		return nil, err
	}
	if constUtil.P6CrossStarterNativeProof == "" {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	result, err := s.ComputeOriginalP6Typed17ProtocolFinalHP(ctx, frozen)
	if err != nil {
		return nil, err
	}
	if err = validateP6CrossStarterTerminal(frozen, result); err != nil {
		return nil, err
	}
	result.Outcome.Authoritative = true
	result.Outcome.CapabilityHash = constUtil.P6CrossStarterNativeProof
	return result, nil
}

func validateP6CrossStarterTerminal(input *model.FrozenBattleInput, result *model.CombatFinalHPOutcome) error {
	if err := validateP6Starter047Terminal(input, result); err != nil {
		return err
	}
	for _, actor := range result.FinalActors {
		if actor.UID != fmt.Sprintf("%d_%s", actor.Camp, actor.GUID) || actor.BloodNumber != 1 {
			return errors.New(constUtil.CombatUnsupported)
		}
	}
	return nil
}

func p6KnownCrossStarterSettlementProof(input *model.FrozenBattleInput, proof string) bool {
	return input != nil && input.BattleType == 17 && input.EngineVersion == constUtil.P6CrossStarterSettlementEngine && input.ClientVersion == constUtil.P6OriginalClientHash && input.ConfigurationHash == constUtil.P6LinearPhysicalTables && proof != "" && proof == constUtil.P6CrossStarterNativeProof
}
