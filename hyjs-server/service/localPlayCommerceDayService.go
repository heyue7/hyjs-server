package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"hyjs-server/db"
)

type localPlayCommerceDailyQuota struct {
	Bought       map[string]int32 `json:"bought"`
	ResourceUsed int32            `json:"resourceUsed"`
}

// The original current-day cache remains the authority. On calendar switches,
// archive that committed cache and restore this date's historical quota. Thus
// changing the demo clock cannot erase a day's purchases or refresh cost cap.
func localPlayCommerceDay(tx *gorm.DB, row *db.P4CommerceState, state *p4CommerceProgress, now time.Time) error {
	today := p1Day(now)
	if state.Day == today {
		return nil
	}
	if localPlayActivityOwner(tx) != row.PlayerID {
		state.Day = today
		state.Bought = map[string]int32{}
		state.ResourceUsed = 0
		return nil
	}
	if state.Day != "" {
		if err := localPlayCommerceDailySave(tx, row, state.Day, state); err != nil {
			return err
		}
	}
	digest := sha256.Sum256([]byte(row.RuleID))
	key := "commerce-day:" + hex.EncodeToString(digest[:16]) + ":" + today
	var prior db.LocalPlayActivityOperation
	err := tx.First(&prior, "player_id=? AND request_id=?", row.PlayerID, key).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	quota := localPlayCommerceDailyQuota{Bought: map[string]int32{}}
	if err == nil {
		if prior.Action != "commerce-day" || prior.ActivityID != row.RuleID || json.Unmarshal(prior.Response, &quota) != nil || quota.ResourceUsed < 0 {
			return errLocalPlayActivitiesInvalid
		}
		for _, count := range quota.Bought {
			if count < 0 {
				return errLocalPlayActivitiesInvalid
			}
		}
		if quota.Bought == nil {
			quota.Bought = map[string]int32{}
		}
	}
	state.Day = today
	state.Bought = quota.Bought
	state.ResourceUsed = quota.ResourceUsed
	return nil
}

func localPlayCommerceDailySave(tx *gorm.DB, row *db.P4CommerceState, day string, state *p4CommerceProgress) error {
	digest := sha256.Sum256([]byte(row.RuleID))
	key := "commerce-day:" + hex.EncodeToString(digest[:16]) + ":" + day
	raw, err := json.Marshal(localPlayCommerceDailyQuota{Bought: state.Bought, ResourceUsed: state.ResourceUsed})
	if err != nil {
		return err
	}
	operation := db.LocalPlayActivityOperation{PlayerID: row.PlayerID, RequestID: key, Action: "commerce-day", ActivityID: row.RuleID, Response: raw, CreatedAt: time.Now().UnixMilli()}
	return tx.Save(&operation).Error
}
