package service

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// Publish a changed line once, then refresh due points in separate short
// transactions. Busy points are skipped without holding up another resource.
func (s *GameService) ggoRecoverLines(ctx context.Context, now int64) error {
	snapshot, err := s.adminFieldRuntimeSnapshotForRequest(ctx, s.OriginalServerID())
	if err != nil || snapshot.ggo == nil {
		return err
	}
	runtime := s.withRuntimeTables(snapshot.tables)
	runtime.GGOFieldConfig, runtime.GGOFieldRevision = snapshot.ggo, snapshot.ggoRevision
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, runtime)
	var migrations []db.GGOFieldLine
	if err = db.GetIns().WithContext(ctx).Select("server_id", "line").
		Where("server_id=? AND (point_state_ready=? OR deployment_revision<>? OR cycle_ms=0)", s.OriginalServerID(), false, snapshot.ggoRevision).
		Order("line").Limit(32).Find(&migrations).Error; err != nil {
		return err
	}
	var failed error
	for _, item := range migrations {
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
	var due []db.GGOFieldPoint
	if err = db.GetIns().WithContext(ctx).Select("server_id", "line", "point_id").Where("server_id=? AND next_due_at<=?", s.OriginalServerID(), now).
		Order("next_due_at,line,point_id").Limit(32).Find(&due).Error; err != nil {
		return errors.Join(failed, err)
	}
	for _, point := range due {
		attemptCtx := context.WithValue(ctx, constUtil.GGOFieldPointSkipLockedContextKey{}, true)
		err = db.GetIns().WithContext(attemptCtx).Transaction(func(tx *gorm.DB) error {
			if err := ServerGameClockGuardTransaction(tx); err != nil {
				return err
			}
			_, _, _, err := ggoLoadLine(tx, s.OriginalServerID(), point.Line, snapshot.ggo, snapshot.ggoRevision, now, point.PointID)
			return err
		}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
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
	if err := db.GetIns().WithContext(ctx).Table("ggo_field_lines AS l").
		Select("l.server_id,l.line,l.revision+COALESCE(SUM(p.revision),0) AS revision").
		Joins("LEFT JOIN ggo_field_points AS p ON p.server_id=l.server_id AND p.line=l.line").
		Where("l.server_id=? AND l.line>=? AND l.line<=?", s.OriginalServerID(), 1, 32).
		Group("l.server_id,l.line,l.revision").Order("l.line").Limit(32).Scan(&rows).Error; err != nil {
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
