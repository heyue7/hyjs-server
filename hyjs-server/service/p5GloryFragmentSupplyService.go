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

// p5GloryFragmentProducts supplies only the four original missing fragment IDs.
// Original sixty-fragment combines and all forging costs/odds stay unchanged.
func (s *GameService) p5GloryFragmentProducts() ([]p4Product, error) {
	if s.table("GameConfigure", "StaticName", "EquipForgeLevel")["NameValue"] != "60" {
		return nil, errors.New("原装备锻造开放等级无效")
	}
	var products []p4Product
	// Original TableText display labels. Text is not a runtime-loaded table.
	names := []string{"荣耀之刃", "荣耀战甲", "荣耀护手", "荣耀战靴"}
	for i, id := range []string{"equip_041", "equip_042", "equip_043", "equip_044"} {
		frag := s.table("Frag", "id", "frag_"+id)
		equip := s.table("Equip", "id", id)
		if frag["target"] != id || frag["num"] != "60" || frag["price"] != "500" || equip["quality"] != "23" {
			return nil, errors.New("原四件荣耀装备或碎片定义无效")
		}
		product := p4BaseProduct("frag_"+id, "frag_"+id+",3", "coin,100000", int32(i+1), 60)
		product.Name = names[i] + "碎片"
		product.DailyLimit, product.TotalLimit = 5, 150
		products = append(products, product)
	}
	return products, nil
}

func (s *GameService) p5GloryFragmentSnapshot(live db.P4CommerceRule) (db.P4CommerceRule, p4CommerceRules, error) {
	cfg, rules, err := s.p4CommerceSnapshot(live)
	if err != nil {
		return cfg, rules, err
	}
	if cfg.Kind != "exchange" || cfg.ShopType != 0 || !strings.HasPrefix(cfg.Source, constUtil.P5GloryFragmentSupplyPolicy+"：") || len(rules.Products) != 4 {
		return cfg, rules, errors.New("荣耀碎片供应来源无效")
	}
	allowed := map[string]bool{"frag_equip_041": true, "frag_equip_042": true, "frag_equip_043": true, "frag_equip_044": true}
	for _, p := range rules.Products {
		// 碎片是实际奖励；招募成本解析器只接货币/道具，不能拿来解析碎片。
		parts := strings.Split(p.Rewards, ",")
		if len(parts) != 2 || parts[0] != p.ID {
			return cfg, rules, errors.New("荣耀碎片供应商品无效")
		}
		_, rewardErr := p1PositiveAmount(parts[1])
		cost, costErr := parsePoolCost(p.Cost)
		if !allowed[p.ID] || p.MinLevel < 60 || rewardErr != nil || costErr != nil || cost.GetId() != "coin" {
			return cfg, rules, errors.New("荣耀碎片供应商品无效")
		}
		delete(allowed, p.ID)
	}
	return cfg, rules, nil
}

func (s *GameService) seedP5GloryFragmentSupply(tx *gorm.DB, now time.Time) error {
	local := now.In(time.FixedZone("UTC+8", 28800))
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	key := constUtil.P5GloryFragmentSupplyPrefix + start.Format("200601")
	var count int64
	if err := tx.Model(&db.P4CommerceRule{}).Where("id=?", key).Count(&count).Error; err != nil || count > 0 {
		return err
	}
	products, err := s.p5GloryFragmentProducts()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(p4CommerceRules{Products: products})
	if err != nil {
		return err
	}
	cfg := db.P4CommerceRule{ID: key, Kind: "exchange", Enabled: false, StartAt: start.UnixMilli(), EndAt: start.AddDate(0, 1, 0).UnixMilli(), ClaimEndAt: start.AddDate(0, 1, 0).UnixMilli(), Rules: raw, Source: constUtil.P5GloryFragmentSupplySource}
	// Global monthly publications alone establish continuity. A 193 private
	// clone has a longer ID and cannot enable a new global month.
	var previous db.P4CommerceRule
	err = tx.Where("id LIKE ? AND LENGTH(id)=? AND start_at<?", constUtil.P5GloryFragmentSupplyPrefix+"%", len(constUtil.P5GloryFragmentSupplyPrefix)+6, start.UnixMilli()).Order("start_at DESC, id DESC").First(&previous).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil {
		if previous.Kind != "exchange" || previous.ShopType != 0 || !strings.HasPrefix(previous.Source, constUtil.P5GloryFragmentSupplyPolicy+"：") {
			return errors.New("荣耀碎片前期来源无效")
		}
		// 新月份保持关闭，历史启用状态不自动续投。
		cfg.Enabled = false
		if previous.Enabled {
			prior, _, e := s.p5GloryFragmentSnapshot(previous)
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
func (s *GameService) p5AppendGloryFragmentBinding(tx *gorm.DB, directory *db.P4ResourceExchangeDirectory, bindings *[]p4ExchangeBinding, now time.Time) error {
	if localPlayPrivateCycleID(directory.ID) != "" {
		return nil
	}
	key := constUtil.P5GloryFragmentSupplyPrefix + now.In(time.FixedZone("UTC+8", 28800)).Format("200601")
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
	cfg, rules, err := s.p5GloryFragmentSnapshot(live)
	if err != nil {
		return err
	}
	if len(*bindings)+len(rules.Products) > 1000 {
		return errors.New("荣耀碎片供应目录无效")
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
