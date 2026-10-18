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

// seedP15RedOSSupply 保留历史月份，新月份仅生成关闭模板，等待运营配置。
func (s *GameService) seedP15RedOSSupply(tx *gorm.DB, now time.Time) error {
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	key := constUtil.P15RedOSSupplyPrefix + start.Format("200601")
	var count int64
	if e := tx.Model(&db.P4CommerceRule{}).Where("id=?", key).Count(&count).Error; e != nil {
		return e
	}
	if count > 0 {
		return nil
	}
	// Original fragments keep their 150 threshold and soul resale field independently of local coin pricing.
	for _, id := range []string{"frag_hero_077", "frag_hero_078", "frag_hero_079"} {
		row := s.table("Frag", "id", id)
		if row == nil || row["num"] != "150" || row["price"] != "16" {
			return errors.New("三红原碎片数量与出售价未恢复")
		}
	}
	raw := []byte(`{"products":[{"id":"frag_hero_077","name":"OS桐人碎片","index":1,"rewards":"frag_hero_077,5","cost":"coin,500000","priority":1001,"dailyLimit":3,"totalLimit":30,"stock":-1,"minLevel":85,"minVip":0,"discountBps":10000,"weight":1},{"id":"frag_hero_078","name":"OS亚丝娜碎片","index":2,"rewards":"frag_hero_078,5","cost":"coin,500000","priority":1002,"dailyLimit":3,"totalLimit":30,"stock":-1,"minLevel":85,"minVip":0,"discountBps":10000,"weight":1},{"id":"frag_hero_079","name":"尤娜碎片","index":3,"rewards":"frag_hero_079,5","cost":"coin,500000","priority":1003,"dailyLimit":3,"totalLimit":30,"stock":-1,"minLevel":85,"minVip":0,"discountBps":10000,"weight":1}]}`)
	cfg := db.P4CommerceRule{ID: key, Kind: "exchange", Enabled: false, StartAt: start.UnixMilli(), EndAt: start.AddDate(0, 1, 0).UnixMilli(), ClaimEndAt: start.AddDate(0, 1, 0).UnixMilli(), Rules: raw, Source: constUtil.P15RedOSSupplySource}
	if _, e := s.validateP4Commerce(cfg); e != nil {
		return e
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error
}

// p15AppendRedOSBindings freezes only the enabled new child and appends its three stable addresses.
// Default-off catalogs remain editable before first actual publication; existing indices never move.
func (s *GameService) p15AppendRedOSBindings(tx *gorm.DB, directory *db.P4ResourceExchangeDirectory, bindings *[]p4ExchangeBinding, now time.Time) error {
	allowed, err := p15RedOSSupplyAllowed(tx)
	if err != nil || !allowed {
		return err
	}
	key := constUtil.P15RedOSSupplyPrefix + now.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
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
	if cfg.Kind != "exchange" || len(rules.Products) != 3 || len(*bindings)+3 > 1000 {
		return errors.New("三红碎片供应目录无效")
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
