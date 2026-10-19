package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type p4Product struct {
	ID          string `json:"id"`
	MaxRank     int64  `json:"maxRank,omitempty"`
	Name        string `json:"name,omitempty"`
	Index       int32  `json:"index"`
	Rewards     string `json:"rewards"`
	Cost        string `json:"cost"`
	TierID      bool   `json:"tierId,omitempty"`
	Priority    int32  `json:"priority"`
	DailyLimit  int32  `json:"dailyLimit"`
	TotalLimit  int32  `json:"totalLimit"`
	Stock       int32  `json:"stock"`
	MinLevel    int32  `json:"minLevel"`
	MinVIP      int32  `json:"minVip"`
	DiscountBps int32  `json:"discountBps"`
	Weight      int64  `json:"weight"`
}
type p4CommerceRules struct {
	Products           []p4Product `json:"products"`
	RankPublishedAt    int64       `json:"rankPublishedAt,omitempty"`
	FreeCap            int32       `json:"freeCap,omitempty"`
	FreeSeconds        int64       `json:"freeSeconds,omitempty"`
	ResourceRefreshCap int32       `json:"resourceRefreshCap,omitempty"`
	RefreshItem        string      `json:"refreshItem,omitempty"`
	RefreshCost        string      `json:"refreshCost,omitempty"`
	Slots              int         `json:"slots,omitempty"`
	Discounts          []int32     `json:"discounts,omitempty"`
}
type p4CommerceProgress struct {
	Day          string           `json:"day"`
	Bought       map[string]int32 `json:"bought"`
	Total        map[string]int32 `json:"total"`
	Version      int64            `json:"version"`
	Slots        []string         `json:"slots,omitempty"`
	FreeLeft     int32            `json:"freeLeft"`
	FreeAt       int64            `json:"freeAt"`
	ResourceUsed int32            `json:"resourceUsed"`
	DiscountBps  int32            `json:"discountBps"`
}

func p4BaseProduct(id, reward, cost string, index, level int32) p4Product {
	return p4Product{ID: id, Index: index, Rewards: reward, Cost: cost, Priority: 1000 + index, DailyLimit: 20, Stock: -1, MinLevel: level, DiscountBps: 10000, Weight: 1}
}
func (s *GameService) seedP4Commerce(tx *gorm.DB, now time.Time) error {
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	end := start.AddDate(0, 1, 0)
	ordinary := p4CommerceRules{}
	for i, reward := range []string{"energy,10", "stamina,25", "item_001,1", "item_002,1", "hero_exp_001,1", "item_016,1", "item_013,1"} {
		id := fmt.Sprintf("itemShopId_%d", i+1)
		cost, err := s.p4TierCost(id, 1, 1)
		if err != nil {
			return err
		}
		product := p4BaseProduct(id, reward, fmt.Sprintf("%s,%d", cost.GetId(), cost.GetNum()), int32(i+1), 1)
		product.TierID = true
		ordinary.Products = append(ordinary.Products, product)
	}
	memory := p4CommerceRules{FreeCap: 10, FreeSeconds: 7200, ResourceRefreshCap: 15, RefreshItem: "item_013,1", RefreshCost: "diamond,20", Slots: 4}
	for i, spec := range []struct{ reward, cost string }{{"frag_hero_001,1", "light,20"}, {"frag_hero_025,1", "light,20"}, {"item_005,5", "light,15"}, {"hero_exp_002,1", "light,30"}, {"item_006,1", "light,30"}, {"item_013,1", "light,10"}} {
		p := p4BaseProduct(fmt.Sprintf("p4Memory_%d", i+1), spec.reward, spec.cost, int32(i+1), 16)
		p.DailyLimit = 1
		memory.Products = append(memory.Products, p)
	}
	popularity := p4CommerceRules{Products: []p4Product{p4BaseProduct("p4Popularity_1", "item_001,1", "popularity,20", 1, 10), p4BaseProduct("p4Popularity_2", "frag_hero_001,1", "popularity,100", 2, 10)}}
	point := p4CommerceRules{Products: []p4Product{p4BaseProduct("p4Point_1", "item_021,1", "point,100", 1, 22), p4BaseProduct("p4Point_2", "equip_037,1", "item_021,10", 2, 22)}}
	point.Products[1].Priority = 2001
	point.Products[1].DailyLimit = 1
	score := p4CommerceRules{Products: []p4Product{p4BaseProduct("p4Score_1", "item_005,5", "score,50", 1, 35), p4BaseProduct("p4Score_2", "item_004,1", "score,100", 2, 35)}}
	half := p4CommerceRules{Products: []p4Product{p4BaseProduct("p4Half_1", "item_006,2", "diamond,40", 1, 1)}}
	half.Products[0].DiscountBps = 5000
	half.Products[0].DailyLimit = 1
	half.Products[0].Stock = 100
	discount := p4CommerceRules{Products: []p4Product{p4BaseProduct("p4Discount_1", "item_005,10", "diamond,50", 1, 1)}}
	discount.Products[0].DiscountBps = 8000
	discount.Products[0].DailyLimit = 5
	exchange := p4CommerceRules{Products: []p4Product{p4BaseProduct("p4Essence_1", "equip_037,1", "item_021,10", 1, 22)}}
	exchange.Products[0].DailyLimit = 1
	sale := p4CommerceRules{Products: []p4Product{p4BaseProduct("p4Flash_1", "item_013,1", "diamond,20", 1, 1)}}
	sale.Products[0].Stock = 30
	sale.Products[0].DailyLimit = 1
	mystic := p4CommerceRules{Discounts: []int32{5000, 7000, 8000}, Products: []p4Product{p4BaseProduct("p4Mystic_1", "item_048,1", "diamond,300", 1, 1), p4BaseProduct("p4Mystic_2", "hero_exp_006,1", "diamond,100", 2, 1)}}
	for _, spec := range []struct {
		kind   string
		shop   int32
		prefix string
		rules  p4CommerceRules
	}{
		{"ordinary", 1, "p4:ordinary:", ordinary}, {"memory", 3, "p4:memory:", memory}, {"popularity", 4, "p4:popularity:", popularity}, {"point", 5, "p4:point:", point}, {"score", 7, "p4:score:", score},
		{"half", 0, "halfDiscountSale_p4_", half}, {"discount", 0, "discountSale_p4_", discount}, {"exchange", 0, "limitExchange_p4_", exchange}, {"sale", 0, "limitSale_p4_", sale}, {"mystic", 0, "mysticDiscount_p4_", mystic},
	} {
		raw, err := json.Marshal(spec.rules)
		if err != nil {
			return err
		}
		cfg := db.P4CommerceRule{ID: spec.prefix + start.Format("200601"), Kind: spec.kind, ShopType: spec.shop, Enabled: false, StartAt: start.UnixMilli(), EndAt: end.UnixMilli(), ClaimEndAt: end.UnixMilli(), Rules: raw, Source: "本地兼容服目录/限购/库存：原普通商品位价格档与圣坛2小时/10次免费/刷新券优先/20钻规则保留；声望、试炼、悬赏货币来源分别由P11/P7/P12联动，神装精华使用item_021，红武ID复用P2核实equip_037。折扣和新商品投放均为SQL本地配置。"}
		var exists int64
		if err = tx.Model(&db.P4CommerceRule{}).Where("id=?", cfg.ID).Count(&exists).Error; err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		if _, err = s.validateP4Commerce(cfg); err != nil {
			return err
		}
		if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error; err != nil {
			return err
		}
	}
	if err := s.seedP5SpecialtySupply(tx, now); err != nil {
		return err
	}
	if err := s.seedP5GemSupply(tx, now); err != nil {
		return err
	}
	if e := s.seedP5GoldEggSupply(tx, now); e != nil {
		return e
	}
	if e := s.seedP8IngredientSupply(tx, now); e != nil {
		return e
	}
	if e := s.seedP15RedOSSupply(tx, now); e != nil {
		return e
	}
	if e := s.seedP5CrystalSupply(tx, now); e != nil {
		return e
	}
	if e := s.seedP4LuckySupply(tx, now); e != nil {
		return e
	}
	if err := s.seedP5WorldSeedSupply(tx, now); err != nil {
		return err
	}
	if err := s.seedP5AwakeMaterialSupply(tx, now); err != nil {
		return err
	}
	if err := s.seedP5GrowthResourceSupply(tx, now); err != nil {
		return err
	}
	if err := s.seedP5AwakeElementSupply(tx, now); err != nil {
		return err
	}
	if err := s.seedP5GloryFragmentSupply(tx, now); err != nil {
		return err
	}
	if err := s.seedP9PetFoodSupply(tx, now); err != nil {
		return err
	}
	if err := s.seedP4PopularityRank(tx, now); err != nil {
		return err
	}
	return s.seedP4GuildShop(tx, now)
}

func (s *GameService) p4ParseCost(content string) (*pb.BaseResource, error) {
	pair := strings.Split(content, ",")
	if len(pair) != 2 {
		return nil, errors.New("交易成本格式无效")
	}
	n, err := p1PositiveAmount(pair[1])
	if err != nil {
		return nil, err
	}
	switch pair[0] {
	case "coin", "diamond", "energy", "stamina", "light", "point", "popularity", "score":
	default:
		if !strings.HasPrefix(pair[0], "item_") || s.table("Item", "itemId", pair[0]) == nil {
			return nil, errors.New("交易货币或道具未恢复")
		}
	}
	return &pb.BaseResource{Id: proto.String(pair[0]), Num: proto.Int32(int32(n))}, nil
}
func (s *GameService) validateP4Commerce(cfg db.P4CommerceRule) (p4CommerceRules, error) {
	var rules p4CommerceRules
	if cfg.ID == "" || len(cfg.ID) > 64 || cfg.StartAt <= 0 || cfg.StartAt >= cfg.EndAt || json.Unmarshal(cfg.Rules, &rules) != nil || len(rules.Products) < 1 || len(rules.Products) > 100 {
		return rules, errors.New("商店实例规则无效")
	}
	ids := map[string]bool{}
	indices := map[int32]bool{}
	for _, p := range rules.Products {
		if p.ID == "" || len(p.ID) > 64 || len(p.Name) > 128 || ids[p.ID] || p.Index < 1 || indices[p.Index] || p.DailyLimit < 1 || p.DailyLimit > 10000 || p.TotalLimit < 0 || p.Stock < -1 || p.MinLevel < 1 || p.MinVIP < 0 || p.DiscountBps < 1 || p.DiscountBps > 10000 || p.Weight < 1 {
			return rules, errors.New("商品库存/资格/折扣无效")
		}
		ids[p.ID] = true
		indices[p.Index] = true
		ok, err := s.openServerRewardSupported(p.Rewards)
		if err != nil || !ok {
			return rules, errors.New("商品奖励未恢复")
		}
		if _, err = s.p4ParseCost(p.Cost); err != nil {
			return rules, err
		}
		if p.TierID {
			if _, err = s.p4TierCost(p.ID, 1, p.DailyLimit); err != nil {
				return rules, err
			}
		}
	}
	if cfg.Kind == "memory" {
		if rules.FreeCap != 10 || rules.FreeSeconds != 7200 || rules.ResourceRefreshCap < 1 || rules.Slots < 1 || rules.Slots > len(rules.Products) || rules.RefreshItem != "item_013,1" || rules.RefreshCost != "diamond,20" {
			return rules, errors.New("圣坛刷新不匹配原客户端规则")
		}
	}
	for _, bps := range rules.Discounts {
		if bps < 1 || bps > 10000 {
			return rules, errors.New("神秘小屋折扣池无效")
		}
	}
	if err := s.p4PopularityRankRules(cfg, rules); err != nil {
		return rules, err
	}
	return rules, nil
}

// p4CommerceSnapshot 让投放列表和交易读取同一冻结目录，后台编辑只影响尚未冻结的实例。
func (s *GameService) p4CommerceSnapshot(live db.P4CommerceRule) (db.P4CommerceRule, p4CommerceRules, error) {
	cfg := live
	if len(live.Frozen) > 0 {
		if json.Unmarshal(live.Frozen, &cfg) != nil || cfg.ID != live.ID || cfg.ShopType != live.ShopType {
			return cfg, p4CommerceRules{}, errors.New("目录冻结存档损坏")
		}
		cfg.Enabled = live.Enabled
	}
	rules, err := s.validateP4Commerce(cfg)
	return cfg, rules, err
}

// p4TierCost 逐份跨越原表价格档，批量购买不能以首份价格乘数量而少扣或多扣。
func (s *GameService) p4TierCost(id string, ordinal, count int32) (*pb.BaseResource, error) {
	if ordinal < 1 || count < 1 || count > 10000 {
		return nil, errors.New("商品价格序号无效")
	}
	var total int64
	kind := ""
	for n := int32(0); n < count; n++ {
		found := false
		for _, row := range s.Tables["ItemShopPrice"] {
			if row["shopItemId"] != id {
				continue
			}
			bounds := strings.Split(row["count"], "-")
			first, err := strconv.ParseInt(bounds[0], 10, 32)
			if err != nil {
				return nil, err
			}
			last := first
			if len(bounds) > 1 {
				last, err = strconv.ParseInt(bounds[1], 10, 32)
				if err != nil {
					return nil, err
				}
			}
			if int64(ordinal+n) < first || int64(ordinal+n) > last {
				continue
			}
			if found {
				return nil, errors.New("原商品价格档重叠")
			}
			cost, err := s.p4ParseCost(row["price"])
			if err != nil {
				return nil, err
			}
			if kind != "" && kind != cost.GetId() {
				return nil, errors.New("跨价格档货币不一致")
			}
			kind = cost.GetId()
			total += int64(cost.GetNum())
			found = true
		}
		if !found || total > math.MaxInt32 {
			return nil, errors.New("商品价格档缺失或总价溢出")
		}
	}
	return &pb.BaseResource{Id: proto.String(kind), Num: proto.Int32(int32(total))}, nil
}
func (s *GameService) loadP4Commerce(tx *gorm.DB, id int64, key string, shopType int32, now time.Time) (db.P4CommerceRule, p4CommerceRules, *db.P4CommerceState, *p4CommerceProgress, error) {
	var cfg db.P4CommerceRule
	var rules p4CommerceRules
	if err := s.seedP4Commerce(tx, now); err != nil {
		return cfg, rules, nil, nil, err
	}
	if key == "" {
		shopKey := map[int32]string{1: "ordinary", 3: "memory", 4: "popularity", 5: "point", 7: "score", 8: "guild"}[shopType]
		if shopKey != "" {
			scheduled, managed, err := s.AdminScheduledOrdinaryShopConsumer(tx, shopKey, now.UnixMilli())
			if err != nil {
				return cfg, rules, nil, nil, err
			}
			if managed && scheduled == "" {
				return cfg, rules, nil, nil, constUtil.ErrP4CommerceNotOpen
			}
			if managed {
				key = scheduled
			}
		}
	}
	if key != "" {
		allowed, err := localPlayActivityAccess(tx, key, false)
		if err != nil {
			return cfg, rules, nil, nil, err
		}
		if !allowed {
			return cfg, rules, nil, nil, errors.New("商店实例不属于当前角色")
		}
	}
	if strings.HasPrefix(key, constUtil.P15RedOSSupplyPrefix) {
		allowed, err := p15RedOSSupplyAllowed(tx)
		if err != nil {
			return cfg, rules, nil, nil, err
		}
		if !allowed {
			return cfg, rules, nil, nil, errors.New("单人人物获取供应尚未开放")
		}
	}
	if strings.HasPrefix(key, constUtil.P5GoldEggSupplyPrefix) {
		if e := s.p5GoldEggSupplyGuard(tx); e != nil {
			return cfg, rules, nil, nil, e
		}
	}
	if strings.HasPrefix(key, constUtil.P4LuckySupplyPrefix) {
		ready, e := p4LuckySupplyReady(tx)
		if e != nil {
			return cfg, rules, nil, nil, e
		}
		if !ready {
			return cfg, rules, nil, nil, errors.New("烟花和福袋用途尚未开放")
		}
	}
	query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("enabled = ?", true)
	if key != "" {
		query = query.Where("id = ?", key)
	} else {
		query = query.Where("shop_type = ? AND id NOT LIKE ?", shopType, "shop_schedule_%")
		// Rank-tab offers append independently; they never replace the original
		// monthly ordinary catalogue or reset its existing per-player quota.
		if shopType == 4 {
			query = query.Where("kind <> ?", constUtil.P4PopularityRankKind)
		}
	}
	var configs []db.P4CommerceRule
	if err := query.Order("start_at DESC,id DESC").Find(&configs).Error; err != nil {
		return cfg, rules, nil, nil, err
	}
	for _, live := range configs {
		// 具名私期允许本人读取旧期；无实例入口只选当前可见目录。
		if key == "" {
			allowed, err := localPlayActivityVisible(tx, live.ID)
			if err != nil {
				return cfg, rules, nil, nil, err
			}
			if !allowed {
				continue
			}
		}
		var err error
		cfg, rules, err = s.p4CommerceSnapshot(live)
		if err != nil {
			return cfg, rules, nil, nil, err
		}
		open, windowErr := p4ShopSourceWindow(tx, cfg, now)
		if windowErr != nil {
			return cfg, rules, nil, nil, windowErr
		}
		if !open {
			continue
		}
		if len(live.Frozen) == 0 {
			raw, _ := json.Marshal(cfg)
			if err = tx.Model(&live).Update("frozen", raw).Error; err != nil {
				return cfg, rules, nil, nil, err
			}
		}
		row := &db.P4CommerceState{PlayerID: id, RuleID: cfg.ID}
		state := &p4CommerceProgress{Day: p1Day(now), Bought: map[string]int32{}, Total: map[string]int32{}, Version: 1, FreeLeft: 1, FreeAt: now.UnixMilli()}
		err = tx.Where("player_id = ? AND rule_id = ?", id, cfg.ID).First(row).Error
		if err == nil {
			if json.Unmarshal(row.Payload, state) != nil {
				return cfg, rules, nil, nil, errors.New("商店进度损坏")
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return cfg, rules, nil, nil, err
		}
		if state.Bought == nil {
			state.Bought = map[string]int32{}
		}
		if state.Total == nil {
			state.Total = map[string]int32{}
		}
		if err := localPlayCommerceDay(tx, row, state, now); err != nil {
			return cfg, rules, nil, nil, err
		}
		if cfg.Kind == "memory" {
			if state.FreeLeft < 0 || state.FreeLeft > rules.FreeCap || state.ResourceUsed < 0 || state.ResourceUsed > rules.ResourceRefreshCap || state.FreeAt <= 0 || state.FreeAt > now.UnixMilli() {
				return cfg, rules, nil, nil, errors.New("圣坛刷新计时存档无效")
			}
			seconds := rules.FreeSeconds * 1000
			elapsed := (now.UnixMilli() - state.FreeAt) / seconds
			if elapsed > 0 {
				state.FreeLeft = int32(min(int64(rules.FreeCap), int64(state.FreeLeft)+elapsed))
				state.FreeAt += elapsed * seconds
			}
			if len(state.Slots) == 0 {
				state.Slots, err = p4SampleSlots(rules)
				if err != nil {
					return cfg, rules, nil, nil, err
				}
			}
		}
		return cfg, rules, row, state, nil
	}
	return cfg, rules, nil, nil, constUtil.ErrP4CommerceNotOpen
}
func saveP4Commerce(tx *gorm.DB, row *db.P4CommerceState, state *p4CommerceProgress) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	row.Payload = raw
	return tx.Save(row).Error
}
func p4SampleSlots(rules p4CommerceRules) ([]string, error) {
	candidates := append([]p4Product(nil), rules.Products...)
	var selected []string
	for len(selected) < rules.Slots {
		var total int64
		for _, p := range candidates {
			if total > math.MaxInt64-p.Weight {
				return nil, errors.New("刷新商品权重溢出")
			}
			total += p.Weight
		}
		draw, err := rand.Int(rand.Reader, big.NewInt(total))
		if err != nil {
			return nil, err
		}
		pick := draw.Int64()
		index := -1
		for i, p := range candidates {
			pick -= p.Weight
			if pick < 0 {
				index = i
				break
			}
		}
		if index < 0 {
			return nil, errors.New("刷新目录抽取失败")
		}
		selected = append(selected, candidates[index].ID)
		candidates = append(candidates[:index], candidates[index+1:]...)
	}
	sort.Strings(selected)
	return selected, nil
}
func p4SlotKey(cfg db.P4CommerceRule, state *p4CommerceProgress, p p4Product) string {
	if cfg.Kind == "memory" {
		return fmt.Sprintf("%d:%s", state.Version, p.ID)
	}
	return p.ID
}
func p4Visible(cfg db.P4CommerceRule, state *p4CommerceProgress, p p4Product) bool {
	if cfg.Kind != "memory" {
		return true
	}
	for _, id := range state.Slots {
		if id == p.ID {
			return true
		}
	}
	return false
}
func (s *GameService) p4Price(p p4Product, state *p4CommerceProgress, ordinal, count int32) (*pb.BaseResource, error) {
	base, err := s.p4ParseCost(p.Cost)
	if err != nil {
		return nil, err
	}
	total := int64(base.GetNum()) * int64(count)
	if p.TierID {
		base, err = s.p4TierCost(p.ID, ordinal, count)
		if err != nil {
			return nil, err
		}
		total = int64(base.GetNum())
	}
	bps := p.DiscountBps
	if state.DiscountBps > 0 {
		bps = state.DiscountBps
	}
	total = (total*int64(bps) + 9999) / 10000
	if total < 1 || total > math.MaxInt32 {
		return nil, errors.New("折扣价格超出协议")
	}
	base.Num = proto.Int32(int32(total))
	return base, nil
}
func (s *GameService) p4Projection(tx *gorm.DB, p *pb.Player, cfg db.P4CommerceRule, rules p4CommerceRules, state *p4CommerceProgress) ([]*pb.ShopItem, []*pb.ActivityShopItem, error) {
	var shop []*pb.ShopItem
	var activity []*pb.ActivityShopItem
	rank := int64(0)
	if cfg.Kind == constUtil.P4PopularityRankKind {
		var err error
		rank, _, err = P11ArenaVerifiedBestWonRank(tx, p.GetPlayerId())
		if err != nil {
			return nil, nil, err
		}
	}
	for _, product := range rules.Products {
		if !p4Visible(cfg, state, product) {
			continue
		}
		key := p4SlotKey(cfg, state, product)
		bought := state.Bought[key]
		if state.Total[product.ID] < 0 {
			return nil, nil, errors.New("商品总限购存档无效")
		}
		if bought < 0 || bought > product.DailyLimit {
			return nil, nil, errors.New("商品日限购存档无效")
		}
		left := product.DailyLimit - bought
		allLeft := int32(math.MaxInt32)
		if product.TotalLimit > 0 {
			allLeft = product.TotalLimit - state.Total[product.ID]
			left = min(left, allLeft)
		}
		if product.Stock >= 0 {
			var stock db.P4CommerceStock
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("rule_id = ? AND product_id = ?", cfg.ID, product.ID).First(&stock).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, nil, err
			}
			if stock.Used < 0 || stock.Used > product.Stock {
				return nil, nil, errors.New("共享库存存档无效")
			}
			left = min(left, product.Stock-stock.Used)
		}
		price, err := s.p4Price(product, state, min(bought+1, product.DailyLimit), 1)
		if err != nil {
			return nil, nil, err
		}
		rewards, err := p1RewardResources(product.Rewards)
		if err != nil {
			return nil, nil, err
		}
		can := left
		if p.GetLevel() < product.MinLevel || p.GetVip() < product.MinVIP {
			can = 0
		}
		bps := product.DiscountBps
		if state.DiscountBps > 0 {
			bps = state.DiscountBps
		}
		condition := &pb.ShopCondition{Type: proto.String("level"), Value: proto.Int64(int64(product.MinLevel))}
		if cfg.ShopType == 4 || cfg.Kind == constUtil.P4PopularityRankKind {
			condition = nil
			if can > 0 {
				can = 1
			}
			if product.TotalLimit > 0 && state.Total[product.ID] >= product.TotalLimit {
				can = 2
			}
			if cfg.Kind == constUtil.P4PopularityRankKind {
				condition = &pb.ShopCondition{Type: proto.String("rank"), Value: proto.Int64(product.MaxRank)}
				if can != 2 && (rank < 1 || rank > product.MaxRank) {
					can = 0
				}
			}
		}
		shop = append(shop, &pb.ShopItem{Id: proto.String(product.ID), Item: rewards, Price: []*pb.BaseResource{price}, ExchangeList: []*pb.BaseResource{price}, Priority: proto.Int32(product.Priority), CanBuy: proto.Int32(max(0, can)), DayLeftTimes: proto.Int32(max(0, left)), DayTotalTimes: proto.Int32(product.DailyLimit), TotalLeftTimes: proto.Int32(max(0, allLeft)), TotalTimes: proto.Int32(max(product.TotalLimit, allLeft)), PerDiscount: proto.String(strconv.Itoa(int(bps / 100))), Condition: condition})
		status := int32(0)
		if can > 0 {
			status = 1
		}
		// Original OnSailItem formats ActivityShopItem.name directly into its title.
		// Keep the transaction SKU unchanged and resolve only this display fallback.
		name := product.Name
		if name == "" {
			name = constUtil.P4ResourceBundleDisplayName
			if len(rewards) == 1 {
				name = rewards[0].GetId()
				if row := s.table("Item", "itemId", name); row != nil && row["name"] != "" {
					name = row["name"]
				}
				if rewards[0].GetId() == "equip_037" {
					name = constUtil.P4EssenceDisplayName
				}
			}
		}
		activity = append(activity, &pb.ActivityShopItem{Index: proto.Int32(product.Index), GetResource: rewards, CostResource: []*pb.BaseResource{price}, Total: proto.Int32(product.DailyLimit), Left: proto.Int32(max(0, left)), Discount: proto.String(strconv.FormatFloat(float64(bps)/10000, 'f', 2, 64)), PickOneOrGetAll: proto.Int32(1), Status: proto.Int32(status), Name: proto.String(name)})
	}
	return shop, activity, nil
}
func p4ResourceBalance(p *pb.Player, g *model.GameState, key string) int64 {
	switch key {
	case "light":
		return int64(p.GetLight())
	case "point":
		return int64(p.GetPoint())
	case "popularity":
		return int64(p.GetPopularity())
	case "score":
		return int64(p.GetScore())
	case "energy":
		return int64(p.GetEnergy())
	case "stamina":
		return int64(p.GetStamina())
	default:
		return simpleOwned(p, g, key)
	}
}
func (s *GameService) p4Deduct(tx *gorm.DB, id int64, p *pb.Player, g *model.GameState, cost *pb.BaseResource, source string, now time.Time) error {
	if cost == nil || cost.GetNum() <= 0 || p4ResourceBalance(p, g, cost.GetId()) < int64(cost.GetNum()) {
		return errors.New("交易资源不足")
	}
	n := cost.GetNum()
	switch cost.GetId() {
	case "light":
		p.Light = proto.Int32(p.GetLight() - n)
	case "point":
		p.Point = proto.Int32(p.GetPoint() - n)
	case "popularity":
		p.Popularity = proto.Int32(p.GetPopularity() - n)
	case "score":
		p.Score = proto.Int32(p.GetScore() - n)
	case "energy":
		p.Energy = proto.Int32(p.GetEnergy() - n)
	case "stamina":
		p.Stamina = proto.Int32(p.GetStamina() - n)
	default:
		if err := deductPoolCost(p, g, cost); err != nil {
			return err
		}
	}
	if cost.GetId() == "score" {
		return s.RecordP1Event(tx, id, source, "spend:score", int64(n), now)
	}
	return s.RecordP1Cost(tx, id, source, []*pb.BaseResource{cost}, now)
}
func p4ScaleRewards(content string, count int32) (string, error) {
	var parts []string
	for _, part := range strings.Split(content, ";") {
		pair := strings.Split(part, ",")
		if len(pair) != 2 {
			return "", errors.New("商品奖励格式无效")
		}
		n, err := p1PositiveAmount(pair[1])
		if err != nil || n*int64(count) > math.MaxInt32 {
			return "", errors.New("批量奖励溢出")
		}
		parts = append(parts, pair[0]+","+strconv.FormatInt(n*int64(count), 10))
	}
	return strings.Join(parts, ";"), nil
}

// p4OrdinaryProjection 为既有普通商店追加完整原价格档目录，查询不覆盖玩家限购状态。
func (s *GameService) p4OrdinaryProjection(tx *gorm.DB, p *pb.Player, now time.Time) ([]*pb.ShopItem, error) {
	if !s.P4Ready {
		return nil, nil
	}
	cfg, rules, _, state, err := s.loadP4Commerce(tx, p.GetPlayerId(), "", 1, now)
	// 原登录初始化必查普通商店；未配置扩展目录返回空列表，不能阻断登录。
	if errors.Is(err, constUtil.ErrP4CommerceNotOpen) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	items, _, err := s.p4Projection(tx, p, cfg, rules, state)
	return items, err
}
func p4IsOrdinaryProduct(id string) bool {
	for i := 1; i <= 7; i++ {
		if id == fmt.Sprintf("itemShopId_%d", i) {
			return true
		}
	}
	return false
}

// p4Commerce 使用冻结目录、服务器总价和行锁库存交易；调用方报价只能确认已展示选项。
func (s *GameService) p4Commerce(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, error) {
	var response proto.Message
	var guidePlayer *pb.Player
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var profile db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
			return err
		}
		p, g, err := decodeProfileForCalendarUpdate(tx, &profile)
		if err != nil {
			return err
		}
		var key, productID string
		var shopType, index int32
		count := int32(1)
		query := false
		refresh := false
		mystic := false
		var quotedCost, quotedGain []*pb.BaseResource
		switch b := request.(type) {
		case *pb.CSMysticDiscountInfo:
			key = "mysticDiscount_p4_" + now.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
			cycle, e := localPlayEntryCycle(tx, "MysticDiscount")
			if e != nil {
				return e
			}
			if cycle != nil {
				key = "mysticDiscount_p4_" + cycle.ID
			} else {
				key, e = AdminCurrentCommerceRuleKey(tx, constUtil.AdminCommerceMysticKind, now.UnixMilli(), key)
				if e != nil {
					return e
				}
			}
			query = true
		case *pb.CSShopProductList:
			shopType = b.GetShopType()
			query = true
		case *pb.CSShopBuy:
			shopType = b.GetShopType()
			productID = b.GetShopItemId()
			count = b.GetBuyCount()
		case *pb.CSRefreshShopQuery:
			shopType = b.GetShopType()
			query = true
		case *pb.CSRefreshShopRefresh:
			shopType = b.GetShopType()
			refresh = true
		case *pb.CSRefreshShopBuy:
			shopType = b.GetShopType()
			productID = b.GetShopItemId()
		case *pb.CSActivityShopInfo:
			key = b.GetActivityId()
			query = true
		case *pb.CSGetActivityShopReward:
			key = b.GetActivityId()
			index = b.GetIndex()
			quotedCost = b.GetCostResources()
			quotedGain = b.GetGetResources()
		case *pb.CSBuyActivityShop:
			key = b.GetActivityId()
			index = b.GetIndex()
			count = b.GetNum()
			quotedCost = b.GetCostResources()
			quotedGain = b.GetGetResources()
		case *pb.CSMysticDiscount:
			key = b.GetActivityId()
			mystic = true
		default:
			return errors.New("交易请求类型无效")
		}
		domain := fmt.Sprintf("commerce:%d", shopType)
		if key != "" {
			domain = key
		}
		if !query {
			prior, err := p4ReadReceipt(tx, id, domain, stamp, request)
			if err != nil {
				return err
			}
			if prior != nil {
				response = prior
				return nil
			}
		}
		// The original client stores one activity ID per type. Keep its outer ID for receipts,
		// and resolve purchases into the existing child transaction and quota.
		rankPurchase := shopType == 4 && strings.HasPrefix(productID, constUtil.P4PopularityRankProductPrefix)
		if rankPurchase {
			// Acquire the existing ordinary directory before all rank offers, matching
			// list/ordinary-buy lock order across different players.
			if _, _, _, _, err = s.loadP4Commerce(tx, id, "", 4, now); err != nil && !errors.Is(err, constUtil.ErrP4CommerceNotOpen) {
				return err
			}
			key, err = s.p4PopularityRankResolve(tx, productID, now)
			if err != nil {
				return err
			}
		}
		aggregateKey := ""
		if strings.HasPrefix(key, constUtil.P4ResourceExchangePrefix) {
			aggregateKey = key
			if mystic || refresh {
				return errors.New("资源兑换请求类型无效")
			}
			if query {
				response, err = s.p4ExchangeProjection(tx, id, p, g, key, now)
				return err
			}
			binding, e := s.p4ExchangeResolve(tx, key, index, now)
			if e != nil {
				return e
			}
			if binding.Domain == "red-p2" {
				if count != 1 {
					return errors.New("原红色兑换每次一份")
				}
				redRequest := &pb.CSGetActivityShopReward{ActivityId: proto.String(binding.ChildID), Index: proto.Int32(binding.ChildIndex), CostResources: quotedCost, GetResources: quotedGain}
				result := &pb.SCGetActivityShopReward{}
				if e = s.redExchangeApply(tx, id, redRequest, stamp, result, p, g); e != nil {
					return e
				}
				projection, e := s.p4ExchangeProjection(tx, id, p, g, aggregateKey, now)
				if e != nil {
					return e
				}
				result.ShopItemList = projection.ShopItemList
				response = result
				if e = p4SaveReceipt(tx, id, domain, stamp, request, response); e != nil {
					return e
				}
				return saveProfile(tx.WithContext(context.WithValue(tx.Statement.Context, constUtil.GameRequestContextKey, nil)), &profile, p, g)
			}
			key, index, productID = binding.ChildID, binding.ChildIndex, binding.ProductID
		}
		if err := s.p15SeasonalGuard(tx, key, now); err != nil {
			return err
		}
		cfg, rules, row, state, err := s.loadP4Commerce(tx, id, key, shopType, now)
		if errors.Is(err, constUtil.ErrP4CommerceNotOpen) && query && key == "" {
			if _, ok := request.(*pb.CSShopProductList); ok {
				var items []*pb.ShopItem
				if shopType == 4 {
					// The normal merchandise period and rank-reward period are
					// independent; a closed normal tab cannot hide published rewards.
					items, err = s.p4PopularityShopItems(tx, id, p, db.P4CommerceRule{}, nil, now)
					if err != nil {
						return err
					}
					if p4PopularityEmptyGuide(p, items) {
						guidePlayer = p
					}
				}
				response = &pb.SCShopProductList{ShopType: proto.Int32(shopType), ShopItemList: items}
				if guidePlayer != nil || len(items) > 0 {
					return saveProfile(tx.WithContext(context.WithValue(tx.Statement.Context, constUtil.GameRequestContextKey, nil)), &profile, p, g)
				}
				return nil
			}
		}
		if err != nil {
			return err
		}
		cost := &pb.BaseResource{}
		gain := &pb.ResultObject{}
		source := fmt.Sprintf("p4:%s:%d", cfg.ID, stamp)
		if mystic {
			if cfg.Kind != "mystic" || len(rules.Discounts) == 0 {
				return errors.New("神秘小屋折扣未开放")
			}
			if state.DiscountBps == 0 {
				draw, err := rand.Int(rand.Reader, big.NewInt(int64(len(rules.Discounts))))
				if err != nil {
					return err
				}
				state.DiscountBps = rules.Discounts[draw.Int64()]
			}
			response = &pb.SCMysticDiscount{Discount: proto.Float64(float64(state.DiscountBps) / 10000)}
		} else if refresh {
			if cfg.Kind != "memory" || p.GetLevel() < 16 {
				return errors.New("回忆圣坛刷新未解锁")
			}
			if state.FreeLeft > 0 {
				state.FreeLeft--
			} else {
				if state.ResourceUsed >= rules.ResourceRefreshCap {
					return errors.New("今日资源刷新次数已耗尽")
				}
				cost, err = s.p4ParseCost(rules.RefreshItem)
				if err != nil {
					return err
				}
				if p4ResourceBalance(p, g, cost.GetId()) < int64(cost.GetNum()) {
					cost, err = s.p4ParseCost(rules.RefreshCost)
					if err != nil {
						return err
					}
				}
				if err = s.p4Deduct(tx, id, p, g, cost, source, now); err != nil {
					return err
				}
				state.ResourceUsed++
			}
			state.Version++
			state.Slots, err = p4SampleSlots(rules)
			if err != nil {
				return err
			}
		} else if !query {
			if count < 1 || count > 100 {
				return errors.New("交易数量无效")
			}
			var selected *p4Product
			for i := range rules.Products {
				product := &rules.Products[i]
				if (productID != "" && product.ID == productID) || (index > 0 && product.Index == index) {
					selected = product
					break
				}
			}
			if selected == nil || !p4Visible(cfg, state, *selected) {
				return errors.New("当前目录无该商品")
			}
			selectedKey := p4SlotKey(cfg, state, *selected)
			bought := state.Bought[selectedKey]
			if state.Total[selected.ID] < 0 || state.Total[selected.ID] > math.MaxInt32-count || bought < 0 || bought > selected.DailyLimit-count || p.GetLevel() < selected.MinLevel || p.GetVip() < selected.MinVIP || (selected.TotalLimit > 0 && state.Total[selected.ID] > selected.TotalLimit-count) {
				return errors.New("商品资格或限购不足")
			}
			cost, err = s.p4Price(*selected, state, bought+1, count)
			if err != nil {
				return err
			}
			unit, err := s.p4Price(*selected, state, bought+1, 1)
			if err != nil {
				return err
			}
			resources, err := p1RewardResources(selected.Rewards)
			if err != nil {
				return err
			}
			totalContent, err := p4ScaleRewards(selected.Rewards, count)
			if err != nil {
				return err
			}
			totalResources, err := p1RewardResources(totalContent)
			if err != nil {
				return err
			}
			if key != "" && !rankPurchase && (!p1SameResources(quotedCost, []*pb.BaseResource{unit}) && !p1SameResources(quotedCost, []*pb.BaseResource{cost}) || !p1SameResources(quotedGain, resources) && !p1SameResources(quotedGain, totalResources)) {
				return errors.New("交易报价或选项不匹配冻结目录")
			}
			if err = s.p4PopularityRankBuy(tx, id, p, cfg, *selected, count, stamp, now); err != nil {
				return err
			}
			var stock db.P4CommerceStock
			if selected.Stock >= 0 {
				stock = db.P4CommerceStock{RuleID: cfg.ID, ProductID: selected.ID}
				e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("rule_id = ? AND product_id = ?", cfg.ID, selected.ID).First(&stock).Error
				if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
					return e
				}
				if stock.Used < 0 || stock.Used > selected.Stock-count {
					return errors.New("共享库存不足")
				}
				stock.Used += count
				if err = tx.Save(&stock).Error; err != nil {
					return err
				}
			}
			if err = s.p4Deduct(tx, id, p, g, cost, source, now); err != nil {
				return err
			}
			gain, err = s.GrantFixedReward(p, g, totalContent)
			if err != nil {
				return err
			}
			state.Bought[selectedKey] += count
			state.Total[selected.ID] += count
			if err = s.RecordP1Event(tx, id, source, constUtil.P1EventPurchase, int64(count), now); err != nil {
				return err
			}
		}
		if err = saveP4Commerce(tx, row, state); err != nil {
			return err
		}
		items, activity, err := s.p4Projection(tx, p, cfg, rules, state)
		if err != nil {
			return err
		}
		if shopType == 4 {
			items, err = s.p4PopularityShopItems(tx, id, p, cfg, items, now)
			if err != nil {
				return err
			}
			if _, listing := request.(*pb.CSShopProductList); listing && p4PopularityEmptyGuide(p, items) {
				guidePlayer = p
			}
		}
		next := int64(0)
		if cfg.Kind == "memory" && state.FreeLeft < rules.FreeCap {
			next = state.FreeAt + rules.FreeSeconds*1000
		}
		var costs []*pb.BaseResource
		if cost.GetNum() > 0 {
			costs = []*pb.BaseResource{cost}
		}
		switch request.(type) {
		case *pb.CSMysticDiscountInfo:
			response = &pb.SCMysticDiscountInfo{ActivityId: proto.String(cfg.ID), StartTime: proto.Int64(cfg.StartAt), EndTime: proto.Int64(cfg.EndAt), Msg: proto.String("活动期间可按页面显示的折扣购买商品。")}
		case *pb.CSShopProductList:
			response = &pb.SCShopProductList{ShopType: proto.Int32(shopType), ShopItemList: items}
		case *pb.CSShopBuy:
			if shopType == 1 {
				items, err = s.shopList(tx, p, p1Day(now))
				if err != nil {
					return err
				}
			}
			response = &pb.SCShopBuy{ShopType: proto.Int32(shopType), ShopItemList: items, Get: gain, Cost: costs}
		case *pb.CSRefreshShopQuery:
			response = &pb.SCRefreshShopQuery{ShopType: proto.Int32(3), ShopItemList: items, FreeLeftTimes: proto.Int32(state.FreeLeft), FreeMaxTimes: proto.Int32(rules.FreeCap), ResLeftTimes: proto.Int32(rules.ResourceRefreshCap - state.ResourceUsed), ResMaxTimes: proto.Int32(rules.ResourceRefreshCap), NextUpdateTime: proto.Int64(next)}
		case *pb.CSRefreshShopRefresh:
			response = &pb.SCRefreshShopRefresh{ShopType: proto.Int32(3), ShopItemList: items, FreeLeftTimes: proto.Int32(state.FreeLeft), FreeMaxTimes: proto.Int32(rules.FreeCap), ResLeftTimes: proto.Int32(rules.ResourceRefreshCap - state.ResourceUsed), ResMaxTimes: proto.Int32(rules.ResourceRefreshCap), Cost: costs, NextUpdateTime: proto.Int64(next)}
		case *pb.CSRefreshShopBuy:
			response = &pb.SCRefreshShopBuy{ShopType: proto.Int32(3), ShopItemList: items, Get: gain, Cost: costs, NextUpdateTime: proto.Int64(next)}
		case *pb.CSActivityShopInfo:
			discount := float64(1)
			if state.DiscountBps > 0 {
				discount = float64(state.DiscountBps) / 10000
			}
			response = &pb.SCActivityShopInfo{ActivityId: proto.String(cfg.ID), ShopItemList: activity, StartTime: proto.Int64(cfg.StartAt), EndTime: proto.Int64(cfg.EndAt), Param: proto.String(strconv.FormatFloat(discount, 'f', 2, 64))}
		case *pb.CSGetActivityShopReward, *pb.CSBuyActivityShop:
			response = &pb.SCGetActivityShopReward{Get: gain, ShopItemList: activity, CostResources: costs}
		}
		if aggregateKey != "" {
			projection, e := s.p4ExchangeProjection(tx, id, p, g, aggregateKey, now)
			if e != nil {
				return e
			}
			response.(*pb.SCGetActivityShopReward).ShopItemList = projection.ShopItemList
		}
		if !query {
			if err = p4SaveReceipt(tx, id, domain, stamp, request, response); err != nil {
				return err
			}
		}
		// 新商店自行记录准确消费/购买，原type1 hook不接管这些SKU；资源恢复和存档保存仍复用原逻辑。
		return saveProfile(tx.WithContext(context.WithValue(tx.Statement.Context, constUtil.GameRequestContextKey, nil)), &profile, p, g)
	})
	if err != nil {
		return nil, err
	}
	if guidePlayer != nil {
		return []proto.Message{response, &pb.SCPlayer{Player: guidePlayer}}, nil
	}
	return []proto.Message{response}, nil
}
