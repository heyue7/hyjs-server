package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// InitializeGrowthRecruit only installs the policy. Publication is a separate guarded operator action.
func (s *GameService) InitializeGrowthRecruit(ctx context.Context) error {
	if err := db.GetIns().WithContext(ctx).AutoMigrate(&db.GrowthRecruitPolicy{}); err != nil {
		return err
	}
	policy := db.GrowthRecruitPolicy{ID: 1, Enabled: false, Version: constUtil.GrowthRecruitVersion, Source: constUtil.GrowthRecruitSource}
	return db.GetIns().WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&policy).Error
}

// growthRecruitPublication keeps pre-policy deployments compatible and rejects malformed registered policy.
func growthRecruitPublication(tx *gorm.DB) (enabled, registered bool, err error) {
	if !gameSchemaHasTable(tx, &db.GrowthRecruitPolicy{}) {
		return false, false, nil
	}
	var policy db.GrowthRecruitPolicy
	if err = tx.First(&policy, 1).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if policy.Version != constUtil.GrowthRecruitVersion || policy.Source != constUtil.GrowthRecruitSource {
		return false, true, errors.New("人物获取政策版本不匹配")
	}
	return policy.Enabled, true, nil
}

// GrowthRecruitPublishCurrent is called only after the deployment tool has verified the running candidate.
// Repeating publication preserves a subsequent manual stop; it never resets prices, frozen bytes or quotas.
func (s *GameService) GrowthRecruitPublishCurrent(ctx context.Context, now time.Time) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var policy db.GrowthRecruitPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
			return err
		}
		if _, _, err := growthRecruitPublication(tx); err != nil {
			return err
		}
		if policy.PublishedAt > 0 {
			return nil
		}
		if err := s.seedP15RedOSSupply(tx, now); err != nil {
			return err
		}
		key := constUtil.P15RedOSSupplyPrefix + now.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
		var live db.P4CommerceRule
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&live, "id=?", key).Error; err != nil {
			return err
		}
		if live.Source != constUtil.P15RedOSSupplySource || live.Kind != "exchange" || live.ShopType != 0 {
			return errors.New("三红供应目录来源不匹配，保留运营配置")
		}
		if _, _, err := s.p4CommerceSnapshot(live); err != nil {
			return err
		}
		if err := tx.Model(&live).Update("enabled", true).Error; err != nil {
			return err
		}
		return tx.Model(&policy).Updates(map[string]any{"enabled": true, "published_at": now.UnixMilli()}).Error
	})
}

type growthRecruitDailyQuota struct {
	Count int32 `json:"count"`
}

// Archive a committed date before changing the current cache. These rows share
// the authenticated owner's profile transaction, preserving quotas on rewind.
func growthRecruitCalendarCount(tx *gorm.DB, id int64, kind, previousDay string, now time.Time, count int32) (int32, error) {
	day := p1Day(now)
	if count < 0 {
		return 0, errors.New(constUtil.GrowthRecruitCalendarInvalid)
	}
	if previousDay == day {
		return count, nil
	}
	if localPlayActivityOwner(tx) != id || id <= 1 {
		return 0, nil
	}
	key := func(date string) string { return "growth-recruit-day:" + kind + ":" + date }
	if previousDay != "" {
		raw, err := json.Marshal(growthRecruitDailyQuota{Count: count})
		if err != nil {
			return 0, err
		}
		prior := db.LocalPlayActivityOperation{PlayerID: id, RequestID: key(previousDay), Action: "recruit-day", ActivityID: kind, Response: raw, CreatedAt: time.Now().UnixMilli()}
		if err := tx.Save(&prior).Error; err != nil {
			return 0, err
		}
	}
	var prior db.LocalPlayActivityOperation
	err := tx.First(&prior, "player_id=? AND request_id=?", id, key(day)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var quota growthRecruitDailyQuota
	if prior.Action != "recruit-day" || prior.ActivityID != kind || json.Unmarshal(prior.Response, &quota) != nil || quota.Count < 0 {
		return 0, errors.New(constUtil.GrowthRecruitCalendarInvalid)
	}
	return quota.Count, nil
}

func growthRecruitState(tx *gorm.DB, id int64, g *model.GameState, kind int32, now time.Time) (*model.RecruitState, error) {
	state := recruitStoredState(g, kind)
	used, err := growthRecruitCalendarCount(tx, id, "recruit_"+strconv.FormatInt(int64(kind), 10), state.Day, now, state.FreeUsed)
	if err != nil {
		return nil, err
	}
	state.Day, state.FreeUsed = p1Day(now), used
	return state, nil
}

// growthRecruitDrawState 单独归档强者招募每日抽数；键与免费次数、王者次数互不影响。
func growthRecruitDrawState(tx *gorm.DB, id int64, g *model.GameState, kind int32, now time.Time) (*model.RecruitState, error) {
	state := recruitStoredState(g, kind)
	used, err := growthRecruitCalendarCount(tx, id, "growth-recruit-draws:"+strconv.FormatInt(int64(kind), 10), state.DrawDay, now, state.Draws)
	if err != nil {
		return nil, err
	}
	state.DrawDay, state.Draws = p1Day(now), used
	return state, nil
}

// growthRecruitKingDay is called after the original king state is loaded. Luck,
// guarantee and free-cooldown progress keep their original global lifecycle.
func growthRecruitKingDay(tx *gorm.DB, state *db.KingRecruitState, now time.Time) error {
	draws, err := growthRecruitCalendarCount(tx, state.PlayerID, "king", state.Day, now, state.Draws)
	if err != nil {
		return err
	}
	state.Day, state.Draws = p1Day(now), draws
	return nil
}

// p15RedOSSupplyAllowed is also used by list/trade consumers so a policy stop closes existing bindings.
func p15RedOSSupplyAllowed(tx *gorm.DB) (bool, error) {
	enabled, registered, err := growthRecruitPublication(tx)
	return !registered || enabled, err
}
