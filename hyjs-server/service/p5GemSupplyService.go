package service

import (
	"encoding/json"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"strconv"
	"strings"
	"time"
)

// seedP5GemSupply connects missing original box sources to a declared coin economy.
// Operator edits and frozen purchase periods retain their existing transaction history.
func (s *GameService) seedP5GemSupply(tx *gorm.DB, now time.Time) error {
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	end := start.AddDate(0, 1, 0)
	id := constUtil.P5GemSupplyPrefix + start.Format("200601")
	var count int64
	if err := tx.Model(&db.P4CommerceRule{}).Where("id=?", id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	rules := p4CommerceRules{}
	for index, spec := range []struct {
		id, gem string
		level   int32
	}{
		{"item_137", "MineItem_023", 55}, {"item_342", "MineItem_025", 70}, {"item_343", "MineItem_026", 75}, {"item_344", "MineItem_027", 80}, {"item_345", "MineItem_028", 85},
	} {
		// Cost follows a documented ten-times original manufacturing coin anchor.
		material := s.table("MineSkill", "MineItemID", spec.gem)["Material"]
		base, err := s.p5GemCoinAnchor(material)
		if err != nil {
			return err
		}
		product := p4BaseProduct(spec.id, spec.id+",1", "coin,"+strconv.FormatInt(base*10, 10), int32(index+1), spec.level)
		product.Name = s.table("Item", "itemId", spec.id)["name"]
		product.DailyLimit, product.TotalLimit = 1, 3
		rules.Products = append(rules.Products, product)
	}
	raw, err := json.Marshal(rules)
	if err != nil {
		return err
	}
	cfg := db.P4CommerceRule{ID: id, ShopType: 0, Kind: "exchange", Enabled: false, StartAt: start.UnixMilli(), EndAt: end.UnixMilli(), ClaimEndAt: end.UnixMilli(), Rules: raw, Source: constUtil.P5GemSupplySource}
	if _, err := s.validateP4Commerce(cfg); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error
}

func (s *GameService) p5GemCoinAnchor(material string) (int64, error) {
	var amount int64
	for _, spec := range strings.Split(material, ";") {
		pair := strings.Split(spec, ",")
		if len(pair) != 2 {
			return 0, p5Error()
		}
		if pair[0] != "coin" {
			continue
		}
		if amount != 0 {
			return 0, p5Error()
		}
		n, err := strconv.ParseInt(pair[1], 10, 64)
		if err != nil || n < 1 || n > 100000000 {
			return 0, p5Error()
		}
		amount = n
	}
	if amount == 0 {
		return 0, p5Error()
	}
	return amount, nil
}
