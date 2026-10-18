package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"hyjs-server/constUtil"
	"io"
	"net"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

func TestBackgroundWorkerTransientClassification(t *testing.T) {
	for _, cause := range []error{context.DeadlineExceeded, driver.ErrBadConn, sql.ErrConnDone, mysql.ErrInvalidConn, &mysql.MySQLError{Number: 1040}, &mysql.MySQLError{Number: 1053}, &mysql.MySQLError{Number: 1205}, &mysql.MySQLError{Number: 1213}, &mysql.MySQLError{Number: 2006}, &mysql.MySQLError{Number: 2013}, &net.OpError{Op: "read", Net: "tcp", Err: syscall.ETIMEDOUT}, &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}} {
		if !backgroundWorkerTransient(fmt.Errorf("query: %w", cause)) {
			t.Fatalf("transient error rejected: %v", cause)
		}
	}
	for _, cause := range []error{context.Canceled, &mysql.MySQLError{Number: 1045}, &mysql.MySQLError{Number: 1062}, &mysql.MySQLError{Number: 1146}, errors.New("settlement source hash mismatch"), errors.New("context deadline exceeded"), io.EOF, mysql.ErrMalformPkt} {
		if backgroundWorkerTransient(fmt.Errorf("query: %w", cause)) {
			t.Fatalf("permanent/ambiguous error retried: %v", cause)
		}
	}
}

func TestBackgroundWorkerBackoffBoundedSequentialAndCancellation(t *testing.T) {
	var (
		calls, reports int
		waits          []time.Duration
		inside         bool
	)
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), constUtil.GameServiceContextKey, "preserved"))
	defer cancel()
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 15 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second}
	err := serveBackgroundWorker(ctx, "p1-periods", func(got context.Context) error {
		if inside || got != ctx || got.Value(constUtil.GameServiceContextKey) != "preserved" {
			t.Fatal("worker overlap or lost parent context")
		}
		inside = true
		calls++
		inside = false
		// A query deadline must leave the live process context usable.
		return fmt.Errorf("read failed: %w", context.DeadlineExceeded)
	}, time.Now, func(got context.Context, delay time.Duration) error {
		if inside || got != ctx {
			t.Fatal("retry started while an invocation was active")
		}
		waits = append(waits, delay)
		if len(waits) == len(want) {
			cancel()
			return got.Err()
		}
		return nil
	}, func(err error, delay time.Duration, failures int) {
		reports++
		if !errors.Is(err, context.DeadlineExceeded) || delay > constUtil.BackgroundWorkerMaxBackoff || failures > 6 {
			t.Fatal("missing cause or unbounded retry")
		}
	})
	if err != nil || calls != len(want) || reports != calls || len(waits) != len(want) {
		t.Fatal("cancellation/retry counts", err, calls, reports, waits)
	}
	for i, delay := range waits {
		if delay != want[i] {
			t.Fatal("incorrect backoff", i, delay)
		}
	}
}

func TestBackgroundWorkerRecoveryThenFatalPreservesCauseAndName(t *testing.T) {
	var calls, waits int
	fatal := errors.New("frozen settlement hash mismatch")
	err := serveBackgroundWorker(context.Background(), "p11-arena-daily", func(context.Context) error {
		calls++
		if calls == 1 {
			return &mysql.MySQLError{Number: 1213}
		}
		return fatal
	}, time.Now, func(context.Context, time.Duration) error { waits++; return nil }, func(error, time.Duration, int) {})
	if !errors.Is(err, fatal) || !strings.Contains(err.Error(), "p11-arena-daily") || calls != 2 || waits != 1 {
		t.Fatal("fatal error hidden or transient not resumed", err, calls, waits)
	}
}

func TestBackgroundWorkerUnexpectedNilDoesNotSpin(t *testing.T) {
	var calls, waits int
	err := serveBackgroundWorker(context.Background(), "p14-delivery", func(context.Context) error { calls++; return nil }, time.Now, func(context.Context, time.Duration) error { waits++; return nil }, func(error, time.Duration, int) {})
	if err == nil || !strings.Contains(err.Error(), "p14-delivery") || calls != 1 || waits != 0 {
		t.Fatal("unexpected clean stop spun or lost name", err, calls, waits)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = ServeBackgroundWorker(ctx, "cancelled", func(context.Context) error { t.Fatal("cancelled worker invoked"); return nil }); err != nil {
		t.Fatal("normal cancellation returned fatal", err)
	}
	if err = waitBackgroundWorker(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatal("backoff failed to observe cancellation", err)
	}
}

func TestBackgroundWorkerHealthyLifetimeResetsBackoff(t *testing.T) {
	var (
		calls int
		waits []time.Duration
	)
	clock := time.Unix(1000, 0)
	fatal := errors.New("stop after recovery")
	err := serveBackgroundWorker(context.Background(), "p10-voice", func(context.Context) error {
		calls++
		if calls == 3 {
			clock = clock.Add(constUtil.BackgroundWorkerHealthyWindow)
		}
		if calls == 4 {
			return fatal
		}
		return driver.ErrBadConn
	}, func() time.Time { return clock }, func(_ context.Context, delay time.Duration) error { waits = append(waits, delay); return nil }, func(error, time.Duration, int) {})
	if !errors.Is(err, fatal) || len(waits) != 3 || waits[0] != time.Second || waits[1] != 2*time.Second || waits[2] != time.Second {
		t.Fatal("healthy running worker retained prior outage backoff", err, waits)
	}
}
