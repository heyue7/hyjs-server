package service

import (
	"encoding/json"
	"errors"
	"math"

	"gorm.io/gorm"
	"hyjs-server/db"
)

// Legacy trial entries displayed a 100-star requirement without enforcing it.
// Explicit higher thresholds override the existing published requirement.
func pointShopRequiredStars(p p4Product) int32 {
	if p.MinTrialStars <= 0 {
		return 100
	}
	return p.MinTrialStars
}

// loadPointShopLifetime 在调用方持有角色行锁的事务中读取终身限购；首次读取合并全部历史期。
func loadPointShopLifetime(tx *gorm.DB, id int64) (*db.P4CommerceState, *p4CommerceProgress, int32, error) {
	row := &db.P4CommerceState{PlayerID: id, RuleID: "p4:point:lifetime-v1"}
	state := &p4CommerceProgress{Total: map[string]int32{}}
	err := tx.Where("player_id = ? AND rule_id = ?", id, row.RuleID).First(row).Error
	if err == nil {
		if json.Unmarshal(row.Payload, state) != nil || state.Total == nil {
			return nil, nil, 0, errors.New("试炼终身限购存档损坏")
		}
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		var history []db.P4CommerceState
		// 不筛选启用、窗口或冻结状态，关闭及归档目录同样计入。
		if err = tx.Table("p4_commerce_states AS states").Select("states.*").Joins("JOIN p4_commerce_rules AS rules ON rules.id = states.rule_id").Where("states.player_id = ? AND rules.kind = ?", id, "point").Find(&history).Error; err != nil {
			return nil, nil, 0, err
		}
		for _, prior := range history {
			var progress p4CommerceProgress
			if json.Unmarshal(prior.Payload, &progress) != nil {
				return nil, nil, 0, errors.New("历史试炼限购存档损坏")
			}
			periodTotal := map[string]int32{}
			for product, count := range progress.Total {
				if _, stable, ok := adminLiveShopParseSKU(product); ok {
					product = stable
				}
				if count < 0 || periodTotal[product] > math.MaxInt32-count {
					return nil, nil, 0, errors.New("历史试炼限购数量无效")
				}
				periodTotal[product] += count
			}
			// 原交易同时写日计数与Total；缺失累计证据时拒绝初始化，避免把旧购买清零。
			for product, count := range progress.Bought {
				if _, stable, ok := adminLiveShopParseSKU(product); ok {
					product = stable
				}
				if count < 0 || count > periodTotal[product] {
					return nil, nil, 0, errors.New("历史试炼购买缺少累计证据")
				}
			}
			for product, count := range periodTotal {
				if state.Total[product] > math.MaxInt32-count {
					return nil, nil, 0, errors.New("历史试炼限购数量无效")
				}
				state.Total[product] += count
			}
		}
		if err = saveP4Commerce(tx, row, state); err != nil {
			return nil, nil, 0, err
		}
	} else {
		return nil, nil, 0, err
	}
	for _, count := range state.Total {
		if count < 0 {
			return nil, nil, 0, errors.New("试炼终身限购数量无效")
		}
	}
	var rush db.P7RushState
	err = tx.Select("record_stars").Where("player_id = ?", id).First(&rush).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, 0, err
	}
	return row, state, rush.RecordStars, nil
}
