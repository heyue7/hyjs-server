package service

import (
	"context"
	"errors"
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

// Persist rejection metadata after the settlement transaction rolled back.
// Network/DB failures remain recoverable and do not release an active battle.
func recordP12WantedRejectedEnd(ctx context.Context, id int64, fightID string, now time.Time, rejected error) error {
	if fightID == "" || rejected == nil {
		return nil
	}
	reason := rejected.Error()
	if reason != constUtil.P6ReplayConflict && reason != constUtil.P12WantedInvalid && reason != constUtil.CombatUnsupported {
		return nil
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var profile db.Profile
		if err := tx.Select("id").Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
			return err
		}
		return tx.Model(&db.P12WantedFight{}).Where("id=? AND player_id=? AND committed=? AND abandoned_at=0", fightID, id, false).Updates(map[string]any{"rejected_at": now.UnixMilli(), "rejected_reason": reason}).Error
	})
}
