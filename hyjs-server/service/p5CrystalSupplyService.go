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

// seedP5CrystalSupply leaves the new crystal source off until operators review and enable its frozen catalog.
func (s *GameService) seedP5CrystalSupply(tx *gorm.DB, now time.Time) error {
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	key := constUtil.P5CrystalSupplyPrefix + start.Format("200601")
	var count int64
	if e := tx.Model(&db.P4CommerceRule{}).Where("id=?", key).Count(&count).Error; e != nil {
		return e
	}
	if count > 0 {
		return nil
	}
	// Keep original item identities; the unavailable boss authority is not needed by this declared local trade.
	for _, id := range []string{"item_442", "item_446", "item_450", "item_454"} {
		if s.table("Item", "itemId", id) == nil {
			return errors.New("原灵魂水晶定义未恢复")
		}
	}
	raw := []byte(`{"products":[{"id":"item_442","name":"新手灵魂水晶","index":1,"rewards":"item_442,3","cost":"coin,500000","priority":1001,"dailyLimit":20,"totalLimit":100,"stock":-1,"minLevel":75,"minVip":0,"discountBps":10000,"weight":1},{"id":"item_446","name":"熟练灵魂水晶","index":2,"rewards":"item_446,3","cost":"coin,500000","priority":1002,"dailyLimit":20,"totalLimit":100,"stock":-1,"minLevel":85,"minVip":0,"discountBps":10000,"weight":1},{"id":"item_450","name":"专家灵魂水晶","index":3,"rewards":"item_450,3","cost":"coin,500000","priority":1003,"dailyLimit":20,"totalLimit":100,"stock":-1,"minLevel":90,"minVip":0,"discountBps":10000,"weight":1},{"id":"item_454","name":"大师灵魂水晶","index":4,"rewards":"item_454,3","cost":"coin,500000","priority":1004,"dailyLimit":20,"totalLimit":100,"stock":-1,"minLevel":95,"minVip":0,"discountBps":10000,"weight":1}]}`)
	cfg := db.P4CommerceRule{ID: key, Kind: "exchange", Enabled: false, StartAt: start.UnixMilli(), EndAt: start.AddDate(0, 1, 0).UnixMilli(), ClaimEndAt: start.AddDate(0, 1, 0).UnixMilli(), Rules: raw, Source: constUtil.P5CrystalSupplySource}
	if _, e := s.validateP4Commerce(cfg); e != nil {
		return e
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error
}

// p5AppendCrystalBindings freezes only the enabled new child and appends its four stable addresses.
// Default-off catalogs remain editable before first actual publication; existing indices never move.
func (s *GameService) p5AppendCrystalBindings(tx *gorm.DB, directory *db.P4ResourceExchangeDirectory, bindings *[]p4ExchangeBinding, now time.Time) error {
	key := constUtil.P5CrystalSupplyPrefix + now.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
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
	if cfg.Kind != "exchange" || len(rules.Products) != 4 || len(*bindings)+4 > 1000 {
		return errors.New("灵魂水晶供应目录无效")
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
