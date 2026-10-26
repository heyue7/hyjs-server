package service

import (
	"context"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
	"hyjs-server/constUtil"
	"hyjs-server/elog"
)

// Finalization has its own one-second cadence. The thirty-second admission
// worker cannot reliably seal a normal round within the frozen five-second
// deadline. Both entry points retain the same authoritative single-match TX.
func (s *GameService) ServeP13UnionFightFinalization(ctx context.Context) error {
	return serveP13UFFinalization(ctx, constUtil.P13UFFinalizationInterval, s.TickP13UnionFightFinalization, func(err error, consecutive int64) {
		elog.Warn("guild war finalization source remains unresolved", zap.Error(err), zap.Int64("consecutiveFailures", consecutive))
	})
}

func serveP13UFFinalization(ctx context.Context, interval time.Duration, tick func(context.Context, time.Time) error, report func(error, int64)) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var failures int64
	var lastReport time.Time
	for {
		if ctx.Err() != nil {
			return nil
		}
		err := tick(ctx, p13UFNow(ctx))
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			// Driver failures use the common durable-worker backoff. A malformed
			// match stays closed to awards while other candidates keep progressing;
			// it must not terminate unrelated game listeners.
			var driverError *mysql.MySQLError
			if backgroundWorkerTransient(err) || errors.As(err, &driverError) {
				return err
			}
			failures++
			wall := time.Now()
			if report != nil && (lastReport.IsZero() || wall.Sub(lastReport) >= constUtil.P13UFFinalizationLogInterval) {
				report(err, failures)
				lastReport = wall
			}
		} else {
			failures = 0
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
