package service

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

// InitializeRecruitRuntime 幂等建立全局运行规则表，并按原静态配置补齐缺失项。
// 已有运营覆盖始终保留，原奖池、P2规则、价格、历史次数与回执不改写。
func (s *GameService) InitializeRecruitRuntime(ctx context.Context) error {
	if err := db.GetIns().WithContext(ctx).AutoMigrate(&db.RecruitRuntimeRule{}); err != nil {
		return err
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.seedRecruitRuntimeRules(tx)
	})
}

// seedRecruitRuntimeRules 从原静态快照复制默认值；只补缺失kind，不覆盖后台调整。
func (s *GameService) seedRecruitRuntimeRules(tx *gorm.DB) error {
	if err := s.seedRecruitRuntimeStrong(tx); err != nil {
		return err
	}
	return s.seedRecruitRuntimeKing(tx)
}

func (s *GameService) seedRecruitRuntimeStrong(tx *gorm.DB) error {
	var cfg db.GameplayPool
	err := tx.First(&cfg, "id=?", "recruit_2").Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	row := db.RecruitRuntimeRule{Kind: constUtil.RecruitRuntimeKindStrong, DailyLimit: constUtil.RecruitRuntimeDailyUnlimited, GuaranteeEvery: cfg.GuaranteeEvery}
	if err = validateRecruitRuntimeRule(row); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
}

func (s *GameService) seedRecruitRuntimeKing(tx *gorm.DB) error {
	var cfg db.GameplayPool
	err := tx.First(&cfg, "id=?", "recruit_3").Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	var rules db.KingRecruitRules
	if err = tx.First(&rules, 1).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	row := db.RecruitRuntimeRule{Kind: constUtil.RecruitRuntimeKindKing, DailyLimit: rules.DailyLimit, GuaranteeEvery: cfg.GuaranteeEvery}
	if err = validateRecruitRuntimeRule(row); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
}

// validateRecruitRuntimeRule 限制后台可表达的范围，避免零保底或负次数进入抽卡路径。
func validateRecruitRuntimeRule(row db.RecruitRuntimeRule) error {
	switch row.Kind {
	case constUtil.RecruitRuntimeKindStrong:
		if row.DailyLimit < constUtil.RecruitRuntimeDailyUnlimited || row.DailyLimit > constUtil.RecruitRuntimeDailyMaximum {
			return errors.New("强者招募每日次数超出范围")
		}
	case constUtil.RecruitRuntimeKindKing:
		if row.DailyLimit < 1 || row.DailyLimit > constUtil.RecruitRuntimeDailyMaximum {
			return errors.New("王者招募每日次数超出范围")
		}
	default:
		return errors.New("招募运行规则类型无效")
	}
	if row.GuaranteeEvery < constUtil.RecruitRuntimeGuaranteeMin || row.GuaranteeEvery > constUtil.RecruitRuntimeGuaranteeMax {
		return errors.New("招募保底间隔超出范围")
	}
	return nil
}

// recruitRuntimeRule 在配置表不存在或缺少该kind时返回nil，保持既有小夹具的原行为。
func (s *GameService) recruitRuntimeRule(tx *gorm.DB, kind int32) (*db.RecruitRuntimeRule, error) {
	if kind != constUtil.RecruitRuntimeKindStrong && kind != constUtil.RecruitRuntimeKindKing {
		return nil, nil
	}
	if !tx.Migrator().HasTable(&db.RecruitRuntimeRule{}) {
		return nil, nil
	}
	var row db.RecruitRuntimeRule
	err := tx.First(&row, "kind=?", kind).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = validateRecruitRuntimeRule(row); err != nil {
		return nil, err
	}
	return &row, nil
}

// applyRecruitRuntimeGuarantee 只修改本次返回的奖池副本，不写回原静态表。
func (s *GameService) applyRecruitRuntimeGuarantee(tx *gorm.DB, kind int32, cfg *db.GameplayPool) error {
	rule, err := s.recruitRuntimeRule(tx, kind)
	if err != nil {
		return err
	}
	if rule != nil {
		cfg.GuaranteeEvery = rule.GuaranteeEvery
	}
	return nil
}

// defaultRecruitRuntimeRule 为尚未建立运行表的兼容数据推导当前有效默认值。
func (s *GameService) defaultRecruitRuntimeRule(tx *gorm.DB, kind int32) (*db.RecruitRuntimeRule, error) {
	switch kind {
	case constUtil.RecruitRuntimeKindStrong:
		var cfg db.GameplayPool
		if err := tx.First(&cfg, "id=?", "recruit_2").Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		} else if err != nil {
			return nil, err
		}
		row := db.RecruitRuntimeRule{Kind: kind, DailyLimit: constUtil.RecruitRuntimeDailyUnlimited, GuaranteeEvery: cfg.GuaranteeEvery}
		if err := validateRecruitRuntimeRule(row); err != nil {
			return nil, err
		}
		return &row, nil
	case constUtil.RecruitRuntimeKindKing:
		var cfg db.GameplayPool
		if err := tx.First(&cfg, "id=?", "recruit_3").Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		} else if err != nil {
			return nil, err
		}
		var rules db.KingRecruitRules
		if err := tx.First(&rules, 1).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		} else if err != nil {
			return nil, err
		}
		row := db.RecruitRuntimeRule{Kind: kind, DailyLimit: rules.DailyLimit, GuaranteeEvery: cfg.GuaranteeEvery}
		if err := validateRecruitRuntimeRule(row); err != nil {
			return nil, err
		}
		return &row, nil
	}
	return nil, nil
}

// effectiveRecruitRuntimeRules 返回强者、王者当前全局规则；缺表时回退原静态值。
func (s *GameService) effectiveRecruitRuntimeRules(tx *gorm.DB) ([]db.RecruitRuntimeRule, error) {
	out := []db.RecruitRuntimeRule{}
	for _, kind := range []int32{constUtil.RecruitRuntimeKindStrong, constUtil.RecruitRuntimeKindKing} {
		rule, err := s.recruitRuntimeRule(tx, kind)
		if err != nil {
			return nil, err
		}
		if rule == nil {
			rule, err = s.defaultRecruitRuntimeRule(tx, kind)
			if err != nil {
				return nil, err
			}
		}
		if rule != nil {
			out = append(out, *rule)
		}
	}
	return out, nil
}

// recruitRuntimeItems 将当前生效规则投影为后台固定顺序的DTO。
func (s *GameService) recruitRuntimeItems(tx *gorm.DB) ([]model.AdminRecruitSetting, error) {
	rules, err := s.effectiveRecruitRuntimeRules(tx)
	if err != nil {
		return nil, err
	}
	if len(rules) != 2 {
		return nil, errors.New("招募运行规则不完整，请检查原奖池与P2规则")
	}
	items := make([]model.AdminRecruitSetting, 0, len(rules))
	for _, rule := range rules {
		items = append(items, model.AdminRecruitSetting{Kind: int64(rule.Kind), Name: constUtil.RecruitRuntimeName(rule.Kind), DailyLimit: int64(rule.DailyLimit), GuaranteeEvery: int64(rule.GuaranteeEvery)})
	}
	return items, nil
}
