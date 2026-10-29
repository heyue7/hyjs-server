package service

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/elog"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type ggoExtractionTimer struct {
	requestID string
	timer     *time.Timer
}

func (s *GameService) ggoScheduleExtraction(ctx context.Context, id int64, requestID string, readyAt int64) {
	base := s.p10ClientOwner()
	task := &ggoExtractionTimer{requestID: requestID}
	delay := time.Until(time.UnixMilli(readyAt))
	if delay < time.Millisecond {
		delay = time.Millisecond
	}
	task.timer = time.AfterFunc(delay, func() {
		if !base.ggoExtractionTimers.CompareAndDelete(id, task) {
			return
		}
		callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		if err := base.ggoFinishExtraction(callCtx, id, requestID); err != nil {
			elog.Warn("ggo extraction deadline failed", zap.Error(err))
		}
	})
	if old, ok := base.ggoExtractionTimers.Swap(id, task); ok {
		old.(*ggoExtractionTimer).timer.Stop()
	}
}

func (s *GameService) ggoFinishExtraction(ctx context.Context, id int64, requestID string) error {
	snapshot, err := s.adminFieldRuntimeSnapshotForRequest(ctx, s.OriginalServerID())
	if err != nil {
		return err
	}
	runtime := s.withRuntimeTables(snapshot.tables)
	runtime.GGOFieldConfig = snapshot.ggo
	runtime.GGOFieldEconomy = snapshot.ggoEconomy
	runtime.GGOFieldRevision = snapshot.ggoRevision
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, runtime)
	completed := false
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		player, game, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		if game.GGOField == nil || game.GGOField.Extraction == nil || game.GGOField.Extraction.RequestID != requestID {
			return nil
		}
		now := time.Now().UnixMilli()
		if now < game.GGOField.Extraction.ReadyAt {
			return nil
		}
		actor := &ggoFieldActor{Service: runtime, Row: &row, Player: player, Game: game, Now: now}
		result, changed, err := ggoExtractionStatusLocked(tx, actor, requestID)
		if err != nil {
			return err
		}
		if changed {
			if err = saveProfile(tx, &row, player, game); err != nil {
				return err
			}
		}
		completed = result.Status == "completed"
		return nil
	})
	if err == nil && completed {
		s.p10SceneSend(ctx, id, &pb.SCRefreshScene{})
	}
	return err
}

func (s *GameService) ggoRecoverExtractions(ctx context.Context, now int64) error {
	var rows []db.GGOFieldReceipt
	if err := db.GetIns().WithContext(ctx).Select("player_id", "request_id").Where("operation=? AND active=? AND ready_at<=?", constUtil.GGOFieldOpExtractStart, true, now).Order("ready_at").Limit(64).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if err := s.ggoFinishExtraction(ctx, row.PlayerID, row.RequestID); err != nil && !errors.Is(err, context.Canceled) {
			elog.Warn("ggo extraction recovery failed", zap.Error(err))
		}
	}
	return nil
}

// The trusted socket's first disconnect decides its original countdown even
// if cleanup is delayed until after the deadline or a login rotates.
func (s *GameService) ggoExtractionDisconnect(ctx context.Context, id int64, session []byte, disconnectedAt int64) (bool, error) {
	if disconnectedAt <= 0 {
		return false, errors.New("断线时间无效")
	}
	var observed db.Profile
	if err := db.GetIns().WithContext(ctx).First(&observed, id).Error; err != nil {
		return false, err
	}
	// Revoked sessions may cancel previously accepted actions, never finish them.
	authErr := authorizeProfileSession(ctx, &observed, session)
	ctx, err := s.BindLocalPlayContext(ctx, &observed)
	if err != nil {
		return false, err
	}
	snapshot, err := s.adminFieldRuntimeSnapshotForRequest(ctx, s.OriginalServerID())
	if err != nil {
		return false, err
	}
	runtime := s.withRuntimeTables(snapshot.tables)
	runtime.GGOFieldConfig = snapshot.ggo
	runtime.GGOFieldEconomy = snapshot.ggoEconomy
	runtime.GGOFieldRevision = snapshot.ggoRevision
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, runtime)
	completed := false
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		player, game, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		if game.GGOField == nil || game.GGOField.Extraction == nil || game.GGOField.Extraction.AcceptedAt > disconnectedAt {
			return nil
		}
		if subtle.ConstantTimeCompare(row.Session, session) != 1 || disconnectedAt < game.GGOField.Extraction.ReadyAt {
			_, err = ggoExtractionCancelLocked(tx, id, game, "disconnected", disconnectedAt)
		} else {
			if authErr != nil {
				return authErr
			}
			actor := &ggoFieldActor{Service: runtime, Row: &row, Player: player, Game: game, Now: disconnectedAt}
			var result *model.GGOFieldExtractionStatusResponse
			result, err = ggoExtractionFinishWithSessionLocked(tx, actor, session)
			completed = err == nil && result.Status == "completed"
		}
		if err != nil {
			return err
		}
		return saveProfile(tx, &row, player, game)
	})
	return completed, err
}
