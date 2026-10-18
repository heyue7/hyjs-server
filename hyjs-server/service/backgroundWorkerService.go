package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"hyjs-server/constUtil"
	"hyjs-server/elog"
	"net"
	"syscall"
	"time"

	"github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
)

// ServeBackgroundWorker resumes an existing durable periodic worker after a
// transient connection/transaction failure. Each worker must re-read its own
// locked ledger before applying effects; request handlers and listeners must
// not use this adapter. No transaction callback is replayed inside a transaction.
func ServeBackgroundWorker(ctx context.Context, name string, serve func(context.Context) error) error {
	return serveBackgroundWorker(ctx, name, serve, time.Now, waitBackgroundWorker, func(err error, delay time.Duration, failures int) {
		elog.Warn("background worker transient failure; resuming durable worker", zap.String("worker", name), zap.Error(err), zap.Duration("retryAfter", delay), zap.Int("consecutiveFailures", failures))
	})
}

func serveBackgroundWorker(ctx context.Context, name string, serve func(context.Context) error, clock func() time.Time, wait func(context.Context, time.Duration) error, report func(error, time.Duration, int)) error {
	failures := 0
	for {
		if ctx.Err() != nil {
			return nil
		}
		started := clock()
		err := serve(ctx)
		// The parent lifecycle owns shutdown. Child query deadlines are handled
		// below while an explicit process cancellation completes normally.
		if ctx.Err() != nil {
			return nil
		}
		if err == nil {
			return fmt.Errorf(constUtil.BackgroundWorkerUnexpectedStop, name)
		}
		if !backgroundWorkerTransient(err) {
			return fmt.Errorf(constUtil.BackgroundWorkerFailure, name, err)
		}
		if clock().Sub(started) >= constUtil.BackgroundWorkerHealthyWindow {
			failures = 0
		}
		// Saturate the exponent and delay during a prolonged outage. There is
		// exactly one synchronous invocation, so recovery never overlaps itself.
		if failures < 6 {
			failures++
		}
		delay := backgroundWorkerBackoff(failures)
		report(err, delay, failures)
		if err = wait(ctx, delay); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf(constUtil.BackgroundWorkerFailure, name, err)
		}
	}
}

func backgroundWorkerBackoff(failures int) time.Duration {
	switch {
	case failures <= 1:
		return constUtil.BackgroundWorkerInitialBackoff
	case failures <= 4:
		return constUtil.BackgroundWorkerInitialBackoff << (failures - 1)
	case failures == 5:
		return constUtil.BackgroundWorkerLateBackoff
	default:
		return constUtil.BackgroundWorkerMaxBackoff
	}
}

func waitBackgroundWorker(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func backgroundWorkerTransient(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, driver.ErrBadConn) || errors.Is(err, sql.ErrConnDone) || errors.Is(err, mysql.ErrInvalidConn) {
		return true
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		switch mysqlErr.Number {
		case 1040, 1053, 1205, 1213, 2006, 2013:
			return true
		default:
			return false
		}
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) && (networkErr.Timeout() || networkErr.Temporary()) {
		return true
	}
	for _, cause := range []error{syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.ECONNABORTED, syscall.EPIPE, syscall.ETIMEDOUT, syscall.ENETUNREACH, syscall.EHOSTUNREACH} {
		if errors.Is(err, cause) {
			return true
		}
	}
	return false
}
