package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// seedP4LuckySupply creates one independent default-off monthly source without replacing old products.
func (s *GameService) seedP4LuckySupply(tx *gorm.DB, now time.Time) error {
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	key := constUtil.P4LuckySupplyPrefix + start.Format("200601")
	var count int64
	if err := tx.Model(&db.P4CommerceRule{}).Where("id=?", key).Count(&count).Error; err != nil || count > 0 {
		return err
	}
	var (
		conversion int
		maxCoin    int64
	)
	for _, row := range s.Tables["ItemUse"] {
		if row["itemId"] == constUtil.P10FireworksItem {
			if row["category"] != "21" || row["result"] != "item_349,20" {
				return errors.New("原烟花20福袋兑换配置无效")
			}
			conversion++
		}
		if row["itemId"] == constUtil.P10LuckyBagItem {
			resources, err := p1RewardResources(row["result"])
			if err != nil {
				return err
			}
			for _, resource := range resources {
				if resource.GetId() == "coin" {
					maxCoin = max(maxCoin, int64(resource.GetNum()))
				}
			}
		}
	}
	if conversion != 1 || maxCoin != 1000000 || s.table("GameConfigure", "StaticName", "fireworksLevelLimit")["NameValue"] != "70" {
		return errors.New("原烟花供应参考条件不完整")
	}
	product := p4BaseProduct(constUtil.P10FireworksItem, "item_350,1", "coin,"+strconv.FormatInt(maxCoin*2, 10), 1, 70)
	product.Name = s.table("Item", "itemId", constUtil.P10FireworksItem)["name"]
	product.DailyLimit = 1
	product.TotalLimit = 3
	raw, _ := json.Marshal(p4CommerceRules{Products: []p4Product{product}})
	cfg := db.P4CommerceRule{ID: key, Kind: "exchange", Enabled: false, StartAt: start.UnixMilli(), EndAt: start.AddDate(0, 1, 0).UnixMilli(), ClaimEndAt: start.AddDate(0, 1, 0).UnixMilli(), Rules: raw, Source: constUtil.P4LuckySupplySource}
	if _, err := s.validateP4Commerce(cfg); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error
}

// p4LuckySupplyReady prevents selling a source whose original downstream activities remain closed.
// This only reads operator switches; it never enables P10 or changes its original reward pool.
func p4LuckySupplyReady(tx *gorm.DB) (bool, error) {
	for _, table := range []any{&db.P10LuckyPolicy{}, &db.P10ScenePolicy{}, &db.P10ChannelPolicy{}} {
		if !gameSchemaHasTable(tx, table) {
			return false, nil
		}
	}
	var (
		lucky   db.P10LuckyPolicy
		scene   db.P10ScenePolicy
		channel db.P10ChannelPolicy
	)
	for _, target := range []any{&lucky, &scene, &channel} {
		if err := tx.First(target, 1).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, nil
			}
			return false, err
		}
	}
	return lucky.FireworksEnabled && lucky.DropEnabled && scene.Enabled && channel.WorldEnabled, nil
}

// p4AppendLuckySupplyBinding adds a late publication at the tail of the frozen directory.
func (s *GameService) p4AppendLuckySupplyBinding(tx *gorm.DB, directory *db.P4ResourceExchangeDirectory, bindings *[]p4ExchangeBinding, now time.Time) error {
	key := constUtil.P4LuckySupplyPrefix + now.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
	for _, binding := range *bindings {
		if binding.ChildID == key {
			return nil
		}
	}
	var live db.P4CommerceRule
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&live, "id=?", key).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if !live.Enabled {
		return nil
	}
	ready, err := p4LuckySupplyReady(tx)
	if err != nil || !ready {
		return err
	}
	cfg, rules, err := s.p4CommerceSnapshot(live)
	if err != nil {
		return err
	}
	if cfg.Kind != "exchange" || len(rules.Products) != 1 || rules.Products[0].ID != constUtil.P10FireworksItem || len(*bindings) >= 1000 {
		return errors.New("烟花供应目录无效")
	}
	if now.UnixMilli() < cfg.StartAt || now.UnixMilli() >= cfg.EndAt {
		return nil
	}
	if len(live.Frozen) == 0 {
		raw, _ := json.Marshal(cfg)
		if err = tx.Model(&live).Update("frozen", raw).Error; err != nil {
			return err
		}
	}
	var index int32
	for _, binding := range *bindings {
		index = max(index, binding.Index)
	}
	*bindings = append(*bindings, p4ExchangeBinding{Index: index + 1, ChildID: key, ChildIndex: rules.Products[0].Index, ProductID: constUtil.P10FireworksItem})
	directory.Bindings, _ = json.Marshal(*bindings)
	hash := sha256.Sum256(directory.Bindings)
	directory.InputHash = hex.EncodeToString(hash[:])
	return tx.Save(directory).Error
}
