package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

type memoryOffer55 struct {
	ID        string     `json:"id"`
	ProductID string     `json:"productId"`
	Quote     *p4Product `json:"quote,omitempty"`
}

func memorySource55(p p4Product) string {
	if p.SourceProductID != "" {
		return p.SourceProductID
	}
	return p.ID
}
func memoryProducts55(cfg db.P4CommerceRule, rules p4CommerceRules, state *p4CommerceProgress) []p4Product {
	if cfg.Kind == "memory" && len(state.Offers) == 0 && len(state.Slots) > 0 {
		for _, id := range state.Slots {
			state.Offers = append(state.Offers, memoryOffer55{ID: id, ProductID: id})
		}
		state.Slots = nil
	}
	if cfg.Kind != "memory" || len(state.Offers) == 0 {
		return rules.Products
	}
	var out []p4Product
	for i := range state.Offers {
		slot := &state.Offers[i]
		for _, p := range rules.Products {
			if p.ID == slot.ProductID {
				if slot.Quote == nil {
					q := p
					q.QuantityTiers = nil
					slot.Quote = &q
				}
				p.Rewards = slot.Quote.Rewards
				p.Cost = slot.Quote.Cost
				p.DiscountBps = slot.Quote.DiscountBps
				p.TierID = slot.Quote.TierID
				p.QuantityTiers = nil
				p.SourceProductID = p.ID
				p.ID = slot.ID
				p.Index = int32(len(out) + 1)
				out = append(out, p)
				break
			}
		}
	}
	return out
}
func memoryRefresh55(rules p4CommerceRules, state *p4CommerceProgress) error {
	if rules.MemoryPolicyVersion < 1 {
		slots, err := p4SampleSlots(rules)
		if err != nil {
			return err
		}
		offers := make([]memoryOffer55, 0, len(slots))
		for _, id := range slots {
			for _, p := range rules.Products {
				if p.ID != id {
					continue
				}
				q, err := memoryDrawQuote63(p)
				if err != nil {
					return err
				}
				offers = append(offers, memoryOffer55{ID: id, ProductID: id, Quote: &q})
				break
			}
		}
		state.Slots = nil
		state.Offers = offers
		return nil
	}
	var fragments, materials []p4Product
	for _, p := range rules.Products {
		if !p.isEnabled() || p.Weight <= 0 {
			continue
		}
		if strings.HasPrefix(p.Rewards, "frag_") {
			fragments = append(fragments, p)
		} else {
			materials = append(materials, p)
		}
	}
	n, err := rand.Int(rand.Reader, big.NewInt(constUtil.MemoryShrine55FragmentChoices))
	if err != nil {
		return err
	}
	count := int(n.Int64()) + 1
	if len(fragments) < count || len(materials) == 0 {
		return errors.New("圣坛商品池不足")
	}
	var offers []memoryOffer55
	for pos := 0; pos < constUtil.MemoryShrine55Slots; pos++ {
		pool := materials
		if pos < count {
			pool = fragments
		}
		sample, err := p4SampleSlots(p4CommerceRules{Products: pool, Slots: 1})
		if err != nil {
			return err
		}
		if len(sample) != 1 {
			return errors.New("圣坛商品池不足")
		}
		id := sample[0]
		var quote p4Product
		for _, p := range pool {
			if p.ID == id {
				quote = p
				break
			}
		}
		quote, err = memoryDrawQuote63(quote)
		if err != nil {
			return err
		}
		offers = append(offers, memoryOffer55{ID: fmt.Sprintf("memory55:%d:%d:%s", state.Version, pos+1, id), ProductID: id, Quote: &quote})
		if pos < count {
			for i, p := range fragments {
				if p.ID == id {
					fragments = append(fragments[:i], fragments[i+1:]...)
					break
				}
			}
		}
	}
	state.Slots = nil
	state.Offers = offers
	return nil
}
func (s *GameService) memoryCatalogue55(rules p4CommerceRules) p4CommerceRules {
	if rules.MemoryPolicyVersion >= constUtil.MemoryShrine55PolicyVersion {
		return rules
	}
	allowed := map[string]int64{}
	heroes := map[string]int64{}
	for _, h := range s.Tables["Hero"] {
		if h["heroClass"] != "0" {
			continue
		}
		switch h["quality"] {
		case "18":
			heroes[h["id"]] = 100
		case "20":
			heroes[h["id"]] = 25
		}
		if h["id"] == "hero_001" || h["id"] == "hero_003" || h["id"] == "hero_005" {
			heroes[h["id"]] = 10
		}
	}
	for _, f := range s.Tables["Frag"] {
		if w := heroes[f["target"]]; w > 0 {
			allowed[f["id"]] = w
		}
	}
	materials := map[string]int64{"item_005": 40, "hero_exp_002": 30, "item_006": 20, "item_013": 10}
	seen := map[string]bool{}
	out := []p4Product{}
	maxIndex := int32(0)
	for _, p := range rules.Products {
		reward := strings.Split(p.Rewards, ",")[0]
		weight := allowed[reward]
		if strings.HasPrefix(reward, "frag_") && weight == 0 {
			continue
		}
		if weight == 0 {
			weight = materials[reward]
		}
		if p.Weight == 1 && weight > 0 {
			p.Weight = weight
		}
		seen[reward] = true
		out = append(out, p)
		if p.Index > maxIndex {
			maxIndex = p.Index
		}
	}
	for _, id := range []string{"item_005", "hero_exp_002", "item_006", "item_013"} {
		if seen[id] {
			continue
		}
		maxIndex++
		reward, cost := id+",1", "light,30"
		if id == "item_005" {
			reward = id + ",5"
			cost = "light,15"
		}
		if id == "item_013" {
			cost = "light,10"
		}
		p := p4BaseProduct("memory55_"+id, reward, cost, maxIndex, 16)
		p.DailyLimit = 1
		p.Weight = materials[id]
		out = append(out, p)
	}
	ids := []string{}
	for id := range allowed {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if seen[id] {
			continue
		}
		maxIndex++
		p := p4BaseProduct("memory55_"+id, id+",1", "light,20", maxIndex, 16)
		p.DailyLimit = 1
		p.Weight = allowed[id]
		out = append(out, p)
	}
	rules.Products = out
	rules.Slots = constUtil.MemoryShrine55Slots
	rules.MemoryPolicyVersion = constUtil.MemoryShrine55PolicyVersion
	return rules
}

// InitializeMemoryShrine55 upgrades only current live catalogues; frozen history remains intact.
func (s *GameService) InitializeMemoryShrine55(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []db.P4CommerceRule
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("kind = ?", "memory").Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			var rules p4CommerceRules
			if err := json.Unmarshal(row.Rules, &rules); err != nil {
				return err
			}
			if rules.MemoryPolicyVersion >= constUtil.MemoryShrine55PolicyVersion {
				continue
			}
			if len(row.Frozen) > 0 && !rules.LiveEditable {
				continue
			}
			rules = s.memoryCatalogue55(rules)
			raw, err := json.Marshal(rules)
			if err != nil {
				return err
			}
			updates := map[string]any{"rules": raw}
			if strings.HasPrefix(row.Source, "本地兼容服目录/限购/库存") {
				updates["source"] = "回忆圣坛常驻商品池：每次6个位置，碎片占1～4个，其余为材料。"
			}
			if err = tx.Model(&db.P4CommerceRule{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *GameService) validateMemoryShrine55(rules p4CommerceRules) error {
	if rules.MemoryPolicyVersion < 1 {
		return nil
	}
	if rules.Slots != 6 {
		return errors.New("圣坛固定六个商品位")
	}
	allowed := map[string]bool{}
	heroes := map[string]bool{}
	for _, h := range s.Tables["Hero"] {
		if h["heroClass"] == "0" && (h["quality"] == "18" || h["quality"] == "20" || h["id"] == "hero_001" || h["id"] == "hero_003" || h["id"] == "hero_005") {
			heroes[h["id"]] = true
		}
	}
	for _, f := range s.Tables["Frag"] {
		if heroes[f["target"]] {
			allowed[f["id"]] = true
		}
	}
	enabled := map[string]bool{}
	materials := 0
	for _, p := range rules.Products {
		parts := strings.Split(p.Rewards, ",")
		if len(parts) != 2 {
			return errors.New("圣坛每格仅允许单项商品")
		}
		id := parts[0]
		if allowed[id] {
			if p.isEnabled() && p.Weight > 0 {
				enabled[id] = true
			}
			continue
		}
		if id != "item_005" && id != "hero_exp_002" && id != "hero_exp_004" && id != "item_006" && id != "item_013" {
			return errors.New("圣坛商品不在允许目录")
		}
		if p.isEnabled() && p.Weight > 0 {
			materials++
		}
	}
	if len(enabled) < 4 || materials < 1 {
		return errors.New("圣坛需要至少四种启用碎片和一种材料")
	}
	return nil
}

func memoryDrawQuote63(p p4Product) (p4Product, error) {
	tier, err := shopDrawQuantityTier(p.QuantityTiers)
	if err != nil {
		return p, err
	}
	if tier != nil {
		p.Rewards = strings.Split(p.Rewards, ",")[0] + fmt.Sprintf(",%d", tier.Amount)
		p.Cost = shopResourceSpec(tier.Cost)
		p.TierID = false
	}
	p.QuantityTiers = nil
	return p, nil
}
