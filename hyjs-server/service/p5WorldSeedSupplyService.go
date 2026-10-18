package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// seedP5WorldSeedSupply preserves the finite original first drops and creates
// only a named local, default-off repeatable coin source for missing supply.
func (s *GameService) seedP5WorldSeedSupply(tx *gorm.DB, now time.Time) error {
	local := now.In(time.FixedZone("UTC+8", 28800))
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	key := constUtil.P5WorldSeedSupplyPrefix + start.Format("200601")
	var count int64
	if err := tx.Model(&db.P4CommerceRule{}).Where("id=?", key).Count(&count).Error; err != nil || count > 0 {
		return err
	}
	if s.table("Item", "itemId", "item_017") == nil {
		return errors.New("原世界之种定义缺失")
	}
	seen := map[int32]bool{}
	for _, row := range s.Tables["WorldSeed"] {
		id := p5SeedID(row)
		if id < 1 || id > 85 || seen[id] || row["cost"] != "item_017,1" {
			return errors.New("原世界之种节点成本无效")
		}
		seen[id] = true
	}
	if len(seen) != 85 {
		return errors.New("原世界之种85节点未恢复")
	}
	product := p4BaseProduct("item_017", "item_017,1", "coin,500000", 1, 25)
	product.Name = s.table("Item", "itemId", "item_017")["name"]
	product.DailyLimit = 3
	product.TotalLimit = 60
	raw, err := json.Marshal(p4CommerceRules{Products: []p4Product{product}})
	if err != nil {
		return err
	}
	cfg := db.P4CommerceRule{ID: key, Kind: "exchange", Enabled: false, StartAt: start.UnixMilli(), EndAt: start.AddDate(0, 1, 0).UnixMilli(), ClaimEndAt: start.AddDate(0, 1, 0).UnixMilli(), Rules: raw, Source: constUtil.P5WorldSeedSupplySource}
	if _, err = s.validateP4Commerce(cfg); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error
}

// p5AppendWorldSeedBinding freezes the first enabled publication and appends one
// address after every earlier product, including previously frozen directories.
func (s *GameService) p5AppendWorldSeedBinding(tx *gorm.DB, directory *db.P4ResourceExchangeDirectory, bindings *[]p4ExchangeBinding, now time.Time) error {
	key := constUtil.P5WorldSeedSupplyPrefix + now.In(time.FixedZone("UTC+8", 28800)).Format("200601")
	for _, b := range *bindings {
		if b.ChildID == key {
			return nil
		}
	}
	var live db.P4CommerceRule
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&live, "id=?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !live.Enabled {
		return nil
	}
	cfg, rules, err := s.p4CommerceSnapshot(live)
	if err != nil {
		return err
	}
	if cfg.Kind != "exchange" || len(rules.Products) != 1 || rules.Products[0].ID != "item_017" || len(*bindings) >= 1000 {
		return errors.New("世界之种供应目录无效")
	}
	if now.UnixMilli() < cfg.StartAt || now.UnixMilli() >= cfg.EndAt {
		return nil
	}
	if len(live.Frozen) == 0 {
		raw, e := json.Marshal(cfg)
		if e != nil {
			return e
		}
		if err = tx.Model(&live).Update("frozen", raw).Error; err != nil {
			return err
		}
	}
	var index int32
	for _, b := range *bindings {
		index = max(index, b.Index)
	}
	*bindings = append(*bindings, p4ExchangeBinding{Index: index + 1, ChildID: key, ChildIndex: rules.Products[0].Index, ProductID: rules.Products[0].ID})
	directory.Bindings, err = json.Marshal(*bindings)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(directory.Bindings)
	directory.InputHash = hex.EncodeToString(hash[:])
	return tx.Save(directory).Error
}
