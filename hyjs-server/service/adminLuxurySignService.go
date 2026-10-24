package service

import (
	"context"
	"errors"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"math"
	"strconv"
	"time"
)

func (s *GameService) adminLuxurySignView(tx *gorm.DB) (*model.AdminLuxurySignRulesResponse, error) {
	tx = tx.Session(&gorm.Session{})
	var rule db.ActivityRules
	err := tx.Where("id=?", constUtil.P3LuxurySignActivityID).First(&rule).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	var rewards []db.ActivityReward
	if err := tx.Where("activity_id=? AND reward_type=?", constUtil.P3LuxurySignActivityID, constUtil.P3LuxurySignRewardType).Order("threshold").Find(&rewards).Error; err != nil {
		return nil, err
	}
	if len(rewards) > 1 {
		return nil, constUtil.ErrAdminConfiguration
	}
	need, err := strconv.ParseFloat(s.table("GameConfigure", "StaticName", "LuxurySignRechargeAmount")["NameValue"], 64)
	if err != nil || math.IsNaN(need) || math.IsInf(need, 0) || need <= 0 || need*100 > math.MaxInt32 {
		return nil, constUtil.ErrAdminConfiguration
	}
	out := &model.AdminLuxurySignRulesResponse{Version: adminVersion([]any{rule, rewards}), Enabled: rule.Enabled, Rewards: []model.AdminResourceAmount{}, RechargeCents: int64(math.Ceil(need * 100)), StartAt: rule.StartAt, EndAt: rule.EndAt}
	if len(rewards) == 1 {
		out.Rewards = adminTypedResources(rewards[0].Reward)
	}
	return out, nil
}

// AdminLuxurySignRules 读取固定豪华签到，不创建规则或玩家记录。
func (s *GameService) AdminLuxurySignRules(ctx context.Context, actor *model.AdminPrincipal) (*model.AdminLuxurySignRulesResponse, error) {
	tx := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(tx, actor, false); err != nil {
		return nil, err
	}
	return s.adminLuxurySignView(tx)
}

// AdminSaveLuxurySignRules 保留原支付资格与领取历史，只更新固定奖励源。
func (s *GameService) AdminSaveLuxurySignRules(ctx context.Context, actor *model.AdminPrincipal, req model.AdminLuxurySignRulesSaveRequest) (*model.AdminLuxurySignRulesResponse, error) {
	var out *model.AdminLuxurySignRulesResponse
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		current, err := s.adminLuxurySignView(tx.Clauses(clause.Locking{Strength: "UPDATE"}))
		if err != nil {
			return err
		}
		if current.Version != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		if req.Enabled && len(req.Rewards) == 0 {
			return constUtil.ErrAdminInput
		}
		content, err := s.adminTypedResourceSpec(req.Rewards, true)
		if err != nil {
			return err
		}
		if len(content) > 512 {
			return constUtil.ErrAdminInput
		}
		if content != "" {
			if _, err = activityLoginResources(content); err != nil {
				return constUtil.ErrAdminInput
			}
			supported, e := s.openServerRewardSupported(content)
			if e != nil {
				return e
			}
			if !supported {
				return constUtil.ErrAdminInput
			}
		}
		var rule db.ActivityRules
		err = tx.Where("id=?", constUtil.P3LuxurySignActivityID).First(&rule).Error
		missing := errors.Is(err, gorm.ErrRecordNotFound)
		if err != nil && !missing {
			return err
		}
		detail := "更新豪华签到启停与每日奖励；保留领取历史"
		if missing {
			rule = db.ActivityRules{ID: constUtil.P3LuxurySignActivityID, StartAt: 1, EndAt: constUtil.AdminPermanentActivityEndAt, RewardEndAt: constUtil.AdminPermanentActivityEndAt, Source: "运营配置"}
		}
		if !missing && req.Enabled && rule.EndAt <= time.Now().UnixMilli() {
			rule.StartAt = 1
			rule.EndAt = constUtil.AdminPermanentActivityEndAt
			rule.RewardEndAt = constUtil.AdminPermanentActivityEndAt
			detail = "过期豪华签到改为常驻开放；保留领取历史"
		}
		if missing || rule.Enabled != req.Enabled || rule.StartAt != current.StartAt || rule.EndAt != current.EndAt {
			rule.Enabled = req.Enabled
			if err = tx.Save(&rule).Error; err != nil {
				return err
			}
		}
		var reward db.ActivityReward
		err = tx.Where("activity_id=? AND reward_type=?", constUtil.P3LuxurySignActivityID, constUtil.P3LuxurySignRewardType).First(&reward).Error
		missingReward := errors.Is(err, gorm.ErrRecordNotFound)
		if err != nil && !missingReward {
			return err
		}
		if missingReward && content != "" {
			reward = db.ActivityReward{ActivityID: constUtil.P3LuxurySignActivityID, RewardType: constUtil.P3LuxurySignRewardType, Reward: content}
			if err = tx.Create(&reward).Error; err != nil {
				return err
			}
		} else if !missingReward && reward.Reward != content {
			if err = tx.Model(&reward).Update("reward", content).Error; err != nil {
				return err
			}
		}
		if err = adminAudit(tx, actor, "luxury-sign.save", constUtil.P3LuxurySignActivityID, detail); err != nil {
			return err
		}
		out, err = s.adminLuxurySignView(tx)
		return err
	})
	// Fixed-source rows may be absent on the first edit. InnoDB gap-lock or
	// unique-key competition rolls the transaction back and asks the editor to reload.
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) && (mysqlErr.Number == 1062 || mysqlErr.Number == 1213) {
		return nil, constUtil.ErrAdminConfigConflict
	}
	for cause := err; cause != nil; cause = errors.Unwrap(cause) {
		if db.IsSQLiteDriverError(cause) && cause.Error() == "database is locked" {
			return nil, constUtil.ErrAdminConfigConflict
		}
	}
	return out, err
}
