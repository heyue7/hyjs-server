package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/db"
	"sort"
	"time"
)

// seedP4Rotations 保留默认关闭的轮换模板；奖励池由运营确认后启用，常驻基础招募保持。
func (s *GameService) seedP4Rotations(tx *gorm.DB, now time.Time) error {
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	monday := day.AddDate(0, 0, -(int(day.Weekday())+6)%7)
	for kind := int32(1); kind <= 2; kind++ {
		pool := fmt.Sprintf("recruit_%d", kind)
		var base []db.GameplayPoolReward
		if err := tx.Where("pool_id=?", pool).Order("id").Find(&base).Error; err != nil {
			return err
		}
		if len(base) == 0 {
			return errors.New("轮换需先初始化普通/强者原奖池")
		}
		groups := map[string][]db.GameplayPoolReward{}
		for _, p := range base {
			row := s.table("Hero", "id", p.ResourceID)
			if row == nil {
				return errors.New("轮换英雄未恢复")
			}
			groups[row["quality"]] = append(groups[row["quality"]], p)
		}
		for half := 0; half < 2; half++ {
			start := monday
			end := monday.AddDate(0, 0, 3)
			if half == 1 {
				start = end
				end = monday.AddDate(0, 0, 7)
			}
			weights := map[string]int64{"12": 65, "13": 30, "18": 5}
			if kind == 2 {
				weights = map[string]int64{"13": 15, "18": 75, "20": 10}
				if half == 0 {
					weights = map[string]int64{"13": 10, "18": 65, "20": 25}
				}
			}
			selected := map[string][]db.GameplayPoolReward{}
			scale := int64(1)
			for q, items := range groups {
				if weights[q] == 0 {
					continue
				}
				for i, p := range items {
					if i%2 == half || len(items) == 1 {
						selected[q] = append(selected[q], p)
					}
				}
				if len(selected[q]) == 0 {
					selected[q] = append(selected[q], items[0])
				}
				scale *= int64(len(selected[q]))
			}
			var rewards []db.GameplayPoolReward
			for q, items := range selected {
				for _, p := range items {
					p.Weight = weights[q] * scale / int64(len(items))
					p.Stock = 0
					rewards = append(rewards, p)
				}
			}
			sort.Slice(rewards, func(i, j int) bool { return rewards[i].ID < rewards[j].ID })
			raw, err := json.Marshal(rewards)
			if err != nil {
				return err
			}
			cfg := db.P4RecruitRotation{ID: fmt.Sprintf("p4:recruit:%d:%s:%d", kind, monday.Format("20060102"), half), Kind: kind, Enabled: false, StartAt: start.UnixMilli(), EndAt: end.UnixMilli(), Rewards: raw, Source: "本地兼容服完整普通/强者轮换：同资质原英雄按ID单双序轮换；各组等概率；普通65/30/5，强者周一窗口10/65/25、周四15/75/10。强者橙卡提升为真实25%组权重；价格、免费与保底沿APK原值。"}
			var exists int64
			if err = tx.Model(&db.P4RecruitRotation{}).Where("id=?", cfg.ID).Count(&exists).Error; err != nil {
				return err
			}
			if exists > 0 {
				continue
			}
			if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *GameService) loadP4Rotation(tx *gorm.DB, kind int32, now time.Time) ([]db.GameplayPoolReward, int64, error) {
	if err := s.seedP4Rotations(tx, now); err != nil {
		return nil, 0, err
	}
	var cfg db.P4RecruitRotation
	var candidates []db.P4RecruitRotation
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("kind=? AND enabled=? AND start_at<=? AND end_at>?", kind, true, now.UnixMilli(), now.UnixMilli()).Order("start_at DESC,id DESC").Find(&candidates).Error; err != nil {
		return nil, 0, err
	}
	for _, candidate := range candidates {
		allowed, err := localPlayActivityVisible(tx, candidate.ID)
		if err != nil {
			return nil, 0, err
		}
		if allowed {
			cfg = candidate
			break
		}
	}
	if cfg.ID == "" {
		return nil, 0, gorm.ErrRecordNotFound
	}
	raw := cfg.Frozen
	if len(raw) == 0 {
		raw = cfg.Rewards
	}
	var rewards []db.GameplayPoolReward
	if json.Unmarshal(raw, &rewards) != nil || len(rewards) == 0 {
		return nil, 0, errors.New("轮换冻结奖池无效")
	}
	seen := map[string]bool{}
	guaranteed := false
	for _, p := range rewards {
		hero := s.table("Hero", "id", p.ResourceID)
		quality := hero["quality"]
		allowed := kind == 1 && (quality == "12" || quality == "13" || quality == "18") || kind == 2 && (quality == "13" || quality == "18" || quality == "20")
		isGuarantee := kind == 1 && quality == "18" || kind == 2 && quality == "20"
		if p.PoolID != fmt.Sprintf("recruit_%d", kind) || p.Amount != 1 || p.Weight < 1 || p.Weight > 1000000000000 || seen[p.ResourceID] || hero["heroClass"] != "0" || !allowed || p.Guaranteed != isGuarantee {
			return nil, 0, errors.New("轮换概率或英雄配置无效")
		}
		seen[p.ResourceID] = true
		guaranteed = guaranteed || p.Guaranteed
	}
	if !guaranteed {
		return nil, 0, errors.New("轮换缺少原保底组")
	}
	if len(cfg.Frozen) == 0 {
		if err := tx.Model(&cfg).Update("frozen", raw).Error; err != nil {
			return nil, 0, err
		}
	}
	return rewards, cfg.EndAt, nil
}
