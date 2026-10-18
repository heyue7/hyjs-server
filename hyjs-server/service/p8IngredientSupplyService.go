package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"time"
)

// seedP8IngredientSupply leaves the new source off until operators review and enable its frozen catalog.
func (s *GameService) seedP8IngredientSupply(tx *gorm.DB, now time.Time) error {
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	key := constUtil.P8IngredientSupplyPrefix + start.Format("200601")
	var count int64
	if e := tx.Model(&db.P4CommerceRule{}).Where("id=?", key).Count(&count).Error; e != nil {
		return e
	}
	if count > 0 {
		return nil
	}
	// Price anchors are the original same-grade healing dishes and the rare final recipe.
	for _, anchor := range []struct{ id, cost string }{{"FishItem_038", "40000"}, {"FishItem_042", "80000"}, {"FishItem_046", "160000"}, {"FishItem_050", "1000000"}} {
		costs, e := p3Resources(s.table("FishSkill", "FishItemID", anchor.id)["Material"])
		if e != nil {
			return e
		}
		found := false
		for _, cost := range costs {
			if cost.GetId() == "coin" {
				found = cost.GetNum() == int32(tableNumber(map[string]string{"coin": anchor.cost}, "coin"))
			}
		}
		if !found {
			return errors.New("料理食材供应价格锚点未恢复")
		}
	}
	raw := []byte(`{"products":[{"id":"FishItem_010","name":"冰河鱼","index":1,"rewards":"FishItem_010,5","cost":"coin,400000","priority":1001,"dailyLimit":1,"totalLimit":4,"stock":-1,"minLevel":65,"minVip":0,"discountBps":10000,"weight":1},{"id":"FishItem_016","name":"爆炎虾","index":2,"rewards":"FishItem_016,5","cost":"coin,400000","priority":1002,"dailyLimit":1,"totalLimit":4,"stock":-1,"minLevel":65,"minVip":0,"discountBps":10000,"weight":1},{"id":"FishItem_022","name":"乌贼","index":3,"rewards":"FishItem_022,5","cost":"coin,400000","priority":1003,"dailyLimit":1,"totalLimit":4,"stock":-1,"minLevel":65,"minVip":0,"discountBps":10000,"weight":1},{"id":"FishItem_011","name":"古代鱼","index":4,"rewards":"FishItem_011,5","cost":"coin,800000","priority":1004,"dailyLimit":1,"totalLimit":4,"stock":-1,"minLevel":75,"minVip":0,"discountBps":10000,"weight":1},{"id":"FishItem_017","name":"钢岩螺","index":5,"rewards":"FishItem_017,5","cost":"coin,800000","priority":1005,"dailyLimit":1,"totalLimit":4,"stock":-1,"minLevel":75,"minVip":0,"discountBps":10000,"weight":1},{"id":"FishItem_023","name":"章鱼","index":6,"rewards":"FishItem_023,5","cost":"coin,800000","priority":1006,"dailyLimit":1,"totalLimit":4,"stock":-1,"minLevel":75,"minVip":0,"discountBps":10000,"weight":1},{"id":"FishItem_012","name":"大龙鱼","index":7,"rewards":"FishItem_012,5","cost":"coin,1600000","priority":1007,"dailyLimit":1,"totalLimit":4,"stock":-1,"minLevel":85,"minVip":0,"discountBps":10000,"weight":1},{"id":"FishItem_018","name":"帝王蟹","index":8,"rewards":"FishItem_018,5","cost":"coin,1600000","priority":1008,"dailyLimit":1,"totalLimit":4,"stock":-1,"minLevel":85,"minVip":0,"discountBps":10000,"weight":1},{"id":"FishItem_024","name":"河豚","index":9,"rewards":"FishItem_024,5","cost":"coin,1600000","priority":1009,"dailyLimit":1,"totalLimit":4,"stock":-1,"minLevel":85,"minVip":0,"discountBps":10000,"weight":1},{"id":"FishItem_025","name":"黄金鱼","index":10,"rewards":"FishItem_025,1","cost":"coin,10000000","priority":1010,"dailyLimit":1,"totalLimit":1,"stock":-1,"minLevel":85,"minVip":0,"discountBps":10000,"weight":1}]}`)
	cfg := db.P4CommerceRule{ID: key, Kind: "exchange", Enabled: false, StartAt: start.UnixMilli(), EndAt: start.AddDate(0, 1, 0).UnixMilli(), ClaimEndAt: start.AddDate(0, 1, 0).UnixMilli(), Rules: raw, Source: constUtil.P8IngredientSupplySource}
	if _, e := s.validateP4Commerce(cfg); e != nil {
		return e
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error
}

// p8AppendIngredientBindings freezes only the enabled new child and appends its ten stable addresses.
// Default-off catalogs remain editable before first actual publication; existing indices never move.
func (s *GameService) p8AppendIngredientBindings(tx *gorm.DB, directory *db.P4ResourceExchangeDirectory, bindings *[]p4ExchangeBinding, now time.Time) error {
	key := constUtil.P8IngredientSupplyPrefix + now.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
	for _, binding := range *bindings {
		if binding.ChildID == key {
			return nil
		}
	}
	var live db.P4CommerceRule
	e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&live, "id=?", key).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil
	}
	if e != nil {
		return e
	}
	if !live.Enabled {
		return nil
	}
	cfg, rules, e := s.p4CommerceSnapshot(live)
	if e != nil {
		return e
	}
	if cfg.Kind != "exchange" || len(rules.Products) != 10 || len(*bindings)+10 > 1000 {
		return errors.New("高阶料理食材供应目录无效")
	}
	if len(live.Frozen) == 0 {
		raw, _ := json.Marshal(cfg)
		if e = tx.Model(&live).Update("frozen", raw).Error; e != nil {
			return e
		}
	}
	var index int32
	for _, b := range *bindings {
		index = max(index, b.Index)
	}
	for _, product := range rules.Products {
		index++
		*bindings = append(*bindings, p4ExchangeBinding{Index: index, ChildID: key, ChildIndex: product.Index, ProductID: product.ID})
	}
	directory.Bindings, _ = json.Marshal(*bindings)
	hash := sha256.Sum256(directory.Bindings)
	directory.InputHash = hex.EncodeToString(hash[:])
	return tx.Save(directory).Error
}
