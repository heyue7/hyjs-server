package service

import (
	"context"
	"fmt"
	"math"
	"strings"
	"unicode"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

func adminDailyTask69Name(kind int32) string {
	switch kind {
	case constUtil.DailyTaskCopy:
		return "副本通关"
	case constUtil.DailyTaskRecruit:
		return "伙伴招募"
	case constUtil.DailyTaskEquipStrengthen:
		return "装备强化"
	case constUtil.DailyTaskTreasureStrengthen:
		return "宝物强化"
	case constUtil.DailyTaskTraining:
		return "伙伴培养"
	case constUtil.DailyTaskCombine:
		return "合成"
	case constUtil.DailyTaskEquipRefine:
		return "装备精炼"
	}
	return "未知任务"
}

func adminDailyTask69Read(tx *gorm.DB) (*model.AdminDailyTask69Response, []db.DailyTaskRule, error) {
	var rows []db.DailyTaskRule
	if err := tx.Order("id").Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	if len(rows) != constUtil.AdminDailyTask69Count {
		return nil, nil, constUtil.ErrAdminConfiguration
	}
	out := &model.AdminDailyTask69Response{Version: adminVersion(rows), Items: []model.AdminDailyTask69Item{}}
	for _, row := range rows {
		out.Items = append(out.Items, model.AdminDailyTask69Item{ID: row.ID, Name: adminDailyTask69Name(row.Type), Type: row.Type, Total: row.Total, Score: row.Score, MinLevel: row.MinLevel, Enabled: row.Enabled, Reward: adminTypedResourceOne(row.Reward)})
	}
	return out, rows, nil
}

// AdminDailyTasks69 复用实时后台权限读取配置。
func (s *GameService) AdminDailyTasks69(ctx context.Context, actor *model.AdminPrincipal) (*model.AdminDailyTask69Response, error) {
	tx := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(tx, actor, false); err != nil {
		return nil, err
	}
	out, _, err := adminDailyTask69Read(tx)
	return out, err
}

// AdminDailyTask69InputValid 同时保护HTTP及内部调用的边界。
func AdminDailyTask69InputValid(req model.AdminDailyTask69SaveRequest) bool {
	if len(req.Version) != 64 || strings.TrimSpace(req.Reason) == "" || len([]rune(req.Reason)) > 500 || len(req.Items) != constUtil.AdminDailyTask69Count {
		return false
	}
	seen := map[string]bool{}
	for _, item := range req.Items {
		if item.ID == "" || len(item.ID) > 64 || seen[item.ID] || item.Reward.ResourceID == "" || len(item.Reward.ResourceID) > 128 || strings.ContainsAny(item.Reward.ResourceID, ",;") || strings.IndexFunc(item.Reward.ResourceID, unicode.IsSpace) >= 0 || item.Reward.Amount < 1 || item.Reward.Amount > math.MaxInt32 {
			return false
		}
		seen[item.ID] = true
	}
	return true
}

// AdminSaveDailyTasks69 只更新规则奖励，既有日档和领取状态保持冻结。
func (s *GameService) AdminSaveDailyTasks69(ctx context.Context, actor *model.AdminPrincipal, req model.AdminDailyTask69SaveRequest) (*model.AdminDailyTask69Response, error) {
	if !AdminDailyTask69InputValid(req) {
		return nil, constUtil.ErrAdminInput
	}
	var out *model.AdminDailyTask69Response
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		before, rows, err := adminDailyTask69Read(tx.Clauses(clause.Locking{Strength: "UPDATE"}))
		if err != nil {
			return err
		}
		if req.Version != before.Version {
			return constUtil.ErrAdminConfigConflict
		}
		byID := map[string]db.DailyTaskRule{}
		for _, row := range rows {
			byID[row.ID] = row
		}
		values := map[string]string{}
		for _, item := range req.Items {
			if _, ok := byID[item.ID]; !ok {
				return constUtil.ErrAdminInput
			}
			value := fmt.Sprintf("%s,%d", item.Reward.ResourceID, item.Reward.Amount)
			if err := s.dailyTaskReward(value, true); err != nil {
				return constUtil.ErrAdminInput
			}
			values[item.ID] = value
		}
		changed := false
		for _, row := range rows {
			if row.Reward == values[row.ID] {
				continue
			}
			changed = true
			if err := tx.Model(&db.DailyTaskRule{}).Where("id=?", row.ID).Update("reward", values[row.ID]).Error; err != nil {
				return err
			}
		}
		if changed {
			detail := fmt.Sprintf("reason=%s; before=%s; rewards=%s; frozen daily states retained", strings.TrimSpace(req.Reason), before.Version, string(adminJSON(req.Items)))
			if err := adminAudit(tx, actor, constUtil.AdminDailyTask69AuditAction, constUtil.AdminDailyTask69AuditTarget, detail); err != nil {
				return err
			}
		}
		out, _, err = adminDailyTask69Read(tx)
		return err
	})
	return out, err
}
