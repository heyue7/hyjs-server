package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
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

type p9MarketOffer struct {
	ID       string `json:"id"`
	Resource string `json:"resource"`
	Amount   int32  `json:"amount"`
	Currency string `json:"currency"`
	Price    int32  `json:"price"`
	Stock    int32  `json:"stock"`
	Weight   int64  `json:"weight"`
}
type p9MarketConfig struct {
	Offers      []p9MarketOffer  `json:"offers"`
	Slots       int              `json:"slots"`
	Decompose   map[string]int32 `json:"decompose"`
	RefreshCost string           `json:"refreshCost"`
	RefreshCap  int32            `json:"refreshCap"`
	Source      string           `json:"source"`
}

// initializeP9Market 保留已有运营配置，只创建完整原碎片和可用材料的本地目录。
func (s *GameService) initializeP9Market(tx *gorm.DB) error {
	if err := tx.AutoMigrate(&db.P9PetMarketRule{}, &db.P9PetMarketState{}); err != nil {
		return err
	}
	cfg := p9MarketConfig{Slots: 6, Decompose: map[string]int32{"18": 20, "20": 100, "23": 500}, RefreshCost: "diamond,150", RefreshCap: 10, Source: constUtil.P9MarketSource}
	for _, frag := range s.Tables["Frag"] {
		if !strings.HasPrefix(frag["id"], "frag_pet_") {
			continue
		}
		pet := s.table("Pet", "id", frag["target"])
		price := map[string]int32{"18": 10, "20": 50, "23": 250}[pet["quality"]]
		if price < 1 {
			return errors.New(constUtil.P9MarketError)
		}
		cfg.Offers = append(cfg.Offers, p9MarketOffer{ID: frag["id"], Resource: frag["id"], Amount: 5, Currency: "coinpet", Price: price, Stock: 1, Weight: 1})
	}
	if len(cfg.Offers) != 78 {
		return errors.New(constUtil.P9MarketError)
	}
	for _, spec := range []struct {
		id            string
		amount, price int32
	}{
		{"item_124", 10, 20}, {"item_125", 5, 40}, {"item_126", 2, 60}, {"item_127", 1, 80}, {"item_139", 1, 50}, {"item_150", 1, 100}, {"item_151", 1, 300}, {"item_152", 1, 40}, {"item_155", 1, 100},
		{"item_374", 5, 50}, {"item_375", 2, 100}, {"item_376", 1, 100}, {"item_391", 1, 100}, {"item_392", 1, 100}, {"item_393", 1, 100}, {"item_394", 1, 100},
	} {
		cfg.Offers = append(cfg.Offers, p9MarketOffer{ID: spec.id, Resource: spec.id, Amount: spec.amount, Currency: "coinpet", Price: spec.price, Stock: 1, Weight: 5})
	}
	// 原红精灵装备各类碎片与五属性觉醒消耗保持逐ID可投放，避免最高阶只靠测试材料。
	for number := 377; number <= 398; number++ {
		if number == 391 || number == 392 || number == 393 || number == 394 {
			continue
		}
		id := fmt.Sprintf("item_%03d", number)
		amount, price := int32(1), int32(150)
		if number >= 379 && number <= 390 {
			amount, price = 5, 100
		}
		if number == 378 {
			price = 500
		}
		cfg.Offers = append(cfg.Offers, p9MarketOffer{ID: id, Resource: id, Amount: amount, Currency: "coinpet", Price: price, Stock: 1, Weight: 5})
	}
	if err := s.validateP9MarketConfig(&cfg); err != nil {
		return err
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	// 原市场商品与兑换比例未恢复，停止自动开放本地目录，等待后台配置。
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P9PetMarketRule{ID: constUtil.P9MarketRuleID, Enabled: false, Input: raw, Source: constUtil.P9MarketSource}).Error
}

func (s *GameService) validateP9MarketConfig(cfg *p9MarketConfig) error {
	if cfg == nil || strings.TrimSpace(cfg.Source) == "" || len(cfg.Offers) < 1 || len(cfg.Offers) > 128 || cfg.Slots < 1 || cfg.Slots > 16 || cfg.Slots > len(cfg.Offers) || cfg.RefreshCost != "diamond,150" || cfg.RefreshCap != 10 || len(cfg.Decompose) != 3 {
		return errors.New(constUtil.P9MarketError)
	}
	if s.table("GameConfigure", "StaticName", "PetShopRefreshCost")["NameValue"] != cfg.RefreshCost || s.table("GameConfigure", "StaticName", "PetShopRefreshCount")["NameValue"] != strconv.Itoa(int(cfg.RefreshCap)) {
		return errors.New(constUtil.P9MarketError)
	}
	for quality, amount := range cfg.Decompose {
		if (quality != "18" && quality != "20" && quality != "23") || amount < 1 || amount > 1000000 {
			return errors.New(constUtil.P9MarketError)
		}
	}
	seen := map[string]bool{}
	for _, offer := range cfg.Offers {
		if offer.ID == "" || len(offer.ID) > 20 || strings.TrimSpace(offer.ID) != offer.ID || strings.ContainsAny(offer.ID, ":|;\r\n") || seen[offer.ID] || offer.Amount < 1 || offer.Amount > 10000 || offer.Price < 1 || offer.Stock < 1 || offer.Stock > 100 || offer.Weight < 1 || offer.Weight > 1000000000000 {
			return errors.New(constUtil.P9MarketError)
		}
		if _, err := s.p4ParseCost(fmt.Sprintf("%s,%d", offer.Currency, offer.Price)); err != nil {
			return err
		}
		if !(strings.HasPrefix(offer.Resource, "item_") && s.table("Item", "itemId", offer.Resource) != nil) && !(strings.HasPrefix(offer.Resource, "frag_pet_") && s.table("Frag", "id", offer.Resource) != nil) {
			return errors.New(constUtil.P9MarketError)
		}
		seen[offer.ID] = true
	}
	return nil
}

func p9MarketWeek(now time.Time) string {
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	return day.AddDate(0, 0, -(int(day.Weekday())+6)%7).Format("20060102")
}

func (s *GameService) p9MarketSelect(state *db.P9PetMarketState, cfg *p9MarketConfig) error {
	if state.Generation == math.MaxInt64 {
		return errors.New(constUtil.P9MarketError)
	}
	state.Generation++
	options := append([]p9MarketOffer(nil), cfg.Offers...)
	selected := make([]p9MarketOffer, 0, cfg.Slots)
	for i := 0; i < cfg.Slots; i++ {
		rewards := make([]db.GameplayPoolReward, 0, len(options))
		for index, offer := range options {
			rewards = append(rewards, db.GameplayPoolReward{ID: int32(index), Weight: offer.Weight, Amount: 1})
		}
		picked, err := choosePoolReward(rewards, false)
		if err != nil {
			return err
		}
		selected = append(selected, options[picked.ID])
		options = append(options[:picked.ID], options[picked.ID+1:]...)
	}
	raw, err := json.Marshal(selected)
	if err != nil {
		return err
	}
	state.Selected = raw
	state.SelectedHash = p6Digest(raw)
	state.Bought = []byte(`{}`)
	return nil
}

// p9MarketState 规则与库存均冻结，周切生成新代次且不覆盖历史回执。
func (s *GameService) p9MarketState(tx *gorm.DB, id int64, now time.Time) (*db.P9PetMarketState, *p9MarketConfig, error) {
	var live db.P9PetMarketRule
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&live, "id=?", constUtil.P9MarketRuleID).Error; err != nil {
		return nil, nil, err
	}
	if !live.Enabled {
		return nil, nil, errors.New(constUtil.P9MarketDisabled)
	}
	var state db.P9PetMarketState
	err := tx.First(&state, "player_id=?", id).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, err
	}
	week := p9MarketWeek(LocalPlayAt(tx.Statement.Context, now))
	if state.WeekKey > week {
		return nil, nil, errors.New(constUtil.P9MarketError)
	}
	cfg := &p9MarketConfig{}
	if len(live.Input) > 131072 || strings.TrimSpace(live.Source) == "" || json.Unmarshal(live.Input, cfg) != nil || s.validateP9MarketConfig(cfg) != nil {
		return nil, nil, errors.New(constUtil.P9MarketError)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) || state.WeekKey != week {
		state.PlayerID = id
		state.WeekKey = week
		state.Input = append([]byte(nil), live.Input...)
		state.InputHash = p6Digest(state.Input)
		state.RefreshUsed = 0
		if err := s.p9MarketSelect(&state, cfg); err != nil {
			return nil, nil, err
		}
		if err := tx.Save(&state).Error; err != nil {
			return nil, nil, err
		}
	} else {
		var oldCfg p9MarketConfig
		if state.InputHash != p6Digest(state.Input) || json.Unmarshal(state.Input, &oldCfg) != nil || s.validateP9MarketConfig(&oldCfg) != nil || state.Generation < 1 || state.RefreshUsed < 0 || state.RefreshUsed > cfg.RefreshCap {
			return nil, nil, errors.New(constUtil.P9MarketError)
		}
		if _, _, err := s.p9MarketInventory(&state, &oldCfg); err != nil {
			return nil, nil, err
		}
		if state.InputHash != p6Digest(live.Input) {
			if err := s.p9MarketUpdateLive(&state, cfg); err != nil {
				return nil, nil, err
			}
			state.Input = append([]byte(nil), live.Input...)
			state.InputHash = p6Digest(state.Input)
			if err := tx.Save(&state).Error; err != nil {
				return nil, nil, err
			}
		}
	}
	if _, _, err := s.p9MarketInventory(&state, cfg); err != nil {
		return nil, nil, err
	}
	return &state, cfg, nil
}

func (s *GameService) p9MarketInventory(state *db.P9PetMarketState, cfg *p9MarketConfig) ([]p9MarketOffer, map[string]int32, error) {
	var selected []p9MarketOffer
	var bought map[string]int32
	if state.SelectedHash != p6Digest(state.Selected) || json.Unmarshal(state.Selected, &selected) != nil || len(selected) != cfg.Slots || json.Unmarshal(state.Bought, &bought) != nil || bought == nil {
		return nil, nil, errors.New(constUtil.P9MarketError)
	}
	allowed := map[string]p9MarketOffer{}
	for _, offer := range cfg.Offers {
		allowed[offer.ID] = offer
	}
	seen := map[string]bool{}
	for _, offer := range selected {
		if seen[offer.ID] || allowed[offer.ID] != offer || bought[offer.ID] < 0 {
			return nil, nil, errors.New(constUtil.P9MarketError)
		}
		seen[offer.ID] = true
	}
	for id, n := range bought {
		if id == "" || n < 0 {
			return nil, nil, errors.New(constUtil.P9MarketError)
		}
	}
	return selected, bought, nil
}

func p9MarketSKU(state *db.P9PetMarketState, id string) string {
	return fmt.Sprintf("p9shop:%s:%d:%s", state.WeekKey, state.Generation, id)
}

func p9MarketQuoteSKU(state *db.P9PetMarketState, offer p9MarketOffer) string {
	return p9MarketSKU(state, offer.ID) + ":" + p6Digest(adminJSON(offer))[:16]
}

// Reconcile current selected identities in place. Operational edits never call
// p9MarketSelect, which deliberately starts a new refresh inventory generation.
func (s *GameService) p9MarketUpdateLive(state *db.P9PetMarketState, cfg *p9MarketConfig) error {
	var previous []p9MarketOffer
	if json.Unmarshal(state.Selected, &previous) != nil {
		return errors.New(constUtil.P9MarketError)
	}
	available := map[string]p9MarketOffer{}
	for _, offer := range cfg.Offers {
		available[offer.ID] = offer
	}
	next := make([]p9MarketOffer, 0, cfg.Slots)
	seen := map[string]bool{}
	for _, old := range previous {
		if offer, ok := available[old.ID]; ok && len(next) < cfg.Slots {
			next = append(next, offer)
			seen[offer.ID] = true
		}
	}
	for len(next) < cfg.Slots {
		var options []p9MarketOffer
		var weights []db.GameplayPoolReward
		for _, offer := range cfg.Offers {
			if !seen[offer.ID] {
				weights = append(weights, db.GameplayPoolReward{ID: int32(len(options)), Amount: 1, Weight: offer.Weight})
				options = append(options, offer)
			}
		}
		chosen, err := choosePoolReward(weights, false)
		if err != nil {
			return err
		}
		offer := options[chosen.ID]
		next = append(next, offer)
		seen[offer.ID] = true
	}
	raw, err := json.Marshal(next)
	if err != nil {
		return err
	}
	state.Selected = raw
	state.SelectedHash = p6Digest(raw)
	return nil
}

func (s *GameService) p9MarketListing(state *db.P9PetMarketState, cfg *p9MarketConfig) (*pb.SCPetShopProductList, error) {
	selected, bought, err := s.p9MarketInventory(state, cfg)
	if err != nil {
		return nil, err
	}
	out := &pb.SCPetShopProductList{RefreshTimes: proto.Int32(cfg.RefreshCap - state.RefreshUsed)}
	for index, offer := range selected {
		left := max(int32(0), offer.Stock-bought[offer.ID])
		price := &pb.BaseResource{Id: proto.String(offer.Currency), Num: proto.Int32(offer.Price)}
		out.ShopItemList = append(out.ShopItemList, &pb.ShopItem{Id: proto.String(p9MarketQuoteSKU(state, offer)), Item: []*pb.BaseResource{{Id: proto.String(offer.Resource), Num: proto.Int32(offer.Amount)}}, Price: []*pb.BaseResource{price}, ExchangeList: []*pb.BaseResource{proto.Clone(price).(*pb.BaseResource)}, Priority: proto.Int32(int32(index + 1)), CanBuy: proto.Int32(left), DayLeftTimes: proto.Int32(left), DayTotalTimes: proto.Int32(offer.Stock), TotalLeftTimes: proto.Int32(left), TotalTimes: proto.Int32(offer.Stock)})
	}
	return out, nil
}

// p9MarketReceiptCurrent 检查旧库存投影，后续购买或刷新后的旧目录不能回写当前UI。
func (s *GameService) p9MarketReceiptCurrent(tx *gorm.DB, id int64, response proto.Message) error {
	row, cfg, err := s.p9MarketState(tx, id, time.Now())
	if err != nil {
		return err
	}
	actual, err := s.p9MarketListing(row, cfg)
	if err != nil {
		return err
	}
	var expected []*pb.ShopItem
	switch result := response.(type) {
	case *pb.SCPetShopProductList:
		if result.GetRefreshTimes() != actual.GetRefreshTimes() {
			return errors.New(constUtil.P9MarketReceiptExpired)
		}
		expected = result.GetShopItemList()
	case *pb.SCPetShopBuy:
		expected = result.GetShopItemList()
	default:
		return errors.New(constUtil.P9MarketError)
	}
	if !proto.Equal(&pb.SCPetShopProductList{ShopItemList: expected}, &pb.SCPetShopProductList{ShopItemList: actual.GetShopItemList()}) {
		return errors.New(constUtil.P9MarketReceiptExpired)
	}
	return nil
}

// p9MarketMutation 的独立事务沿用P9回执表，并在同一Profile锁内校验库存代次。
func (s *GameService) p9MarketMutation(ctx context.Context, id int64, operation string, stamp int64, request, response proto.Message, apply func(*gorm.DB, *pb.Player, *model.GameState) error) ([]proto.Message, error) {
	if stamp <= 0 {
		return nil, p5Error()
	}
	raw, err := proto.Marshal(request)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		raw = []byte{}
	}
	err = s.p3Profile(ctx, id, func(tx *gorm.DB, row *db.Profile, p *pb.Player, g *model.GameState) error {
		var saved db.P9PetReceipt
		err := tx.First(&saved, "player_id=? AND operation=? AND timestamp=?", id, operation, stamp).Error
		if err == nil {
			if !bytes.Equal(saved.Request, raw) {
				return p5Error()
			}
			if err := proto.Unmarshal(saved.Response, response); err != nil {
				return err
			}
			return s.p9MarketReceiptCurrent(tx, id, response)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := s.requireShopAvailability(tx, "pet-market", LocalPlayNow(ctx)); err != nil {
			return err
		}
		if err := s.p9Open(p); err != nil {
			return err
		}
		if err := apply(tx, p, g); err != nil {
			return err
		}
		if err := saveProfile(tx, row, p, g); err != nil {
			return err
		}
		data, err := proto.Marshal(response)
		if err != nil {
			return err
		}
		if data == nil {
			data = []byte{}
		}
		return tx.Create(&db.P9PetReceipt{PlayerID: id, Operation: operation, Timestamp: stamp, Request: raw, Response: data}).Error
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}

func (s *GameService) p9PetShopList(ctx context.Context, id int64) ([]proto.Message, error) {
	var out *pb.SCPetShopProductList
	err := s.p3Profile(ctx, id, func(tx *gorm.DB, _ *db.Profile, p *pb.Player, _ *model.GameState) error {
		if err := s.requireShopAvailability(tx, "pet-market", LocalPlayNow(ctx)); err != nil {
			return err
		}
		if err := s.p9Open(p); err != nil {
			return err
		}
		if p.GetCoinpet() < 0 {
			return p5Error()
		}
		state, cfg, err := s.p9MarketState(tx, id, time.Now())
		if err != nil {
			return err
		}
		out, err = s.p9MarketListing(state, cfg)
		return err
	})
	return []proto.Message{out}, err
}

func (s *GameService) p9PetShopBuy(ctx context.Context, id int64, request *pb.CSPetShopBuy, stamp int64) ([]proto.Message, error) {
	out := &pb.SCPetShopBuy{Get: &pb.ResultObject{}}
	messages, err := s.p9MarketMutation(ctx, id, constUtil.P9MarketBuyOperation, stamp, request, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		if request.GetBuyCount() < 1 || request.GetBuyCount() > 100 || len(request.GetShopItemId()) > 128 || strings.TrimSpace(request.GetShopItemId()) != request.GetShopItemId() {
			return p5Error()
		}
		state, cfg, err := s.p9MarketState(tx, id, time.Now())
		if err != nil {
			return err
		}
		selected, bought, err := s.p9MarketInventory(state, cfg)
		if err != nil {
			return err
		}
		var chosen *p9MarketOffer
		for i, offer := range selected {
			if p9MarketQuoteSKU(state, offer) == request.GetShopItemId() {
				chosen = &selected[i]
				break
			}
		}
		if chosen == nil || bought[chosen.ID] > chosen.Stock-request.GetBuyCount() {
			return errors.New(constUtil.P9MarketReceiptExpired)
		}
		price := int64(chosen.Price) * int64(request.GetBuyCount())
		amount := int64(chosen.Amount) * int64(request.GetBuyCount())
		if price > math.MaxInt32 || amount > math.MaxInt32 {
			return p5Error()
		}
		cost, err := s.p4ParseCost(fmt.Sprintf("%s,%d", chosen.Currency, price))
		if err != nil {
			return err
		}
		if err = s.p4Deduct(tx, id, p, g, cost, "p9:shop-buy:"+strconv.FormatInt(stamp, 10), LocalPlayNow(ctx)); err != nil {
			return err
		}
		gain, err := s.GrantFixedReward(p, g, chosen.Resource+","+strconv.FormatInt(amount, 10))
		if err != nil {
			return err
		}
		out.Get = gain
		out.Cost = []*pb.BaseResource{{Id: proto.String(chosen.Currency), Num: proto.Int32(int32(price))}}
		bought[chosen.ID] += request.GetBuyCount()
		state.Bought, err = json.Marshal(bought)
		if err != nil {
			return err
		}
		if err := tx.Save(state).Error; err != nil {
			return err
		}
		listing, err := s.p9MarketListing(state, cfg)
		if err != nil {
			return err
		}
		out.ShopItemList = listing.ShopItemList

		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.shopInventorySync(ctx, id, messages)
}

// p9PetShopRefresh 发送原回调实际读取的ProductList，并追加完整钱包同步扣费。
func (s *GameService) p9PetShopRefresh(ctx context.Context, id int64, request *pb.CSRefreshPetShop, stamp int64) ([]proto.Message, error) {
	out := &pb.SCPetShopProductList{}
	messages, err := s.p9MarketMutation(ctx, id, constUtil.P9MarketRefreshOperation, stamp, request, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		state, cfg, err := s.p9MarketState(tx, id, time.Now())
		if err != nil {
			return err
		}
		if state.RefreshUsed >= cfg.RefreshCap {
			return errors.New("本周精灵商店刷新次数已用完")
		}
		cost, err := parsePoolCost(cfg.RefreshCost)
		if err != nil {
			return err
		}
		if err := deductPoolCost(p, g, cost); err != nil {
			return err
		}
		if err := s.p9MarketSelect(state, cfg); err != nil {
			return err
		}
		state.RefreshUsed++
		if err := tx.Save(state).Error; err != nil {
			return err
		}
		listing, err := s.p9MarketListing(state, cfg)
		if err != nil {
			return err
		}
		proto.Merge(out, listing)
		if s.P1Ready {
			return s.RecordP1Cost(tx, id, "p9:shop-refresh:"+strconv.FormatInt(stamp, 10), []*pb.BaseResource{cost}, LocalPlayNow(ctx))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	var row db.Profile
	if err := db.GetIns().WithContext(ctx).First(&row, id).Error; err != nil {
		return nil, err
	}
	p, _, err := decodeProfile(&row)
	if err != nil {
		return nil, err
	}
	return append(messages, &pb.SCPlayer{Player: p}), nil
}
