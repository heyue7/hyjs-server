package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// p5GrowthResourceProducts supplies raw inputs while retaining the actual
// refining, recipe, skill-book and experience consumers. These local prices
// do not assert a recovered original server economy.
func (s *GameService) p5GrowthResourceProducts() ([]p4Product, error) {
	var products []p4Product
	levels := []int32{18, 25, 35, 45, 60, 80}
	// TableText names are fixed display strings; raw skill rows have empty Name.
	mineNames := []string{"长石矿", "石英矿", "晶石矿", "水晶矿", "魔晶矿", "圣辉矿"}
	fishNames := []string{"爆裂鱼", "磨刀鱼", "剑齿鱼", "冰河鱼", "古代鱼", "大龙鱼", "小河蟹", "水箭虾", "岩石蟹", "爆炎虾", "钢岩螺", "帝王蟹", "贝壳", "扇贝", "水母", "乌贼", "章鱼", "河豚", "黄金鱼"}
	appendProduct := func(id, table, key, kind string, quantity, price int64, level, daily int32) error {
		row := s.table(table, key, id)
		if row == nil || row["Type"] != kind {
			return errors.New("单人材料原定义缺失")
		}
		n, err := strconv.Atoi(id[len(id)-3:])
		if err != nil {
			return err
		}
		name := ""
		if table == "MineSkill" && n >= 7 && n <= 12 {
			name = mineNames[n-7]
		}
		if table == "FishSkill" && n >= 7 && n <= 25 {
			name = fishNames[n-7]
		}
		if name == "" {
			return errors.New("单人材料原名称缺失")
		}
		product := p4BaseProduct(id, id+","+strconv.FormatInt(quantity, 10), "coin,"+strconv.FormatInt(price, 10), int32(len(products)+1), level)
		product.Name = name
		product.DailyLimit = daily
		products = append(products, product)
		return nil
	}
	for quality := 1; quality <= 6; quality++ {
		id := fmt.Sprintf("MineItem_%03d", quality+6)
		row := s.table("MineSkill", "MineItemID", id)
		if tableNumber(row, "Quality") != float64(quality) || tableNumber(row, "LevelLimit") != float64(quality) || row["ExtractMagicStone"] == "" || row["ExtractPowder"] == "" {
			return nil, errors.New("六阶原矿石提炼定义不完整")
		}
		if err := appendProduct(id, "MineSkill", "MineItemID", "stone", 5, int64(quality)*50000, levels[quality-1], 20); err != nil {
			return nil, err
		}
	}
	for quality := 1; quality <= 6; quality++ {
		// Anchor fish prices to the original same-grade recovery dish. The
		// multiplier, stock and character level remain explicitly local policy.
		dish := s.table("FishSkill", "FishItemID", fmt.Sprintf("FishItem_%03d", 26+(quality-1)*4))
		if dish["ValueType"] != "onetimeRecover" || tableNumber(dish, "Quality") != float64(quality) {
			return nil, errors.New("六阶料理价格锚点缺失")
		}
		anchor, err := s.p5GemCoinAnchor(dish["Material"])
		if err != nil {
			return nil, err
		}
		for group := 0; group < 3; group++ {
			id := fmt.Sprintf("FishItem_%03d", 6+quality+6*group)
			if err = appendProduct(id, "FishSkill", "FishItemID", "fish", 5, anchor*10, levels[quality-1], 20); err != nil {
				return nil, err
			}
		}
	}
	if err := appendProduct("FishItem_025", "FishSkill", "FishItemID", "fish", 1, 10000000, 80, 1); err != nil {
		return nil, err
	}
	return products, nil
}

// seedP5GrowthResourceSupply preserves operator edits and frozen history. A
// separate closed monthly child avoids changing any already purchased goods.
func (s *GameService) seedP5GrowthResourceSupply(tx *gorm.DB, now time.Time) error {
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	key := constUtil.P5GrowthResourceSupplyPrefix + start.Format("200601")
	var count int64
	if err := tx.Model(&db.P4CommerceRule{}).Where("id=?", key).Count(&count).Error; err != nil || count > 0 {
		return err
	}
	products, err := s.p5GrowthResourceProducts()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(p4CommerceRules{Products: products})
	if err != nil {
		return err
	}
	cfg := db.P4CommerceRule{ID: key, Kind: "exchange", Enabled: false, StartAt: start.UnixMilli(), EndAt: start.AddDate(0, 1, 0).UnixMilli(), ClaimEndAt: start.AddDate(0, 1, 0).UnixMilli(), Rules: raw, Source: constUtil.P5GrowthResourceSupplySource}
	// 续月只继承本来源最近的全局月份。193私期带后缀，不能成为运营发布事实。
	// 既有本月行已在上方返回，人工停运不会被重启或回拨恢复。
	var previous db.P4CommerceRule
	err = tx.Where("id LIKE ? AND LENGTH(id)=? AND start_at<?", constUtil.P5GrowthResourceSupplyPrefix+"%", len(constUtil.P5GrowthResourceSupplyPrefix)+6, start.UnixMilli()).Order("start_at DESC, id DESC").First(&previous).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil {
		if previous.Kind != "exchange" || previous.ShopType != 0 || !strings.HasPrefix(previous.Source, constUtil.P5GrowthResourceSupplyPolicy+"：") {
			return errors.New("单人材料前期来源无效")
		}
		// 新月份保持关闭，历史启用状态不自动续投。
		cfg.Enabled = false
		if previous.Enabled {
			prior, rules, e := s.p4CommerceSnapshot(previous)
			if e != nil {
				return e
			}
			if len(rules.Products) != 25 {
				return errors.New("单人材料前期目录不完整")
			}
			cfg.Rules = append([]byte(nil), prior.Rules...)
		}
	}
	if _, err = s.validateP4Commerce(cfg); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error
}

// p5AppendGrowthResourceBindings appends only after publication, even when
// this month's original directory has already been used. It never renumbers
// existing goods or freezes an unpublished operator catalog.
func (s *GameService) p5AppendGrowthResourceBindings(tx *gorm.DB, directory *db.P4ResourceExchangeDirectory, bindings *[]p4ExchangeBinding, now time.Time) error {
	// 个人193期的完整映射只在显式刷新工厂创建，后续供应开放不改旧期。
	if localPlayPrivateCycleID(directory.ID) != "" {
		return nil
	}
	key := constUtil.P5GrowthResourceSupplyPrefix + now.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
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
	cfg, rules, err := s.p4CommerceSnapshot(live)
	if err != nil {
		return err
	}
	if cfg.Kind != "exchange" || len(rules.Products) != 25 || len(*bindings)+25 > 1000 {
		return errors.New("单人材料供应目录无效")
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
	hash := sha256.Sum256(directory.Bindings)
	directory.InputHash = hex.EncodeToString(hash[:])
	return tx.Save(directory).Error
}
