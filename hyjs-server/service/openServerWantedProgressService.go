package service

import (
	"errors"
	"fmt"
	"math"
	"time"

	"gorm.io/gorm"
	"hyjs-server/db"
)

type openServerWantedProgress struct {
	damage      int32
	performance int32
}

// Read the committed personal fights once. Daily performance follows the same
// UTC+8 day boundary as the Wanted standings, within this activity window.
func readOpenServerWantedProgress(tx *gorm.DB, id int64, cfg *db.OpenServerActivityRule, now time.Time) (openServerWantedProgress, error) {
	var facts []struct {
		CommittedAt   int64
		AppliedDamage float64
		Performance   int64
	}
	err := tx.Model(&db.P12WantedFight{}).
		Select("committed_at", "applied_damage", "performance").
		Where("player_id=? AND committed=? AND committed_at>=? AND committed_at<=? AND committed_at<?", id, true, cfg.StartAt, now.UnixMilli(), cfg.EndAt).
		Find(&facts).Error
	if err != nil {
		return openServerWantedProgress{}, fmt.Errorf("读取开服悬赏结算: %w", err)
	}
	var result openServerWantedProgress
	daily := make(map[string]int64)
	for _, fact := range facts {
		if fact.AppliedDamage < 0 || math.IsNaN(fact.AppliedDamage) || math.IsInf(fact.AppliedDamage, 0) || fact.Performance < 0 {
			return openServerWantedProgress{}, errors.New("开服悬赏结算存档无效")
		}
		result.damage = max(result.damage, int32(min(fact.AppliedDamage, float64(math.MaxInt32))))
		day, _, _ := p12WantedDay(time.UnixMilli(fact.CommittedAt))
		daily[day] = min(int64(math.MaxInt32), daily[day]+min(fact.Performance, int64(math.MaxInt32)))
		result.performance = max(result.performance, int32(daily[day]))
	}
	return result, nil
}
