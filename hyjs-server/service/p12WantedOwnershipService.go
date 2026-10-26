package service

import (
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
