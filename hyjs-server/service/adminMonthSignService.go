package service

import (
	"context"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"strings"
)

func adminMonthSignView(tx *gorm.DB) (*model.AdminMonthSignRulesResponse, error) {
	var rows []db.MonthSignRule
	if err := tx.Order("day").Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) != 31 {
		return nil, constUtil.ErrAdminConfiguration
	}
	out := &model.AdminMonthSignRulesResponse{Version: adminVersion(rows), Enabled: true, Rows: []model.AdminMonthSignDay{}}
	sources := []string{}
	for i, row := range rows {
		if row.Day != int32(i+1) {
			return nil, constUtil.ErrAdminConfiguration
		}
		out.Enabled = out.Enabled && row.Enabled
		if len(sources) == 0 || sources[len(sources)-1] != row.Source {
			sources = append(sources, row.Source)
		}
		out.Rows = append(out.Rows, model.AdminMonthSignDay{Day: int64(row.Day), Reward: adminTypedResources(row.Reward), DoubleVIP: int64(row.DoubleVIP)})
	}
	out.Source = strings.Join(sources, "；")
	return out, nil
}

// AdminMonthSignRules 只读当前全局奖历，不创建领取状态。
func (s *GameService) AdminMonthSignRules(ctx context.Context, actor *model.AdminPrincipal) (*model.AdminMonthSignRulesResponse, error) {
	tx := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(tx, actor, false); err != nil {
		return nil, err
	}
	return adminMonthSignView(tx)
}

// AdminSaveMonthSignRules 原子保存31档；已签到日期继续使用原LastReward快照。
func (s *GameService) AdminSaveMonthSignRules(ctx context.Context, actor *model.AdminPrincipal, req model.AdminMonthSignRulesSaveRequest) (*model.AdminMonthSignRulesResponse, error) {
	var out *model.AdminMonthSignRulesResponse
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		current, err := adminMonthSignView(tx.Clauses(clause.Locking{Strength: "UPDATE"}))
		if err != nil {
			return err
		}
		if current.Version != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		for _, day := range req.Rows {
			content, err := s.adminTypedResourceSpec(day.Reward, false)
			if err != nil {
				return err
			}
			supported, err := s.openServerRewardSupported(content)
			if err != nil {
				return err
			}
			if !supported {
				return constUtil.ErrAdminInput
			}
			if err = tx.Model(&db.MonthSignRule{}).Where("day=?", day.Day).Updates(map[string]any{"reward": content, "double_vip": day.DoubleVIP, "enabled": req.Enabled, "source": "运营配置；保留已领取日期快照"}).Error; err != nil {
				return err
			}
		}
		if err = adminAudit(tx, actor, "month-sign.save", "month-sign", "更新31档固定奖历；保留原领取历史"); err != nil {
			return err
		}
		out, err = adminMonthSignView(tx)
		return err
	})
	return out, err
}
