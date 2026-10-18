package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"regexp"
	"strings"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// AdminCreateActivity 从已存在同类模板建立独立ID，旧Frozen和玩家快照不会被写入。
// 新ID保留整个模板前缀，确保已有客户端和服务端前缀路由仍可识别同类入口。
func (s *GameService) AdminCreateActivity(ctx context.Context, p *model.AdminPrincipal, req model.AdminActivityCreateRequest) (*model.AdminActivityDTO, error) {
	domain, template, ok := strings.Cut(req.TemplateID, ":")
	key := strings.TrimPrefix(req.NewID, domain+":")
	if !ok || key == template || len(key) > 64 || !strings.HasPrefix(key, template+"_") || !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]*$`).MatchString(key) {
		return nil, constUtil.ErrAdminInput
	}
	var saved model.AdminActivityDTO
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if err := adminOperationUser(tx, p, true); err != nil {
			return err
		}
		// 所有新配置仅Create；同ID已存在时返回冲突，不能覆盖历史或并发创建结果。
		absent := func(table any) error {
			var count int64
			if err := tx.Model(table).Where("id = ?", key).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return constUtil.ErrAdminConfigConflict
			}
			return nil
		}
		switch domain {
		case "commerce":
			var old db.P4CommerceRule
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&old, "id = ?", template).Error; err != nil {
				return err
			}
			if err := absent(&db.P4CommerceRule{}); err != nil {
				return err
			}
			var cfg db.P4CommerceRule
			if adminDecodeActivityRule(req.Config, &cfg) != nil || cfg.ID != key || cfg.Kind != old.Kind || cfg.ShopType != old.ShopType || cfg.Source == "" || cfg.StartAt <= 0 || cfg.EndAt <= cfg.StartAt || cfg.ClaimEndAt < cfg.EndAt {
				return constUtil.ErrAdminInput
			}
			cfg.Enabled = req.Enabled
			cfg.Frozen = nil
			var raw p4CommerceRules
			if json.Unmarshal(cfg.Rules, &raw) != nil {
				return constUtil.ErrAdminInput
			}
			if req.Enabled || len(raw.Products) > 0 {
				if _, err := s.validateP4Commerce(cfg); err != nil {
					return adminActivityValidationError(err)
				}
			}
			if err := tx.Create(&cfg).Error; err != nil {
				return err
			}
			saved = adminActivityProjection(domain, cfg, nil)
		case "welfare":
			var old db.P4WelfareRule
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&old, "id = ?", template).Error; err != nil {
				return err
			}
			if err := absent(&db.P4WelfareRule{}); err != nil {
				return err
			}
			var cfg db.P4WelfareRule
			if adminDecodeActivityRule(req.Config, &cfg) != nil || cfg.ID != key || cfg.Kind != old.Kind || cfg.Source == "" || cfg.StartAt <= 0 || cfg.EndAt <= cfg.StartAt || cfg.ClaimEndAt < cfg.EndAt {
				return constUtil.ErrAdminInput
			}
			cfg.Enabled = req.Enabled
			cfg.Frozen = nil
			var raw map[string]json.RawMessage
			if json.Unmarshal(cfg.Rules, &raw) != nil {
				return constUtil.ErrAdminInput
			}
			if req.Enabled || len(raw) > 0 {
				if _, err := s.validateP4Welfare(cfg); err != nil {
					return adminActivityValidationError(err)
				}
			}
			if err := tx.Create(&cfg).Error; err != nil {
				return err
			}
			saved = adminActivityProjection(domain, cfg, nil)
		case "period":
			var old db.PeriodInstance
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&old, "id = ?", template).Error; err != nil {
				return err
			}
			if err := absent(&db.PeriodInstance{}); err != nil {
				return err
			}
			var cfg db.PeriodInstance
			if json.Unmarshal(req.Config, &cfg) != nil || cfg.ID != key || cfg.Kind != old.Kind || cfg.Mode != old.Mode || cfg.Source == "" || cfg.Title == "" || cfg.StartAt <= 0 || cfg.EndAt <= cfg.StartAt || cfg.RewardEndAt < cfg.EndAt || cfg.Goal <= 0 || cfg.Goal > math.MaxInt32 || cfg.RankPlaces < 0 || cfg.RankPlaces > 1000 {
				return constUtil.ErrAdminInput
			}
			cfg.Enabled = req.Enabled
			cfg.Frozen = nil
			if req.Enabled || cfg.Rewards != "" {
				if err := s.validateP1Period(cfg); err != nil {
					return adminActivityValidationError(err)
				}
			}
			if err := tx.Create(&cfg).Error; err != nil {
				return err
			}
			saved = adminActivityProjection(domain, cfg, nil)
		case "pool":
			var old db.GameplayPool
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&old, "id = ?", template).Error; err != nil {
				return err
			}
			if err := absent(&db.GameplayPool{}); err != nil {
				return err
			}
			var cfg struct {
				Pool    db.GameplayPool         `json:"pool"`
				Rewards []db.GameplayPoolReward `json:"rewards"`
			}
			if json.Unmarshal(req.Config, &cfg) != nil || cfg.Pool.ID != key || cfg.Pool.Name == "" || len(cfg.Pool.Name) > 128 || cfg.Pool.Source == "" || len(cfg.Pool.Source) > 255 || cfg.Pool.MinLevel < 1 || len(cfg.Rewards) > 200 {
				return constUtil.ErrAdminInput
			}
			if cfg.Pool.StartAt < 0 || cfg.Pool.EndAt <= cfg.Pool.StartAt || cfg.Pool.RewardEndAt < cfg.Pool.EndAt || cfg.Pool.InitialDiamondPool < 0 || cfg.Pool.JackpotPerMillion < 0 || cfg.Pool.JackpotPerMillion > 1000000 || cfg.Pool.PoolContributionBps < 0 || cfg.Pool.PoolContributionBps > 10000 || cfg.Pool.FreePerDay < -1 || cfg.Pool.FreeSeconds < 1 || cfg.Pool.FreeSeconds > 31536000 || cfg.Pool.GuaranteeEvery < 0 || cfg.Pool.GuaranteeEvery > 10000 || cfg.Pool.CoinPerDraw < 0 {
				return constUtil.ErrAdminInput
			}
			var wheel db.TurntableRules
			wheelErr := gorm.ErrRecordNotFound
			if tx.Migrator().HasTable(&db.TurntableRules{}) {
				wheelErr = tx.First(&wheel, "pool_id = ?", template).Error
			}
			if wheelErr != nil && !errors.Is(wheelErr, gorm.ErrRecordNotFound) {
				return wheelErr
			}
			seen := map[int32]bool{}
			for i, reward := range cfg.Rewards {
				if reward.ID < 0 || seen[reward.ID] || reward.Amount < 1 || reward.Weight < 1 || reward.Weight > 1000000000000 || reward.Stock < 0 {
					return constUtil.ErrAdminInput
				}
				seen[reward.ID] = true
				valid := s.turntableResource(reward.ResourceID)
				if wheelErr != nil {
					var err error
					valid, err = s.openServerRewardSupported(fmt.Sprintf("%s,%d", reward.ResourceID, reward.Amount))
					if err != nil {
						return constUtil.ErrAdminInput
					}
				}
				if !valid {
					return constUtil.ErrAdminInput
				}
				cfg.Rewards[i].PoolID = key
			}
			for _, cost := range []string{cfg.Pool.SingleCost, cfg.Pool.TenCost, cfg.Pool.SingleTicket, cfg.Pool.TenTicket} {
				if cost != "" {
					if _, err := parsePoolCost(cost); err != nil {
						return constUtil.ErrAdminInput
					}
				}
			}
			if req.Enabled && len(cfg.Rewards) == 0 {
				return constUtil.ErrAdminInput
			}
			// 临时Enabled只对当前事务的验证可见，提交前恢复真实请求开关。
			cfg.Pool.Enabled = true
			if err := tx.Create(&cfg.Pool).Error; err != nil {
				return err
			}
			if len(cfg.Rewards) > 0 {
				if err := tx.Create(&cfg.Rewards).Error; err != nil {
					return err
				}
			}
			if wheelErr == nil {
				wheel.PoolID = key
				if err := tx.Create(&wheel).Error; err != nil {
					return err
				}
			}
			if len(cfg.Rewards) > 0 {
				if wheelErr == nil {
					if _, _, _, err := s.turntableConfigForPool(tx, key); err != nil {
						return adminActivityValidationError(err)
					}
				} else {
					if _, _, err := s.loadGameplayPool(tx, key); err != nil {
						return adminActivityValidationError(err)
					}
				}
			}
			cfg.Pool.Enabled = req.Enabled
			if err := tx.Model(&db.GameplayPool{}).Where("id = ?", key).Update("enabled", req.Enabled).Error; err != nil {
				return err
			}
			saved = adminActivityProjection(domain, cfg.Pool, cfg.Rewards)
		default:
			return constUtil.ErrAdminInput
		}
		return adminAudit(tx, p, "activity.create", saved.ID, "template="+req.TemplateID+" version="+saved.Version)
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

// adminActivityValidationError 将原业务的普通配置错误转为400语义，保留连接/查询故障供500处理。
func adminActivityValidationError(err error) error {
	if err == nil {
		return nil
	}
	var mysqlError *mysqlDriver.MySQLError
	var networkError net.Error
	if errors.As(err, &networkError) || strings.HasPrefix(err.Error(), "sql:") || strings.HasPrefix(err.Error(), "driver:") || errors.As(err, &mysqlError) || db.IsSQLiteDriverError(err) || errors.Is(err, sql.ErrConnDone) || errors.Is(err, sql.ErrTxDone) || errors.Is(err, driver.ErrBadConn) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, gorm.ErrInvalidDB) || errors.Is(err, gorm.ErrInvalidTransaction) {
		return err
	}
	return fmt.Errorf("%w: %v", constUtil.ErrAdminInput, err)
}
