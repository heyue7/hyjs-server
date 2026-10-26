package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

type ggoQuoteSlot struct {
	ID              string    `json:"id"`
	SourceProductID string    `json:"sourceProductId"`
	Product         p4Product `json:"product"`
}
type ggoQuoteState struct {
	Generation int64            `json:"generation"`
	Day        string           `json:"day"`
	FreeLeft   int64            `json:"freeLeft"`
	FreeAt     int64            `json:"freeAt"`
	PaidUsed   int64            `json:"paidUsed"`
	Slots      []ggoQuoteSlot   `json:"slots"`
	SlotBought map[string]int64 `json:"slotBought"`
}

func ggoQuoteRuleID(ruleID string) string { return "ggoq:" + p6Digest([]byte(ruleID))[:40] }
func ggoSourceProductID(p p4Product) string {
	if p.SourceProductID != "" {
		return p.SourceProductID
	}
	return p.ID
}
func ggoHasRandomQuotes(rules p4CommerceRules) bool {
	if shopRefreshEnabled(rules.RefreshEnabled, false) {
		return true
	}
	for _, p := range rules.Products {
		if len(p.QuantityTiers) > 0 {
			return true
		}
	}
	return false
}
func ggoDrawQuoteSlots(rules p4CommerceRules, state *ggoQuoteState) error {
	if state.Generation < 0 || state.Generation == math.MaxInt64 {
		return errors.New("信用商店代次溢出")
	}
	var pool []p4Product
	var weight int64
	for _, p := range rules.Products {
		if !p.isEnabled() {
			continue
		}
		if p.Weight < 1 || p.Weight > math.MaxInt64-weight {
			return errors.New("信用商店商品权重无效或溢出")
		}
		weight += p.Weight
		pool = append(pool, p)
	}
	if len(pool) == 0 {
		return errors.New("信用商店尚无可刷新商品")
	}
	generation := state.Generation + 1
	slots := make([]ggoQuoteSlot, 0, 6)
	for pos := 0; pos < 6; pos++ {
		roll, err := rand.Int(rand.Reader, big.NewInt(weight))
		if err != nil {
			return err
		}
		value := roll.Int64()
		var chosen p4Product
		for _, p := range pool {
			if value < p.Weight {
				chosen = p
				break
			}
			value -= p.Weight
		}
		quote, err := memoryDrawQuote63(chosen)
		if err != nil {
			return err
		}
		id := fmt.Sprintf("ggoq:%d:%d", generation, pos+1)
		quote.SourceProductID = chosen.ID
		quote.ID = id
		quote.Index = int32(pos + 1)
		slots = append(slots, ggoQuoteSlot{ID: id, SourceProductID: chosen.ID, Product: quote})
	}
	state.Generation = generation
	state.Slots = slots
	state.SlotBought = map[string]int64{}
	return nil
}
func ggoSaveQuote(tx *gorm.DB, row *db.P4CommerceState, state *ggoQuoteState) error {
	row.Payload = adminJSON(state)
	return tx.Save(row).Error
}

// ggoLoadQuote retains the accepted generation after quantity tiers are removed.
// The caller holds the profile UPDATE lock, shared by buys and refreshes.
func (s *GameService) ggoLoadQuote(tx *gorm.DB, playerID int64, cfg db.P4CommerceRule, rules p4CommerceRules, allowCreate bool) (*db.P4CommerceState, *ggoQuoteState, error) {
	requested := ggoHasRandomQuotes(rules)
	if !requested && !s.P4Ready {
		return nil, nil, nil
	}
	row := &db.P4CommerceState{PlayerID: playerID, RuleID: ggoQuoteRuleID(cfg.ID)}
	err := tx.First(row, "player_id=? AND rule_id=?", playerID, row.RuleID).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) && (!requested || !allowCreate) {
		return nil, nil, nil
	}
	now := LocalPlayNow(tx.Statement.Context)
	day := now.Format("2006-01-02")
	state := &ggoQuoteState{}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		state.Day = day
		state.FreeLeft = int64(rules.FreeCap)
		state.FreeAt = now.UnixMilli()
		if err := ggoDrawQuoteSlots(rules, state); err != nil {
			return nil, nil, err
		}
	} else {
		if len(row.Payload) > 262144 || json.Unmarshal(row.Payload, state) != nil || state.Generation < 1 || len(state.Slots) != 6 || state.SlotBought == nil || state.FreeLeft < 0 || state.PaidUsed < 0 || state.Day > day || state.FreeAt <= 0 || state.FreeAt > now.UnixMilli() {
			return nil, nil, errors.New("信用商店报价存档无效")
		}
		seen := map[string]bool{}
		for pos, slot := range state.Slots {
			if slot.ID != fmt.Sprintf("ggoq:%d:%d", state.Generation, pos+1) || len(slot.SourceProductID)+len(constUtil.GGOShopStatePrefix) > 64 || slot.ID == "" || len(slot.ID) > 64 || slot.ID != slot.Product.ID || slot.SourceProductID == "" || seen[slot.ID] || (state.SlotBought[slot.ID] < 0 || state.SlotBought[slot.ID] > 1) {
				return nil, nil, errors.New("信用商店槽位存档无效")
			}
			seen[slot.ID] = true
		}
		for id, n := range state.SlotBought {
			if !seen[id] || n < 0 || n > 1 {
				return nil, nil, errors.New("信用商店购买存档无效")
			}
		}
		if state.Day != day {
			state.Day = day
			state.PaidUsed = 0
		}
		state.FreeLeft = min(state.FreeLeft, int64(rules.FreeCap))
		if rules.FreeCap > 0 && rules.FreeSeconds > 0 {
			seconds := rules.FreeSeconds * 1000
			elapsed := (now.UnixMilli() - state.FreeAt) / seconds
			if elapsed > 0 {
				state.FreeLeft = min(int64(rules.FreeCap), state.FreeLeft+elapsed)
				state.FreeAt += elapsed * seconds
			}
		}
	}
	if err := ggoSaveQuote(tx, row, state); err != nil {
		return nil, nil, err
	}
	return row, state, nil
}
func ggoQuoteProducts(rules p4CommerceRules, state *ggoQuoteState) []p4Product {
	if state == nil {
		return rules.Products
	}
	live := map[string]p4Product{}
	for _, p := range rules.Products {
		live[p.ID] = p
	}
	products := make([]p4Product, 0, len(state.Slots))
	for _, slot := range state.Slots {
		source, exists := live[slot.SourceProductID]
		if !exists || !source.isEnabled() {
			continue
		}
		p := slot.Product
		p.SourceProductID = slot.SourceProductID
		p.Enabled = source.Enabled
		p.MinLevel = source.MinLevel
		p.MinVIP = source.MinVIP
		p.DailyLimit = source.DailyLimit
		p.TotalLimit = source.TotalLimit
		p.AccountWeeklyLimit = source.AccountWeeklyLimit
		p.Stock = source.Stock
		products = append(products, p)
	}
	return products
}
func ggoQuoteVersion(cfg db.P4CommerceRule, rules p4CommerceRules, state *ggoQuoteState, enabled bool) string {
	type eligibility struct {
		ID                                              string
		AccountWeeklyLimit                              int64
		Enabled                                         bool
		DailyLimit, TotalLimit, Stock, MinLevel, MinVIP int32
	}
	live := make([]eligibility, 0, len(rules.Products))
	for _, p := range rules.Products {
		live = append(live, eligibility{p.ID, p.AccountWeeklyLimit, p.isEnabled(), p.DailyLimit, p.TotalLimit, p.Stock, p.MinLevel, p.MinVIP})
	}
	sort.Slice(live, func(i, j int) bool { return live[i].ID < live[j].ID })
	return adminVersion(struct {
		Rule       string
		Generation int64
		Slots      []ggoQuoteSlot
		Live       []eligibility
		Enabled    bool
		Refresh    *model.AdminShopRefreshSettings
	}{cfg.ID, state.Generation, state.Slots, live, enabled, shopP4RefreshSettings("ggo", rules)})
}
func ggoQuoteRefreshInfo(rules p4CommerceRules, state *ggoQuoteState, enabled bool) *model.GGOShopRefreshInfo {
	cost := adminTypedResourceOne(rules.RefreshCost)
	out := &model.GGOShopRefreshInfo{Enabled: enabled && shopRefreshEnabled(rules.RefreshEnabled, false), FreeLeft: state.FreeLeft, FreeCap: int64(rules.FreeCap), PaidLeft: max(int64(0), int64(rules.ResourceRefreshCap)-state.PaidUsed), PaidCap: int64(rules.ResourceRefreshCap), CostID: cost.ResourceID, CostAmount: cost.Amount, Generation: state.Generation}
	if rules.FreeCap > 0 && rules.FreeSeconds > 0 && state.FreeLeft < int64(rules.FreeCap) {
		out.NextFreeAt = state.FreeAt + rules.FreeSeconds*1000
	}
	return out
}

// GGOShopRefresh uses the same profile lock and UUID namespace as purchases.
func (s *GameService) GGOShopRefresh(ctx context.Context, req model.GGOShopRefreshRequest, session []byte) (*model.GGOShopRefreshResponse, error) {
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, s)
	authorized, err := AuthorizeAccount(ctx, req.PlayerID, session)
	if err != nil {
		return nil, err
	}
	if AdminProfileServerID(*authorized) != s.OriginalServerID() {
		return nil, constUtil.ErrAdminPermission
	}
	ctx, err = s.BindLocalPlayContext(ctx, authorized)
	if err != nil {
		return nil, err
	}
	var out model.GGOShopRefreshResponse
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var profile db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, req.PlayerID).Error; err != nil {
			return err
		}
		if AdminProfileServerID(profile) != s.OriginalServerID() || !bytes.Equal(profile.Session, session) {
			return errors.New("本地会话无效，请重新登录")
		}
		var receipt db.GGOShopReceipt
		err := tx.First(&receipt, "player_id=? AND request_id=?", req.PlayerID, req.RequestID).Error
		if err == nil {
			if receipt.ProductID != constUtil.GGOShopRefreshReceiptProduct || receipt.Count != 0 || receipt.Version != req.Version {
				return errors.New("请求编号已用于其他操作或版本")
			}
			return json.Unmarshal(receipt.Response, &out)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		player, game, err := decodeProfileForCalendarUpdate(tx, &profile)
		if err != nil {
			return err
		}
		catalogue, err := s.ggoCatalogue(tx, player, game)
		if err != nil {
			return err
		}
		if catalogue.Version != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		if !catalogue.Enabled || catalogue.Refresh == nil || !catalogue.Refresh.Enabled {
			return errors.New("信用商店刷新未开放")
		}
		_, raw, err := s.adminReadShop(tx, constUtil.GGOShopKey)
		if err != nil {
			return err
		}
		if raw.Period == nil {
			return constUtil.ErrAdminConfiguration
		}
		rules, err := s.validateP4Commerce(*raw.Period)
		if err != nil {
			return err
		}
		if err = s.validateShopRefreshSettings("ggo", shopP4RefreshSettings("ggo", rules)); err != nil {
			return err
		}
		row, state, err := s.ggoLoadQuote(tx, req.PlayerID, *raw.Period, rules, true)
		if err != nil {
			return err
		}
		if state == nil {
			return errors.New("信用商店刷新未配置")
		}
		now := LocalPlayNow(ctx)
		if state.FreeLeft > 0 {
			state.FreeLeft--
		} else {
			if state.PaidUsed >= int64(rules.ResourceRefreshCap) {
				return errors.New("今日信用商店刷新次数已用完")
			}
			cost, err := s.p4ParseCost(rules.RefreshCost)
			if err != nil || !ggoShopCostAllowed("ggo", cost.GetId()) {
				return errors.New("信用商店刷新消耗未配置")
			}
			if err = s.p4Deduct(tx, req.PlayerID, player, game, cost, "ggo-shop-refresh:"+req.RequestID, now); err != nil {
				return err
			}
			out.CostID = cost.GetId()
			out.CostAmount = int64(cost.GetNum())
			state.PaidUsed++
		}
		if err = ggoDrawQuoteSlots(rules, state); err != nil {
			return err
		}
		if err = ggoSaveQuote(tx, row, state); err != nil {
			return err
		}
		if err = saveProfile(tx, &profile, player, game); err != nil {
			return err
		}
		out.Catalogue, err = s.ggoCatalogue(tx, player, game)
		if err != nil {
			return err
		}
		out.RequestID = req.RequestID
		// Map insertion preserves explicit zero despite the legacy Count default of one.
		return tx.Model(&db.GGOShopReceipt{}).Create(map[string]any{"player_id": req.PlayerID, "request_id": req.RequestID, "product_id": constUtil.GGOShopRefreshReceiptProduct, "version": req.Version, "count": int64(0), "response": adminJSON(out), "created_at": now.UnixMilli()}).Error
	})
	return &out, err
}
