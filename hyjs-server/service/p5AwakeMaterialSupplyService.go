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

// seedP5AwakeMaterialSupply retains the exact original deterministic choice box.
// Only its missing publication/coin price/limits use an explicit local policy.
func (s *GameService) seedP5AwakeMaterialSupply(tx *gorm.DB, now time.Time) error {
	local := now.In(time.FixedZone("UTC+8", 28800))
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	key := constUtil.P5AwakeMaterialSupplyPrefix + start.Format("200601")
	var count int64
	if err := tx.Model(&db.P4CommerceRule{}).Where("id=?", key).Count(&count).Error; err != nil || count > 0 {
		return err
	}
	box := s.table("Item", "itemId", "item_330")
	if box == nil || box["category"] != "5" || s.table("GameConfigure", "StaticName", "heroAwakeLevel")["NameValue"] != "70" {
		return errors.New("原觉醒选择盒或开放等级无效")
	}
	expected := map[string]string{"1": "item_312,2", "2": "item_315,8", "3": "item_313,8"}
	seen := map[string]bool{}
	for _, option := range s.Tables["ItemUse"] {
		if option["itemId"] == "item_330" {
			index := option["odds"]
			if expected[index] == "" || expected[index] != option["result"] || seen[index] || option["category"] != "5" {
				return errors.New("原觉醒选择盒三选一规则无效")
			}
			seen[index] = true
		}
	}
	if len(seen) != 3 {
		return errors.New("原觉醒选择盒选项未恢复")
	}
	product := p4BaseProduct("item_330", "item_330,1", "coin,1000000", 1, 70)
	product.Name = box["name"]
	product.DailyLimit = 1
	product.TotalLimit = 30
	raw, err := json.Marshal(p4CommerceRules{Products: []p4Product{product}})
	if err != nil {
		return err
	}
	cfg := db.P4CommerceRule{ID: key, Kind: "exchange", Enabled: false, StartAt: start.UnixMilli(), EndAt: start.AddDate(0, 1, 0).UnixMilli(), ClaimEndAt: start.AddDate(0, 1, 0).UnixMilli(), Rules: raw, Source: constUtil.P5AwakeMaterialSupplySource}
	// 只让缺失的新月份继承本供应最近全局月份；个人私期不成为发布依据。
	// 已有本月行在上方原样返回，停运不会因重启或日期回拨恢复。
	var previous db.P4CommerceRule
	err = tx.Where("id LIKE ? AND LENGTH(id)=? AND start_at<?", constUtil.P5AwakeMaterialSupplyPrefix+"%", len(constUtil.P5AwakeMaterialSupplyPrefix)+6, start.UnixMilli()).Order("start_at DESC, id DESC").First(&previous).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil {
		if previous.Kind != "exchange" || previous.ShopType != 0 || previous.Source != constUtil.P5AwakeMaterialSupplySource {
			return errors.New(constUtil.P5AwakeMaterialContinuityInvalid)
		}
		// 新月份保持关闭，历史启用状态不自动续投。
		cfg.Enabled = false
		if previous.Enabled {
			prior, rules, e := s.p4CommerceSnapshot(previous)
			if e != nil {
				return e
			}
			if len(rules.Products) != 1 || rules.Products[0].ID != "item_330" || rules.Products[0].MinLevel < 70 {
				return errors.New(constUtil.P5AwakeMaterialContinuityInvalid)
			}
			cfg.Rules = append([]byte(nil), prior.Rules...)
		}
	}
	if _, err = s.validateP4Commerce(cfg); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error
}

// p5AppendAwakeMaterialBinding freezes the first enabled publication and appends one
// address after every earlier product, including previously frozen directories.
func (s *GameService) p5AppendAwakeMaterialBinding(tx *gorm.DB, directory *db.P4ResourceExchangeDirectory, bindings *[]p4ExchangeBinding, now time.Time) error {
	// 个人目录只在显式新期工厂创建；后开放330不追改已用期的商品地址。
	if localPlayPrivateCycleID(directory.ID) != "" {
		return nil
	}
	key := constUtil.P5AwakeMaterialSupplyPrefix + now.In(time.FixedZone("UTC+8", 28800)).Format("200601")
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
	if cfg.Kind != "exchange" || len(rules.Products) != 1 || rules.Products[0].ID != "item_330" || len(*bindings) >= 1000 {
		return errors.New("觉醒材料供应目录无效")
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
