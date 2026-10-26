package service

import (
	"context"
	"time"

	"go.uber.org/zap"
	"hyjs-server/constUtil"
	"hyjs-server/elog"
)

// ServeP13UnionFightGuardBattle owns two bounded calculation workers. A bad
// durable input is quarantined by the calculator and cannot stop other matches
// or the process. Repeated diagnostics are summarized once per minute.
func (s *GameService) ServeP13UnionFightGuardBattle(ctx context.Context) error {
	var nextWarning time.Time
	var suppressed int
	return s.RunP13UnionFightGuardBattleWorker(ctx, func(err error) {
		now := time.Now()
		if now.Before(nextWarning) {
			suppressed++
			return
		}
		elog.Warn("guild guard calculation failed; durable retry retained", zap.Error(err), zap.Int("suppressed", suppressed))
		nextWarning = now.Add(constUtil.P13UFGuardWorkerWarnInterval)
		suppressed = 0
	})
}
