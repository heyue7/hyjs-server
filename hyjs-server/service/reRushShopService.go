package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"reflect"
	"strings"
	"time"
)

func reRushShopRules() p4CommerceRules {
	rules := p4CommerceRules{LiveEditable: true, Products: []p4Product{}}
	for i, id := range []string{"item_005", "item_008", "item_012", "item_007", "item_006", "item_013", "item_009", "item_010"} {
		p := p4BaseProduct(fmt.Sprintf("offline_rush_shop_%d", i), id+",10", fmt.Sprintf("point,%d", 500+i*100), int32(i+1), 22)
		p.Priority = int32(1000 + (i%4)*1000 + i)
		p.DailyLimit = 1000
		rules.Products = append(rules.Products, p)
	}
	return rules
}

// Replace only the exact old generated placeholder. Source identity, frozen
// bytes, player progress, stock and receipts stay in place. Operator edits win.
func (s *GameService) seedReRushShop(tx *gorm.DB, now time.Time) error {
	var source db.P4CommerceRule
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("kind=? AND id NOT LIKE ?", "point", "shop_schedule_%").Order("start_at DESC,id DESC").First(&source).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil && strings.Contains(source.Source, constUtil.ReRushShopPolicy) {
		return nil
	}
	if tx.Migrator().HasTable(&db.AdminAudit{}) {
		var edits int64
		if e := tx.Model(&db.AdminAudit{}).Where("target=? AND action LIKE ?", "point", "shop.%").Count(&edits).Error; e != nil {
			return e
		}
		if edits > 0 {
			return nil
		}
	}
	if err == nil {
		legacy := p4CommerceRules{Products: []p4Product{p4BaseProduct("p4Point_1", "item_021,1", "point,100", 1, 22), p4BaseProduct("p4Point_2", "equip_037,1", "item_021,10", 2, 22)}}
		legacy.Products[1].Priority = 2001
		legacy.Products[1].DailyLimit = 1
		var old p4CommerceRules
		if !strings.HasPrefix(source.ID, "p4:point:") || !strings.HasPrefix(source.Source, "本地兼容服目录/限购/库存：") || json.Unmarshal(source.Rules, &old) != nil || !reflect.DeepEqual(old, legacy) {
			return nil
		}
	} else {
		local := now.In(time.FixedZone("UTC+8", 8*3600))
		start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
		source = db.P4CommerceRule{ID: constUtil.ReRushShopID, Kind: "point", ShopType: 5, StartAt: start.UnixMilli(), EndAt: start.AddDate(0, 1, 0).UnixMilli(), ClaimEndAt: start.AddDate(0, 1, 0).UnixMilli()}
	}
	source.Enabled = true
	source.Rules = adminJSON(reRushShopRules())
	source.Source = constUtil.ReRushShopPolicy + "；" + constUtil.ReRushShopSource
	if _, err := s.validateP4Commerce(source); err != nil {
		return err
	}
	return tx.Save(&source).Error
}
