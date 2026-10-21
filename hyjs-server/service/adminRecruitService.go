package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// AdminRecruitSettings 返回两项全局运行规则；读取权限沿用后台活动只读口径。
func (s *GameService) AdminRecruitSettings(ctx context.Context, actor *model.AdminPrincipal) (*model.AdminRecruitSettingsResponse, error) {
	tx := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(tx, actor, false); err != nil {
		return nil, err
	}
	if !tx.Migrator().HasTable(&db.RecruitRuntimeRule{}) {
		if err := s.InitializeRecruitRuntime(ctx); err != nil {
			return nil, err
		}
	}
	items, err := s.recruitRuntimeItems(tx)
	if err != nil {
		return nil, err
	}
	return &model.AdminRecruitSettingsResponse{Version: adminVersion(items), Items: items}, nil
}

// AdminSaveRecruitSettings 在事务内校验版本并保存两项规则，审计后立即对后续抽取生效。
// 不改原奖池、价格、保底奖励范围、免费冷却、历史quota或已提交回执。
func (s *GameService) AdminSaveRecruitSettings(ctx context.Context, actor *model.AdminPrincipal, req model.AdminRecruitSettingsSaveRequest) (*model.AdminRecruitSettingsResponse, error) {
	canonical, err := canonicalRecruitRuntimeItems(req.Items)
	if err != nil {
		return nil, err
	}
	if len(req.Version) != 64 || strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 1000 {
		return nil, constUtil.ErrAdminInput
	}
	if err = adminOperationUser(db.GetIns().WithContext(ctx), actor, true); err != nil {
		return nil, err
	}
	if !db.GetIns().Migrator().HasTable(&db.RecruitRuntimeRule{}) {
		if err = s.InitializeRecruitRuntime(ctx); err != nil {
			return nil, err
		}
	}
	var out *model.AdminRecruitSettingsResponse
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err = adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		// Lock the two kinds in stable order and version these locked rows.
		// Earlier consistent reads must not supply a stale InnoDB snapshot.
		var locked []db.RecruitRuntimeRule
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Order("kind").Find(&locked).Error; err != nil {
			return err
		}
		if len(locked) != 2 || locked[0].Kind != constUtil.RecruitRuntimeKindStrong || locked[1].Kind != constUtil.RecruitRuntimeKindKing {
			return constUtil.ErrAdminConfiguration
		}
		current := make([]model.AdminRecruitSetting, 0, len(locked))
		for _, row := range locked {
			current = append(current, recruitRuntimeSetting(row))
		}
		version := adminVersion(current)
		if version != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		for _, item := range canonical {
			updated := tx.Model(&db.RecruitRuntimeRule{}).Where("kind=?", item.Kind).Updates(map[string]any{"daily_limit": item.DailyLimit, "guarantee_every": item.GuaranteeEvery, "luck_exchange_cost": item.LuckExchangeCost})
			if updated.Error != nil {
				return updated.Error
			}
			// MySQL reports zero changed rows when an existing setting is unchanged.
			// The locked rows above have already established its existence.
		}
		next := &model.AdminRecruitSettingsResponse{Version: adminVersion(canonical), Items: canonical}
		if err = adminAudit(tx, actor, constUtil.RecruitRuntimeAuditAction, constUtil.RecruitRuntimeAuditTarget, fmt.Sprintf("before=%s after=%s; %s", adminJSON(current), adminJSON(canonical), strings.TrimSpace(req.Reason))); err != nil {
			return err
		}
		out = next
		return nil
	})
	return out, err
}

// canonicalRecruitRuntimeItems 校验完整的两条唯一kind，并把数值收敛到运行表可存范围。
func canonicalRecruitRuntimeItems(items []model.AdminRecruitSetting) ([]model.AdminRecruitSetting, error) {
	if len(items) != 2 {
		return nil, constUtil.ErrAdminInput
	}
	seen := map[int64]bool{}
	out := make([]model.AdminRecruitSetting, 0, len(items))
	for _, item := range items {
		if seen[item.Kind] {
			return nil, constUtil.ErrAdminInput
		}
		seen[item.Kind] = true
		name := constUtil.RecruitRuntimeName(int32(item.Kind))
		if name == "" || item.Name != name || item.GuaranteeEvery < int64(constUtil.RecruitRuntimeGuaranteeMin) || item.GuaranteeEvery > int64(constUtil.RecruitRuntimeGuaranteeMax) {
			return nil, constUtil.ErrAdminInput
		}
		switch item.Kind {
		case int64(constUtil.RecruitRuntimeKindStrong):
			if item.LuckExchangeCost != 0 || item.DailyLimit < int64(constUtil.RecruitRuntimeDailyUnlimited) || item.DailyLimit > int64(constUtil.RecruitRuntimeDailyMaximum) {
				return nil, constUtil.ErrAdminInput
			}
		case int64(constUtil.RecruitRuntimeKindKing):
			if item.LuckExchangeCost < 1 || item.LuckExchangeCost > math.MaxInt32 || item.DailyLimit < 1 || item.DailyLimit > int64(constUtil.RecruitRuntimeDailyMaximum) {
				return nil, constUtil.ErrAdminInput
			}
		default:
			return nil, constUtil.ErrAdminInput
		}
		out = append(out, model.AdminRecruitSetting{Kind: item.Kind, Name: name, DailyLimit: item.DailyLimit, GuaranteeEvery: item.GuaranteeEvery, LuckExchangeCost: item.LuckExchangeCost})
	}
	if !seen[int64(constUtil.RecruitRuntimeKindStrong)] || !seen[int64(constUtil.RecruitRuntimeKindKing)] {
		return nil, constUtil.ErrAdminInput
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out, nil
}

// adminSaveActivityRecruitSettings 把当前活动的全局招募规则纳入调用者事务。
// 双行固定顺序加锁并核全局版本；另一个招募类型必须与当前值完全一致。
func (s *GameService) adminSaveActivityRecruitSettings(tx *gorm.DB, actor *model.AdminPrincipal, kind int32, req *model.AdminRecruitSettingsSaveRequest) error {
	if req == nil {
		return nil
	}
	if kind != constUtil.RecruitRuntimeKindStrong && kind != constUtil.RecruitRuntimeKindKing {
		return constUtil.ErrAdminInput
	}
	items, err := canonicalRecruitRuntimeItems(req.Items)
	if err != nil {
		return err
	}
	if len(req.Version) != 64 || strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 1000 {
		return constUtil.ErrAdminInput
	}
	var locked []db.RecruitRuntimeRule
	if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Order("kind").Find(&locked).Error; err != nil {
		return err
	}
	if len(locked) != 2 || locked[0].Kind != constUtil.RecruitRuntimeKindStrong || locked[1].Kind != constUtil.RecruitRuntimeKindKing {
		return constUtil.ErrAdminConfiguration
	}
	current := make([]model.AdminRecruitSetting, 0, len(locked))
	for _, row := range locked {
		current = append(current, recruitRuntimeSetting(row))
	}
	if req.Version != adminVersion(current) {
		return constUtil.ErrAdminConfigConflict
	}
	var selected model.AdminRecruitSetting
	for i, item := range items {
		if item.Kind != int64(kind) {
			if item != current[i] {
				return constUtil.ErrAdminInput
			}
			continue
		}
		selected = item
	}
	if err = tx.Model(&db.RecruitRuntimeRule{}).Where("kind=?", kind).Updates(map[string]any{"daily_limit": selected.DailyLimit, "guarantee_every": selected.GuaranteeEvery, "luck_exchange_cost": selected.LuckExchangeCost}).Error; err != nil {
		return err
	}
	return adminAudit(tx, actor, constUtil.RecruitRuntimeAuditAction, constUtil.RecruitRuntimeAuditTarget, fmt.Sprintf("activity kind=%d daily=%d guarantee=%d luckExchangeCost=%d; %s", kind, selected.DailyLimit, selected.GuaranteeEvery, selected.LuckExchangeCost, strings.TrimSpace(req.Reason)))
}
