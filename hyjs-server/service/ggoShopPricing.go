package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"math"
	"strconv"
	"strings"
)

func ggoRaisedPrice(base int64, cfg model.AdminFieldGGOShop) (int64, error) {
	if base <= 0 || cfg.PriceNumerator < cfg.PriceDenominator || cfg.PriceDenominator <= 0 || cfg.RoundTo <= 0 || base > math.MaxInt64/cfg.PriceNumerator {
		return 0, errors.New("信用商店涨价参数无效")
	}
	value := base * cfg.PriceNumerator
	value = value/cfg.PriceDenominator + min(int64(1), value%cfg.PriceDenominator)
	if value > math.MaxInt64-(cfg.RoundTo-1) {
		return 0, errors.New("信用商店售价溢出")
	}
	value = ((value + cfg.RoundTo - 1) / cfg.RoundTo) * cfg.RoundTo
	if value > math.MaxInt32 {
		return 0, errors.New("信用商店单份售价超过原协议上限")
	}
	return value, nil
}

// Only newly drawn quotes consume the published multiplier. Persisted quotes
// carry their accepted price and refresh cost until they are replaced.
func (s *GameService) ggoDrawRuntimeQuote(tx *gorm.DB, rules p4CommerceRules, state *ggoQuoteState) error {
	cfg := s.GGOFieldConfig
	if err := ggoDrawQuoteSlots(rules, state); err != nil {
		return err
	}
	state.RefreshCost = rules.RefreshCost
	state.DayDrawn = state.Day
	if cfg == nil || cfg.Runtime == nil {
		return nil
	}
	pricing := cfg.Runtime.Shop
	for i := range state.Slots {
		p := &state.Slots[i].Product
		pair := strings.Split(p.Cost, ",")
		if len(pair) != 2 || pair[0] != constUtil.GGOCurrencyID {
			return errors.New("信用商店费用格式无效")
		}
		n, err := strconv.ParseInt(pair[1], 10, 32)
		if err != nil || n <= 0 {
			return errors.New("信用商店费用无效")
		}
		n = (n*int64(p.DiscountBps) + 9999) / 10000
		price, err := ggoRaisedPrice(n, pricing)
		if err != nil {
			return err
		}
		p.Cost = fmt.Sprintf("%s,%d", constUtil.GGOCurrencyID, price)
		p.DiscountBps = 10000
	}
	state.RefreshCost = fmt.Sprintf("%s,%d", constUtil.GGOCurrencyID, pricing.RefreshCost)
	return nil
}

// Validate both base offers and all possible quantity-tier prices before a new
// desert economy is published. Each accepted quote remains independently frozen.
func (s *GameService) validateGGOShopPricingChange(tx *gorm.DB, previous, next *model.AdminFieldGGOConfiguration) error {
	if previous != nil && previous.Runtime != nil && (next == nil || next.Runtime == nil) {
		return errors.New("已发布沙漠经济不能删除，请保留价格配置")
	}
	if next == nil || next.Runtime == nil {
		return nil
	}
	before := model.AdminFieldGGOShop{PriceNumerator: 1, PriceDenominator: 1, RoundTo: 1, RefreshCost: 10}
	if previous != nil && previous.Runtime != nil {
		before = previous.Runtime.Shop
	}
	after := next.Runtime.Shop
	if after.RefreshCost < before.RefreshCost {
		return errors.New("信用商店刷新费用只能上调")
	}
	_, raw, err := s.adminReadShop(tx, constUtil.GGOShopKey)
	if err != nil {
		return err
	}
	if raw.Period == nil {
		return errors.New("请先配置信用商店")
	}
	var rules p4CommerceRules
	if err = json.Unmarshal(raw.Period.Rules, &rules); err != nil {
		return err
	}
	check := func(amount int64, discount int32) error {
		if amount <= 0 || discount <= 0 {
			return errors.New("信用商店价格无效")
		}
		base := (amount*int64(discount) + 9999) / 10000
		a, e := ggoRaisedPrice(base, before)
		if e != nil {
			return e
		}
		b, e := ggoRaisedPrice(base, after)
		if e != nil {
			return e
		}
		if b < a {
			return errors.New("信用商店商品售价只能上调")
		}
		return nil
	}
	for _, p := range rules.Products {
		cost, e := s.p4ParseCost(p.Cost)
		if e != nil {
			return e
		}
		if cost.GetId() != constUtil.GGOCurrencyID {
			return errors.New("信用商店费用类型无效")
		}
		if e = check(int64(cost.GetNum()), p.DiscountBps); e != nil {
			return e
		}
		for _, tier := range p.QuantityTiers {
			if e = check(tier.Cost.Amount, p.DiscountBps); e != nil {
				return e
			}
		}
	}
	return nil
}

// Resolve committed pricing before taking the profile lock. This avoids a second
// database connection inside single-connection compatibility transactions.
func (s *GameService) ggoShopPricingService(ctx context.Context) (*GameService, error) {
	snapshot, err := s.adminFieldRuntimeSnapshotForRequest(ctx, s.OriginalServerID())
	if err != nil {
		return nil, err
	}
	runtime := s.withRuntimeTables(s.Tables)
	runtime.GGOFieldConfig = snapshot.ggoEconomy
	return runtime, nil
}
