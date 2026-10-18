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

// p5AwakeElementProducts retains the original five choices, with no direct
// material grants and no changes to the existing lucky bag/gold egg pools.
func (s *GameService) p5AwakeElementProducts() ([]p4Product, error) {
	box := s.table("Item", "itemId", "item_337")
	if box == nil || box["category"] != "5" || s.table("GameConfigure", "StaticName", "heroAwakeLevel")["NameValue"] != "70" {
		return nil, errors.New("原觉醒原石任选箱或开放等级无效")
	}
	expected := map[string]string{"1": "item_320,1", "2": "item_321,1", "3": "item_322,1", "4": "item_323,1", "5": "item_324,1"}
	seen := map[string]bool{}
	for _, option := range s.Tables["ItemUse"] {
		if option["itemId"] != "item_337" {
			continue
		}
		index := option["odds"]
		if expected[index] == "" || option["result"] != expected[index] || option["category"] != "5" || seen[index] {
			return nil, errors.New("原觉醒原石五选一配置无效")
		}
		seen[index] = true
	}
	if len(seen) != 5 {
		return nil, errors.New("原觉醒原石五选一配置缺失")
	}
	product := p4BaseProduct("item_337", "item_337,100", "coin,500000", 1, 70)
	product.Name = box["name"]
	product.DailyLimit, product.TotalLimit = 5, 150
	return []p4Product{product}, nil
}

func (s *GameService) p5AwakeElementSnapshot(live db.P4CommerceRule) (db.P4CommerceRule, p4CommerceRules, error) {
	cfg, rules, err := s.p4CommerceSnapshot(live)
	if err != nil {
		return cfg, rules, err
	}
	if cfg.Kind != "exchange" || cfg.ShopType != 0 || !strings.HasPrefix(cfg.Source, constUtil.P5AwakeElementSupplyPolicy+"：") || len(rules.Products) != 1 {
		return cfg, rules, errors.New("觉醒原石供应来源无效")
	}
	p := rules.Products[0]
	reward, rewardErr := parsePoolCost(p.Rewards)
	cost, costErr := parsePoolCost(p.Cost)
	if p.ID != "item_337" || p.MinLevel < 70 || rewardErr != nil || costErr != nil || reward.GetId() != "item_337" || cost.GetId() != "coin" {
		return cfg, rules, errors.New("觉醒原石供应商品无效")
	}
	return cfg, rules, nil
}

func (s *GameService) seedP5AwakeElementSupply(tx *gorm.DB, now time.Time) error {
	local := now.In(time.FixedZone("UTC+8", 28800))
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	key := constUtil.P5AwakeElementSupplyPrefix + start.Format("200601")
	var count int64
	if err := tx.Model(&db.P4CommerceRule{}).Where("id=?", key).Count(&count).Error; err != nil || count > 0 {
		return err
	}
	products, err := s.p5AwakeElementProducts()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(p4CommerceRules{Products: products})
	if err != nil {
		return err
	}
	cfg := db.P4CommerceRule{ID: key, Kind: "exchange", Enabled: false, StartAt: start.UnixMilli(), EndAt: start.AddDate(0, 1, 0).UnixMilli(), ClaimEndAt: start.AddDate(0, 1, 0).UnixMilli(), Rules: raw, Source: constUtil.P5AwakeElementSupplySource}
	// Global monthly publications alone establish continuity. A 193 private
	// clone has a longer ID and cannot enable a new global month.
	var previous db.P4CommerceRule
	err = tx.Where("id LIKE ? AND LENGTH(id)=? AND start_at<?", constUtil.P5AwakeElementSupplyPrefix+"%", len(constUtil.P5AwakeElementSupplyPrefix)+6, start.UnixMilli()).Order("start_at DESC, id DESC").First(&previous).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil {
		if previous.Kind != "exchange" || previous.ShopType != 0 || !strings.HasPrefix(previous.Source, constUtil.P5AwakeElementSupplyPolicy+"：") {
			return errors.New("觉醒原石前期来源无效")
		}
		// 新月份保持关闭，历史启用状态不自动续投。
		cfg.Enabled = false
		if previous.Enabled {
			prior, _, e := s.p5AwakeElementSnapshot(previous)
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
func (s *GameService) p5AppendAwakeElementBinding(tx *gorm.DB, directory *db.P4ResourceExchangeDirectory, bindings *[]p4ExchangeBinding, now time.Time) error {
	if localPlayPrivateCycleID(directory.ID) != "" {
		return nil
	}
	key := constUtil.P5AwakeElementSupplyPrefix + now.In(time.FixedZone("UTC+8", 28800)).Format("200601")
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
	cfg, rules, err := s.p5AwakeElementSnapshot(live)
	if err != nil {
		return err
	}
	if len(*bindings) >= 1000 {
		return errors.New("觉醒原石供应目录无效")
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
	*bindings = append(*bindings, p4ExchangeBinding{Index: index + 1, ChildID: key, ChildIndex: rules.Products[0].Index, ProductID: "item_337"})
	directory.Bindings, err = json.Marshal(*bindings)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(directory.Bindings)
	directory.InputHash = hex.EncodeToString(digest[:])
	return tx.Save(directory).Error
}
