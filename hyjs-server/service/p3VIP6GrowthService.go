package service

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// seedP3VIP6GrowthProduct restores one missing original parcel through the
// existing paid membership store. Operator edits and existing daily quotes stay.
func (s *GameService) seedP3VIP6GrowthProduct(tx *gorm.DB) error {
	item := s.table("Item", "itemId", "item_040")
	if item["category"] != "2" || item["name"] != "vip6礼包" {
		return errors.New("原VIP6成长礼包定义无效")
	}
	var matches int
	for _, row := range s.Tables["ItemUse"] {
		if row["itemId"] != "item_040" {
			continue
		}
		matches++
		if row["category"] != "2" || row["odds"] != "100" || row["result"] != "trea_011,1;trea_013,1;item_005,1000;item_048,4;item_002,10" {
			return errors.New("原VIP6完整固定奖励无效")
		}
	}
	if matches != 1 {
		return errors.New("原VIP6礼包奖励不唯一")
	}
	product := db.VIPShopProduct{ID: constUtil.P3VIP6GrowthProduct, MinVIP: 6, Cost: 700, Reward: "item_040,1", Source: constUtil.P3VIP6GrowthSource, Enabled: false}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&product).Error
}
