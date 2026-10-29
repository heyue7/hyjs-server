package service

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

type trialRefresh82Offer struct {
	ID      string    `json:"id"`
	Source  string    `json:"source"`
	Product p4Product `json:"product"`
}

// This role-wide record is independent of the historical directory and lifetime records.
type trialRefresh82State struct {
	RuleID       string                `json:"ruleId"`
	Generation   int64                 `json:"generation"`
	Offers       []trialRefresh82Offer `json:"offers"`
	Bought       map[string]int32      `json:"bought"`
	Day          string                `json:"day"`
	ResourceUsed int32                 `json:"resourceUsed"`
	FreeLeft     int32                 `json:"freeLeft"`
	FreeAt       int64                 `json:"freeAt"` // wall time, unaffected by a game calendar jump
}

func trialRefresh82Enabled(cfg db.P4CommerceRule, rules p4CommerceRules) bool {
	return cfg.Kind == "point" && cfg.ShopType == 5 && rules.TrialRefreshVersion >= 1
}

func (s *GameService) trialRefresh82Priority(priority int32, rewards []*pb.BaseResource) int32 {
	if len(rewards) > 0 {
		switch rewards[0].GetId() {
		case "frag_equip_037", "frag_equip_038", "frag_equip_039", "frag_equip_040":
			order := priority % 1000
			if order < 0 {
				order = 0
			}
			return 1000 + order
		}
	}
	return s.pointShop48Priority(priority, rewards)
}

func (s *GameService) trialRefresh82Group(p p4Product) (int32, error) {
	rewards, err := p1RewardResources(p.Rewards)
	if err != nil {
		return 0, err
	}
	return s.trialRefresh82Priority(p.Priority, rewards) / 1000, nil
}

func (s *GameService) trialRefresh82ValidateTiers(p p4Product) error {
	if len(p.QuantityTiers) > 64 {
		return errors.New("试炼商品数量档过多")
	}
	var total int64
	amounts := map[int64]bool{}
	for _, tier := range p.QuantityTiers {
		if tier.Amount < 1 || tier.Amount > math.MaxInt32 || tier.Weight < 1 || tier.Weight > math.MaxInt64-total || amounts[tier.Amount] || tier.Cost.Amount < 1 || tier.Cost.Amount > math.MaxInt32 {
			return errors.New("试炼商品数量档无效")
		}
		if _, err := s.p4ParseCost(shopResourceSpec(tier.Cost)); err != nil {
			return err
		}
		total += tier.Weight
		amounts[tier.Amount] = true
	}
	return nil
}

func trialRefresh82OfferID(rule string, generation int64, position int, source string) string {
	return fmt.Sprintf("tr82:%s:%d:%d:%s", p6Digest([]byte(rule))[:10], generation, position, p6Digest([]byte(source))[:10])
}

// Each slot is drawn independently. The source and quoted resources are frozen
// together, while the current source still controls eligibility and availability.
func (s *GameService) trialRefresh82Draw(cfg db.P4CommerceRule, rules p4CommerceRules, state *trialRefresh82State) error {
	if state.Generation < 0 || state.Generation == math.MaxInt64 {
		return errors.New("试炼商店代次无效")
	}
	var pool []p4Product
	var weight int64
	for _, p := range rules.Products {
		group, err := s.trialRefresh82Group(p)
		if err != nil {
			return err
		}
		if group != 1 || !p.isEnabled() {
			continue
		}
		if p.Weight < 1 || p.Weight > math.MaxInt64-weight {
			return errors.New("试炼商店商品权重无效")
		}
		weight += p.Weight
		pool = append(pool, p)
	}
	if len(pool) == 0 {
		return errors.New("试炼商店尚无可刷新商品")
	}
	generation := state.Generation + 1
	offers := make([]trialRefresh82Offer, 0, constUtil.TrialRefresh82Slots)
	for pos := 1; pos <= constUtil.TrialRefresh82Slots; pos++ {
		draw, err := rand.Int(rand.Reader, big.NewInt(weight))
		if err != nil {
			return err
		}
		value := draw.Int64()
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
		if quote.TierID {
			price, err := s.p4TierCost(chosen.ID, 1, 1)
			if err != nil {
				return err
			}
			quote.Cost = fmt.Sprintf("%s,%d", price.GetId(), price.GetNum())
			quote.TierID = false
		}
		id := trialRefresh82OfferID(cfg.ID, generation, pos, chosen.ID)
		quote.ID = id
		quote.Index = int32(pos)
		offers = append(offers, trialRefresh82Offer{ID: id, Source: chosen.ID, Product: quote})
	}
	state.RuleID = cfg.ID
	state.Generation = generation
	state.Offers = offers
	state.Bought = map[string]int32{}
	return nil
}

func trialRefresh82RealTime(tx *gorm.DB, now time.Time) time.Time {
	if GameCalendarContextBound(tx.Statement.Context) {
		return now.AddDate(0, 0, -int(GameCalendarOffsetDays(tx.Statement.Context)))
	}
	return now
}

// Archive only the daily paid quota. Slot ownership and free cooldown persist
// across dates and rule windows; switching back to a date restores its quota.
func trialRefresh82Day(tx *gorm.DB, id int64, state *trialRefresh82State, now time.Time) error {
	day := p1Day(now)
	if _, err := time.Parse("2006-01-02", state.Day); err != nil {
		return errors.New("试炼商店日期存档无效")
	}
	if state.Day == day {
		return nil
	}
	if err := calendarQuota75Save(tx, id, constUtil.TrialRefresh82Quota, state.Day, state.ResourceUsed); err != nil {
		return err
	}
	var used int32
	if _, err := calendarQuota75Load(tx, id, constUtil.TrialRefresh82Quota, day, &used); err != nil {
		return err
	}
	if used < 0 {
		return errors.New("试炼商店刷新额度存档无效")
	}
	state.Day = day
	state.ResourceUsed = used
	return nil
}

func trialRefresh82Save(tx *gorm.DB, row *db.P4CommerceState, state *trialRefresh82State) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if len(row.Payload) > 0 && bytes.Equal(row.Payload, raw) {
		return nil
	}
	row.Payload = raw
	return tx.Save(row).Error
}

// The profile row lock held by the caller serializes loads, buys and refreshes.
func (s *GameService) trialRefresh82Load(tx *gorm.DB, id int64, cfg db.P4CommerceRule, rules p4CommerceRules, now time.Time) (*db.P4CommerceState, *trialRefresh82State, error) {
	row := &db.P4CommerceState{PlayerID: id, RuleID: constUtil.TrialRefresh82StateID}
	err := tx.First(row, "player_id=? AND rule_id=?", id, row.RuleID).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, err
	}
	realNow := trialRefresh82RealTime(tx, now).UnixMilli()
	state := &trialRefresh82State{}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		state.Day = p1Day(now)
		state.FreeAt = realNow
		state.FreeLeft = min(int32(1), rules.FreeCap)
		if err = s.trialRefresh82Draw(cfg, rules, state); err != nil {
			return nil, nil, err
		}
	} else {
		if len(row.Payload) > constUtil.CalendarQuota75MaxBytes || json.Unmarshal(row.Payload, state) != nil || state.Generation < 1 || state.RuleID == "" || len(state.Offers) != constUtil.TrialRefresh82Slots || state.Bought == nil || state.ResourceUsed < 0 || state.FreeLeft < 0 || state.FreeAt <= 0 {
			return nil, nil, errors.New("试炼商店刷新存档无效")
		}
		seen := map[string]bool{}
		for i, offer := range state.Offers {
			if offer.Source == "" || offer.ID != trialRefresh82OfferID(state.RuleID, state.Generation, i+1, offer.Source) || offer.Product.ID != offer.ID || seen[offer.ID] {
				return nil, nil, errors.New("试炼商店商品位存档无效")
			}
			seen[offer.ID] = true
		}
		for key, count := range state.Bought {
			if !seen[key] || count < 0 || count > 1 {
				return nil, nil, errors.New("试炼商店购买存档无效")
			}
		}
		if err = trialRefresh82Day(tx, id, state, now); err != nil {
			return nil, nil, err
		}
		if state.RuleID != cfg.ID {
			if err = s.trialRefresh82Draw(cfg, rules, state); err != nil {
				return nil, nil, err
			}
		}
	}
	state.FreeLeft = min(state.FreeLeft, rules.FreeCap)
	if state.FreeAt > realNow {
		// A wall-clock correction never grants free uses or erases an existing cooldown.
		realNow = state.FreeAt
	}
	if rules.FreeCap > 0 && rules.FreeSeconds > 0 {
		interval := rules.FreeSeconds * 1000
		elapsed := (realNow - state.FreeAt) / interval
		if elapsed > 0 {
			state.FreeLeft = int32(min(int64(rules.FreeCap), int64(state.FreeLeft)+elapsed))
			state.FreeAt += elapsed * interval
		}
	}
	return row, state, nil
}

func (s *GameService) trialRefresh82Products(rules p4CommerceRules, state *trialRefresh82State) ([]p4Product, error) {
	live := make(map[string]p4Product, len(rules.Products))
	var products []p4Product
	for _, p := range rules.Products {
		group, err := s.trialRefresh82Group(p)
		if err != nil {
			return nil, err
		}
		if group == 1 {
			live[p.ID] = p
		} else {
			products = append(products, p)
		}
	}
	for _, offer := range state.Offers {
		source, ok := live[offer.Source]
		if !ok || !source.isEnabled() {
			continue
		}
		p := offer.Product
		p.SourceProductID = offer.Source
		p.Enabled, p.MinLevel, p.MinVIP, p.MinTrialStars, p.Stock = source.Enabled, source.MinLevel, source.MinVIP, source.MinTrialStars, source.Stock
		p.MinLevel = max(p.MinLevel, int32(constUtil.TrialRefresh82MinLevel))
		p.Priority = 1000 + p.Index
		p.DailyLimit, p.TotalLimit, p.AccountWeeklyLimit = 1, 0, 0
		products = append(products, p)
	}
	return products, nil
}

func trialRefresh82Slot(state *trialRefresh82State, id string) bool {
	if state == nil {
		return false
	}
	for _, offer := range state.Offers {
		if offer.ID == id {
			return true
		}
	}
	return false
}

func trialRefresh82Next(tx *gorm.DB, rules p4CommerceRules, state *trialRefresh82State) int64 {
	if state.FreeLeft >= rules.FreeCap || rules.FreeCap <= 0 {
		return 0
	}
	next := state.FreeAt + rules.FreeSeconds*1000
	if GameCalendarContextBound(tx.Statement.Context) {
		next += GameCalendarOffsetDays(tx.Statement.Context) * 86400000
	}
	return next
}
