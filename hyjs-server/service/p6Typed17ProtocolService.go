package service

import (
	"context"
	"errors"

	"hyjs-server/constUtil"
	"hyjs-server/model"
)

// ComputeOriginalP6Typed17ProtocolFinalHP audits the original two-player
// SCKuafuMatchPlayer SetData wrapper. KFReplayData and recorded/multi-fighter
// brackets retain their independent unsupported boundary. This API only returns
// a diagnostic terminal snapshot and never grants settlement authority.
func (s *GameService) ComputeOriginalP6Typed17ProtocolFinalHP(ctx context.Context, input *model.FrozenBattleInput) (*model.CombatFinalHPOutcome, error) {
	if input == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	if input.BattleType != 17 || input.Attacker == nil || input.Defender == nil || input.Monster != nil || input.KFReplay != nil || input.IsAttToDef || input.MaxRound != 40 || len(input.WinConditions) != 0 || input.PVECompatibilityRule != "" || input.DiagnosticAIDriverPolicy != "" {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	a, b := input.Attacker.GetInst(), input.Defender.GetInst()
	if a == nil || b == nil {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	first := a.GetFirstOrder() == 0 && b.GetFirstOrder() == 1
	if !(first || a.GetFirstOrder() == 1 && b.GetFirstOrder() == 0 || a.GetFirstOrder() == 0 && b.GetFirstOrder() == 0) || input.DefenderFirst != first {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	for _, factor := range []*float64{input.AttackerDamageFactor, input.DefenderDamageFactor} {
		if factor != nil && *factor != 1 {
			return nil, errors.New(constUtil.CombatUnsupported)
		}
	}
	return s.computeFrozenBattleFinalHP(ctx, input, nil, true)
}
