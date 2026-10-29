package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// P12WantedRequireIdle runs under the owning profile lock and never calls a common guard.
// Accepted receipt recovery must run before this helper. Closing the policy does
// not erase an accepted pending fight or prevent its authenticated End.
func P12WantedRequireIdle(tx *gorm.DB, id int64, now time.Time) error {
	if !gameSchemaHasTable(tx, &db.P12WantedFight{}) {
		return nil
	}
	var pending []db.P12WantedFight
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("player_id=? AND committed=? AND abandoned_at=0", id, false).Order("created_at,id").Find(&pending).Error; err != nil {
		return err
	}
	for _, fight := range pending {
		var instance db.P12WantedInstance
		if err := tx.First(&instance, "id=?", fight.InstanceID).Error; err != nil {
			return err
		}
		if instance.State != constUtil.P12WantedActive && instance.State != constUtil.P12WantedKilled && instance.State != constUtil.P12WantedExpired {
			return errors.New(constUtil.P12WantedInvalid)
		}
		if calendar75Adjusted(tx) && (fight.CreatedAt > now.UnixMilli() || instance.CreatedAt > now.UnixMilli()) {
			continue
		}
		// A definitive rejected End has already returned the client to the room.
		// Keep its paid immutable fight recoverable, while releasing only the
		// presentation lease so another mode is not blocked by a failed replay.
		if fight.RejectedAt > 0 {
			continue
		}
		if instance.State == constUtil.P12WantedActive && instance.ExpiresAt > now.UnixMilli() && instance.CurrentHP > 0 {
			return errors.New(constUtil.P12WantedBusy)
		}
		// A killed/escaped instance frees only its unfinished presentation lease.
		// No shared HP, attempt, wallet, event or committed historical receipt changes.
		if err := tx.Model(&fight).Where("committed=? AND abandoned_at=0", false).Update("abandoned_at", now.UnixMilli()).Error; err != nil {
			return err
		}
	}
	return nil
}

// The detailed P6 mismatch can only be marked at the recorded-computation
// boundary. A similarly worded DB or network error stays unclassified.
type p12WantedRecordedReplayMismatch struct{ cause error }

func (e *p12WantedRecordedReplayMismatch) Error() string { return e.cause.Error() }
func (e *p12WantedRecordedReplayMismatch) Unwrap() error { return e.cause }

func p12WantedRecordedReplayError(err error) error {
	if err != nil && strings.HasPrefix(err.Error(), constUtil.P6ReplayConflict+": 原录像终局或完整技能记录不符 round=") {
		return &p12WantedRecordedReplayMismatch{cause: err}
	}
	return err
}

// Persist rejection metadata after the settlement transaction rolled back.
// Network/DB failures remain recoverable and do not release an active battle.
func recordP12WantedRejectedEnd(ctx context.Context, id int64, fightID string, now time.Time, rejected error) error {
	if fightID == "" || rejected == nil {
		return nil
	}
	reason := rejected.Error()
	var replayMismatch *p12WantedRecordedReplayMismatch
	if errors.As(rejected, &replayMismatch) {
		reason = constUtil.P6ReplayConflict
	}
	if reason != constUtil.P6ReplayConflict && reason != constUtil.P12WantedInvalid && reason != constUtil.CombatUnsupported {
		return nil
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var profile db.Profile
		if err := tx.Select("id").Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
			return err
		}
		query := tx.Model(&db.P12WantedFight{}).Where("id=? AND player_id=? AND committed=? AND abandoned_at=0", fightID, id, false)
		if calendar75Adjusted(tx) {
			var fight db.P12WantedFight
			err := tx.Select("id", "instance_id", "created_at").Where("id=? AND player_id=? AND committed=? AND abandoned_at=0", fightID, id, false).First(&fight).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			if fight.CreatedAt > now.UnixMilli() {
				return nil
			}
			var instance db.P12WantedInstance
			err = tx.Select("id", "created_at").First(&instance, "id=?", fight.InstanceID).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			if instance.CreatedAt > now.UnixMilli() {
				return nil
			}
		}
		return query.Updates(map[string]any{"rejected_at": now.UnixMilli(), "rejected_reason": reason}).Error
	})
}
