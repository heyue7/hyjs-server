package service

import (
	"context"
	"errors"

	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// ggoBattleTerminal first accepts a completely verified live record. If the
// record cannot be used, the frozen server input still produces a deterministic
// automatic result; client result, HP and operations do not decide that result.
// Callers qualify the frozen input and validate fight ownership before this call.
func (s *GameService) ggoBattleTerminal(ctx context.Context, input *model.FrozenBattleInput, record *pb.BattleOperate, reportedResult int32, reportedPlayer *pb.BattlePlayer, playerID int64, verifyPlayer bool) (*model.CombatFinalHPOutcome, int32, error) {
	check := func(terminal *model.CombatFinalHPOutcome) (int32, error) {
		if err := validateP6GenericMechanismTerminal(input, terminal); err != nil {
			return 0, err
		}
		if terminal.Outcome.Winner == 0 {
			return 1, nil
		}
		if terminal.Outcome.Winner == 1 {
			return 0, nil
		}
		return 0, errors.New("沙漠战斗未产生胜败")
	}
	if record != nil {
		terminal, err := s.ComputeOriginalP6ProtocolFinalHP(ctx, input, record)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, 0, err
		}
		if err == nil {
			result, checkErr := check(terminal)
			if checkErr == nil && p6OriginalProtocolRecordEqual(record, terminal.OriginalOperates) && result == reportedResult &&
				(!verifyPlayer || ggoMonsterReportedHP(playerID, reportedPlayer, input, terminal) == nil) {
				return terminal, result, nil
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	terminal, err := s.ComputeOriginalP6ProtocolFinalHP(ctx, input, nil)
	if err != nil {
		return nil, 0, err
	}
	result, err := check(terminal)
	if err != nil {
		return nil, 0, err
	}
	return terminal, result, nil
}
