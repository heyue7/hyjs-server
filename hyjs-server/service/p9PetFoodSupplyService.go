package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// p9PetFoodProducts exposes only the original first-pet same-nature food.
// The original first recruit, category/nature and 34-level gate stay bound.
func (s *GameService) p9PetFoodProducts() ([]p4Product, error) {
	food := s.table("Item", "itemId", "item_129")
	first := s.table("GameConfigure", "StaticName", "PetFirstRecruitGet")
	gate := s.table("GameConfigure", "StaticName", "PetUnlockLevel")
	// P4 seeds before the later P9-only tables load. The source validates its
	// Item/GameConfigure contract here; the actual pet stays under P9 consumers.
	if food["category"] != "9" || food["nature"] != "1" || food["exp"] != "10" || first["NameValue"] != "pet_002,1" || gate["NameValue"] != "34" {
		return nil, errors.New("原首只精灵或火系食物定义无效")
	}
	product := p4BaseProduct("item_129", "item_129,6", "coin,60000", 1, 34)
	product.Name = food["name"]
	product.DailyLimit, product.TotalLimit = 5, 150
	return []p4Product{product}, nil
}

func (s *GameService) p9PetFoodSnapshot(live db.P4CommerceRule) (db.P4CommerceRule, p4CommerceRules, error) {
	cfg, rules, err := s.p4CommerceSnapshot(live)
	if err != nil {
		return cfg, rules, err
	}
	if cfg.Kind != "exchange" || cfg.ShopType != 0 || !strings.HasPrefix(cfg.Source, constUtil.P9PetFoodSupplyPolicy+"：") || len(rules.Products) != 1 {
		return cfg, rules, errors.New("火系精灵食物供应来源无效")
	}
	product := rules.Products[0]
	reward, rewardErr := parsePoolCost(product.Rewards)
	cost, costErr := parsePoolCost(product.Cost)
	if product.ID != "item_129" || product.MinLevel < 34 || rewardErr != nil || costErr != nil || reward.GetId() != product.ID || cost.GetId() != "coin" {
		return cfg, rules, errors.New("火系精灵食物供应商品无效")
	}
	return cfg, rules, nil
}

func (s *GameService) seedP9PetFoodSupply(tx *gorm.DB, now time.Time) error {
	local := now.In(time.FixedZone("UTC+8", 28800))
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	key := constUtil.P9PetFoodSupplyPrefix + start.Format("200601")
	var count int64
	if err := tx.Model(&db.P4CommerceRule{}).Where("id=?", key).Count(&count).Error; err != nil || count > 0 {
		return err
	}
	products, err := s.p9PetFoodProducts()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(p4CommerceRules{Products: products})
	if err != nil {
		return err
	}
	cfg := db.P4CommerceRule{ID: key, Kind: "exchange", Enabled: false, StartAt: start.UnixMilli(), EndAt: start.AddDate(0, 1, 0).UnixMilli(), ClaimEndAt: start.AddDate(0, 1, 0).UnixMilli(), Rules: raw, Source: constUtil.P9PetFoodSupplySource}
	// Global monthly publications alone establish continuity. A 193 private
	// clone has a longer ID and cannot enable a new global month.
	var previous db.P4CommerceRule
	err = tx.Where("id LIKE ? AND LENGTH(id)=? AND start_at<?", constUtil.P9PetFoodSupplyPrefix+"%", len(constUtil.P9PetFoodSupplyPrefix)+6, start.UnixMilli()).Order("start_at DESC, id DESC").First(&previous).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil {
		if previous.Kind != "exchange" || previous.ShopType != 0 || !strings.HasPrefix(previous.Source, constUtil.P9PetFoodSupplyPolicy+"：") {
			return errors.New("火系精灵食物前期来源无效")
		}
		// 新月份保持关闭，历史启用状态不自动续投。
		cfg.Enabled = false
		if previous.Enabled {
			prior, _, e := s.p9PetFoodSnapshot(previous)
			if e != nil {
				return e
			}
			cfg.Rules = append([]byte(nil), prior.Rules...)
		}
	}
	if _, err = s.validateP4Commerce(cfg); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error
}

// Existing global indices stay stable. A 193 directory is immutable; its new
// enabled child is included only by the existing explicit refresh factory.
func (s *GameService) p9AppendPetFoodBinding(tx *gorm.DB, directory *db.P4ResourceExchangeDirectory, bindings *[]p4ExchangeBinding, now time.Time) error {
	if localPlayPrivateCycleID(directory.ID) != "" {
		return nil
	}
	key := constUtil.P9PetFoodSupplyPrefix + now.In(time.FixedZone("UTC+8", 28800)).Format("200601")
	for _, binding := range *bindings {
		if binding.ChildID == key {
			return nil
		}
	}
	var live db.P4CommerceRule
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&live, "id=?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil || !live.Enabled {
		return err
	}
	cfg, rules, err := s.p9PetFoodSnapshot(live)
	if err != nil {
		return err
	}
	if len(*bindings)+len(rules.Products) > 1000 {
		return errors.New("火系精灵食物供应目录无效")
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
	for _, binding := range *bindings {
		index = max(index, binding.Index)
	}
	for _, product := range rules.Products {
		index++
		*bindings = append(*bindings, p4ExchangeBinding{Index: index, ChildID: key, ChildIndex: product.Index, ProductID: product.ID})
	}
	directory.Bindings, err = json.Marshal(*bindings)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(directory.Bindings)
	directory.InputHash = hex.EncodeToString(digest[:])
	return tx.Save(directory).Error
}
