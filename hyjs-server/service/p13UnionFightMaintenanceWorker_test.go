package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestP13UFMaintenanceProofFailureDoesNotStopOtherTicks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var calls, reports int
	err := serveP13UFFinalization(ctx, time.Millisecond, func(context.Context, time.Time) error {
		calls++
		if calls < 3 {
			return errors.New("one match has an invalid frozen proof")
		}
		cancel()
		return nil
	}, func(error, int64) { reports++ })
	if err != nil || calls != 3 || reports != 1 {
		t.Fatalf("proof failure stopped or flooded the maintenance worker: calls=%d reports=%d err=%v", calls, reports, err)
	}
}

func TestP13UFMaintenanceDriverFailureStillUsesOuterBackoff(t *testing.T) {
	var calls int
	err := serveP13UFFinalization(context.Background(), time.Millisecond, func(context.Context, time.Time) error {
		calls++
		return sql.ErrConnDone
	}, func(error, int64) { t.Fatal("driver failure must reach the common backoff") })
	if !errors.Is(err, sql.ErrConnDone) || calls != 1 {
		t.Fatalf("driver failure was hidden: calls=%d err=%v", calls, err)
	}
}
