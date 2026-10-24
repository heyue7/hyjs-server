package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

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
	if row.LuckExchangeCost < 0 || row.Kind == constUtil.RecruitRuntimeKindStrong && row.LuckExchangeCost != 0 {
		return errors.New("幸运兑换所需值超出范围")
	}
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
	if !gameSchemaHasTable(tx, &db.RecruitRuntimeRule{}) {
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
	// 新列为0的历史运行行沿用500兑换门槛，余额从此不受该门槛裁剪。
	if kind == constUtil.RecruitRuntimeKindKing && row.LuckExchangeCost == 0 {
		row.LuckExchangeCost = constUtil.RecruitRuntimeLuckCostDefault
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
		row := db.RecruitRuntimeRule{Kind: kind, DailyLimit: rules.DailyLimit, GuaranteeEvery: cfg.GuaranteeEvery, LuckExchangeCost: rules.LuckLimit}
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
		items = append(items, recruitRuntimeSetting(rule))
	}
	return items, nil
}

// recruitRuntimeSetting 保持历史零列与实时读取的版本口径一致。
func recruitRuntimeSetting(rule db.RecruitRuntimeRule) model.AdminRecruitSetting {
	cost := rule.LuckExchangeCost
	if rule.Kind == constUtil.RecruitRuntimeKindKing && cost == 0 {
		cost = constUtil.RecruitRuntimeLuckCostDefault
	}
	return model.AdminRecruitSetting{Kind: int64(rule.Kind), Name: constUtil.RecruitRuntimeName(rule.Kind), DailyLimit: int64(rule.DailyLimit), GuaranteeEvery: int64(rule.GuaranteeEvery), LuckExchangeCost: int64(cost)}
}

// balancedRecruit35Rewards 按品质总概率分配整数权重，同品质各角色等概率，三世界橙卡概率与常驻橙池一致。
func (s *GameService) balancedRecruit35Rewards(poolID string, world int64, king bool) ([]db.GameplayPoolReward, error) {
	if world < 0 || world > 3 {
		return nil, constUtil.ErrAdminConfiguration
	}
	groups := map[string][]string{}
	ppm := map[string]int64{"12": 600000, "13": 343000, "18": 51000, "20": 6000}
	order := []string{"12", "13", "18", "20"}
	if king {
		ppm["13"] -= constUtil.RecruitBalance35RedPPM
		for _, space := range []int64{1, 2, 3} {
			if world == 0 || world == space {
				group := "23:" + strconv.FormatInt(space, 10)
				ppm[group] = constUtil.RecruitBalance35RedPPM
				order = append(order, group)
			}
		}
	}
	for _, hero := range s.Tables["Hero"] {
		if hero["heroClass"] != "0" {
			continue
		}
		group := hero["quality"]
		if group == "23" {
			group += ":" + hero["space"]
		}
		if _, ok := ppm[group]; ok {
			groups[group] = append(groups[group], hero["id"])
		}
	}
	// 每个红卡世界独立分配同一总权重；world=0是预筛选集合，过滤后每个世界的总量均为百万分母。
	scale := int64(1)
	for _, group := range order {
		count := int64(len(groups[group]))
		if count == 0 {
			return nil, constUtil.ErrAdminConfiguration
		}
		a, b := scale, count
		for b != 0 {
			a, b = b, a%b
		}
		if scale/a > math.MaxInt64/count {
			return nil, constUtil.ErrAdminConfiguration
		}
		scale = scale / a * count
	}
	out := []db.GameplayPoolReward{}
	for _, group := range order {
		sort.Strings(groups[group])
		factor := scale / int64(len(groups[group]))
		if ppm[group] < 1 || factor > 1000000000000/ppm[group] {
			return nil, constUtil.ErrAdminConfiguration
		}
		for _, id := range groups[group] {
			out = append(out, db.GameplayPoolReward{PoolID: poolID, ID: int32(len(out) + 1), ResourceID: id, Amount: 1, Weight: ppm[group] * factor, Guaranteed: group == "20" || strings.HasPrefix(group, "23:")})
		}
	}
	return out, nil
}

// InitializeRecruitBalance35 一次性应用获授权的全品质奖池和软保底初值；重启保留后台后续改动。
// 每个旧配置在同事务审计中完整留存，不改玩家资源、抽数、免费冷却、领取和订单回执。
func (s *GameService) InitializeRecruitBalance35(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		// 先锁真实稳定基础池，再检查全局版本标记；同库各区服与并发启动共享这一互斥入口。
		var stable db.GameplayPool
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&stable, "id=?", "recruit_2").Error; err != nil {
			return err
		}
		target := constUtil.RecruitBalance35Version
		var marker db.AdminAudit
		// 锁定读使用当前已提交版本，避免MySQL旧读视图在等待基础池锁后再次迁移。
		markerErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("action=? AND target=?", constUtil.RecruitBalance35ApplyAction, target).Take(&marker).Error
		if markerErr == nil {
			return nil
		}
		if !errors.Is(markerErr, gorm.ErrRecordNotFound) {
			return markerErr
		}
		backup := func(kind, key string, value any) error {
			raw := adminJSON(value)
			if len(raw) > 60000 {
				return fmt.Errorf("招募旧配置过大，保留原配置并中止升级: %s", key)
			}
			return tx.Create(&db.AdminAudit{Time: time.Now().UnixMilli(), Actor: "system", Action: constUtil.RecruitBalance35BeforeAction, Target: kind + ":" + key, Detail: string(raw)}).Error
		}
		// 按各区服已持久化的有效游戏日期选择当前和未来排期，保留虚拟日期中已经结束的历史。
		realNow := time.Now()
		offsets := map[int64]int64{}
		if gameSchemaHasTable(tx, &db.ServerGameClock{}) {
			var clocks []db.ServerGameClock
			if err := tx.Find(&clocks).Error; err != nil {
				return err
			}
			for _, clock := range clocks {
				if !serverGameClockValid(&clock) {
					return constUtil.ErrAdminConfiguration
				}
				offsets[clock.ServerID] = clock.OffsetDays
			}
		}
		var candidates []db.AdminActivityOccurrence
		if err := tx.Where("mode=? AND archived=? AND enabled=?", constUtil.AdminKingRecruitMode, false, true).Order("id").Find(&candidates).Error; err != nil {
			return err
		}
		occurrenceIDs := []string{}
		poolIDs := []string{"recruit_2", "recruit_3"}
		for _, row := range candidates {
			if row.EndAt <= realNow.AddDate(0, 0, int(offsets[row.ServerID])).UnixMilli() {
				continue
			}
			if !adminKingRecruitPoolID(row.ConsumerKey) {
				return constUtil.ErrAdminConfiguration
			}
			occurrenceIDs = append(occurrenceIDs, row.ID)
			poolIDs = append(poolIDs, row.ConsumerKey)
		}
		// 基础池入口锁之后按ID锁全体目标奖池，模板、排期和运行规则放在其后。
		var pools []db.GameplayPool
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", poolIDs).Order("id").Find(&pools).Error; err != nil {
			return err
		}
		foundPools := make(map[string]bool, len(pools))
		for _, pool := range pools {
			foundPools[pool.ID] = true
		}
		for _, id := range poolIDs {
			if !foundPools[id] {
				return constUtil.ErrAdminConfiguration
			}
		}
		for _, pool := range pools {
			var before []db.GameplayPoolReward
			if err := tx.Where("pool_id=?", pool.ID).Order("id").Find(&before).Error; err != nil {
				return err
			}
			world := int64(0)
			king := pool.ID != "recruit_2"
			var snapshot db.KingRecruitActivitySnapshot
			var source adminKingRecruitSource
			hasSnapshot := false
			if king {
				err := tx.First(&snapshot, "pool_id=?", pool.ID).Error
				if err == nil {
					if json.Unmarshal(snapshot.Payload, &source) != nil {
						return constUtil.ErrAdminConfiguration
					}
					world = source.World
					hasSnapshot = true
				} else if !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
			}
			if err := backup("pool", pool.ID, map[string]any{"pool": pool, "rewards": before, "snapshot": map[string]any{"poolId": snapshot.PoolID, "frozen": snapshot.Frozen, "payload": json.RawMessage(snapshot.Payload)}}); err != nil {
				return err
			}
			rewards, err := s.balancedRecruit35Rewards(pool.ID, world, king)
			if err != nil {
				return err
			}
			if err = tx.Where("pool_id=?", pool.ID).Delete(&db.GameplayPoolReward{}).Error; err != nil {
				return err
			}
			if err = tx.Create(&rewards).Error; err != nil {
				return err
			}
			if hasSnapshot {
				// 原已冻结的价格、世界与时窗保持，仅更新本次明确调整的品质候选和权重来源。
				source.Pool.Name = "王者招募"
				source.Pool.Source = constUtil.RecruitBalance35Source
				snapshot.Payload = adminJSON(source)
				if err = tx.Save(&snapshot).Error; err != nil {
					return err
				}
			}
			if king {
				if err = tx.Model(&db.GameplayPool{}).Where("id=?", pool.ID).Updates(map[string]any{"name": "王者招募", "source": constUtil.RecruitBalance35Source}).Error; err != nil {
					return err
				}
			}
		}
		// 同步现存模板与排期的实际角色清单；旧FormJSON分别存审计，历史领奖身份和实例ID保持。
		var templates []db.AdminActivityDefinition
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("mode=? AND archived=?", constUtil.AdminKingRecruitMode, false).Order("id").Find(&templates).Error; err != nil {
			return err
		}
		for _, row := range templates {
			var form model.AdminActivityForm
			if json.Unmarshal(row.FormJSON, &form) != nil {
				return constUtil.ErrAdminConfiguration
			}
			if err := backup("template", row.ID, json.RawMessage(row.FormJSON)); err != nil {
				return err
			}
			rewards, err := s.balancedRecruit35Rewards("", form.Options.KingWorld, true)
			if err != nil {
				return err
			}
			form.Prizes = []model.AdminActivityPrize{}
			for _, r := range rewards {
				form.Prizes = append(form.Prizes, model.AdminActivityPrize{ID: int64(r.ID), ResourceID: r.ResourceID, Amount: 1, Weight: r.Weight, Guaranteed: r.Guaranteed})
			}
			form.Name = "王者招募"
			form.Source = constUtil.RecruitBalance35Source
			row.Name = "王者招募"
			row.FormJSON = adminJSON(form)
			row.Revision++
			row.UpdatedAtMillis = time.Now().UnixMilli()
			if err = tx.Save(&row).Error; err != nil {
				return err
			}
		}
		var occurrences []db.AdminActivityOccurrence
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", occurrenceIDs).Order("id").Find(&occurrences).Error; err != nil {
			return err
		}
		for _, row := range occurrences {
			var form model.AdminActivityForm
			if json.Unmarshal(row.FormJSON, &form) != nil {
				return constUtil.ErrAdminConfiguration
			}
			if err := backup("occurrence", row.ID, json.RawMessage(row.FormJSON)); err != nil {
				return err
			}
			rewards, err := s.balancedRecruit35Rewards("", form.Options.KingWorld, true)
			if err != nil {
				return err
			}
			form.Prizes = []model.AdminActivityPrize{}
			for _, r := range rewards {
				form.Prizes = append(form.Prizes, model.AdminActivityPrize{ID: int64(r.ID), ResourceID: r.ResourceID, Amount: 1, Weight: r.Weight, Guaranteed: r.Guaranteed})
			}
			form.Name = "王者招募"
			form.Source = constUtil.RecruitBalance35Source
			row.Name = "王者招募"
			row.FormJSON = adminJSON(form)
			row.Revision++
			if err = tx.Save(&row).Error; err != nil {
				return err
			}
		}
		if err := s.seedRecruitRuntimeRules(tx); err != nil {
			return err
		}
		var oldRules []db.RecruitRuntimeRule
		if err := tx.Order("kind").Find(&oldRules).Error; err != nil {
			return err
		}
		if err := backup("runtime", target, oldRules); err != nil {
			return err
		}
		if err := tx.Model(&db.RecruitRuntimeRule{}).Where("kind IN ?", []int32{2, 3}).Updates(map[string]any{"daily_limit": constUtil.RecruitBalance35DailyLimit, "guarantee_every": constUtil.RecruitBalance35GuaranteeEvery}).Error; err != nil {
			return err
		}
		return tx.Create(&db.AdminAudit{Time: time.Now().UnixMilli(), Actor: "system", Action: constUtil.RecruitBalance35ApplyAction, Target: target, Detail: "全品质招募、跨世界橙卡与递增软保底；原价格、抽数和领取历史保留"}).Error
	})
}
