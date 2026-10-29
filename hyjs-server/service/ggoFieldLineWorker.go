package service

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"hyjs-server/db"
)

// Recover due public generations without scanning profiles or spawning timers
// per point. Every line update commits before its revision is observed.
func (s *GameService) ggoRecoverLines(ctx context.Context, now int64) error {
	snapshot, err := s.adminFieldRuntimeSnapshotForRequest(ctx, s.OriginalServerID())
	if err != nil || snapshot.ggo == nil {
		return err
	}
	var due []db.GGOFieldLine
	if err = db.GetIns().WithContext(ctx).Select("server_id", "line").
		Where("server_id=? AND next_due_at<=?", s.OriginalServerID(), now).
		Order("next_due_at, line").Limit(32).Find(&due).Error; err != nil {
		return err
	}
	var failed error
	for _, item := range due {
		err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := ServerGameClockGuardTransaction(tx); err != nil {
				return err
			}
			_, _, _, err := ggoLoadLine(tx, s.OriginalServerID(), item.Line, snapshot.ggo, snapshot.ggoRevision, now)
			return err
		})
		if err != nil {
			failed = errors.Join(failed, err)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return failed
}

// A bounded revision read detects all commits, including cancellations made by
// original field paths. Initial observations resynchronize connected clients.
func (s *GameService) ggoObserveLineChanges(ctx context.Context, seen map[int64]int64) error {
	var rows []db.GGOFieldLine
	if err := db.GetIns().WithContext(ctx).Select("server_id", "line", "revision").
		Where("server_id=? AND line>=? AND line<=?", s.OriginalServerID(), 1, 32).
		Order("line").Limit(32).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		previous, exists := seen[row.Line]
		if !exists || previous != row.Revision {
			seen[row.Line] = row.Revision
			s.ggoNotifyFieldChange(ctx, int32(row.Line))
		}
	}
	return nil
}

// Preserve readable diagnostics during persistent storage failures without
// generating a warning per half-second for each retry.
type ggoWorkerWarning struct {
	last       time.Time
	suppressed int64
}
