package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"sort"
	"strconv"
	"strings"
	"time"
)

type growth75VIP7 struct {
	Price      int64 `json:"price_diamond"`
	TotalLimit int64 `json:"total_limit"`
}

type growth75Offer struct {
	Quantity int64 `json:"quantity"`
	Price    int64 `json:"price"`
}
type growth75Memory struct {
	Enabled        bool                     `json:"enabled"`
	RedQuantity    int64                    `json:"red_hero_fragment_quantity"`
	RedPrice       int64                    `json:"red_hero_fragment_light_price"`
	OrangeQuantity int64                    `json:"orange_hero_fragment_quantity"`
	OrangePrice    int64                    `json:"orange_hero_fragment_light_price"`
	PurpleQuantity int64                    `json:"purple_hero_fragment_quantity"`
	PurplePrice    int64                    `json:"purple_hero_fragment_light_price"`
	Materials      map[string]growth75Offer `json:"material_prices"`
}
type growth75PetMarket struct {
	Enabled          bool                     `json:"enabled"`
	FragmentQuantity int64                    `json:"fragment_quantity"`
	FragmentPrices   map[string]int64         `json:"fragment_prices"`
	Materials        map[string]growth75Offer `json:"material_overrides"`
}

// Stable matching uses resource identities, never the old quantity or price.
// The curated fixed experience bundles are unwrapped for matching their original SKU.
func (s *GameService) growth75ResourceSet(resources []model.AdminResourceAmount) string {
	keys := map[string]bool{}
	var add func(string)
	add = func(id string) {
		if keys[id] {
			return
		}
		if id == "item_9762" {
			keys["hero_exp_004"] = true
			return
		}
		if id == "item_9763" {
			keys["trea_010"] = true
			return
		}
		keys[id] = true
	}
	for _, r := range resources {
		add(r.ResourceID)
	}
	result := make([]string, 0, len(keys))
	for k := range keys {
		result = append(result, k)
	}
	sort.Strings(result)
	return strings.Join(result, ";")
}
func (s *GameService) growth75StableShopID(items []model.AdminShopProduct, used map[string]bool, want growth74Product) (string, error) {
	key := s.growth75ResourceSet(growth74Packed(want.Reward, want.Quantity))
	var candidates, enabled, named []model.AdminShopProduct
	for _, old := range items {
		if used[old.ID] || s.growth75ResourceSet(old.Rewards) != key {
			continue
		}
		candidates = append(candidates, old)
		if old.Enabled {
			enabled = append(enabled, old)
		}
		if old.Name == want.Name {
			named = append(named, old)
		}
	}
	if len(named) == 1 {
		return named[0].ID, nil
	}
	if len(enabled) == 1 {
		return enabled[0].ID, nil
	}
	if len(candidates) == 1 {
		return candidates[0].ID, nil
	}
	if len(candidates) == 0 {
		return "", nil
	}
	ids := []string{}
	for _, v := range candidates {
		ids = append(ids, v.ID)
	}
	return "", fmt.Errorf("%s存在同资源歧义SKU，请先明确归属：%s", want.Name, strings.Join(ids, ","))
}

// ApplyGrowth75Economy is called only by the explicit maintenance deployment CLI.
// It uses original admin validation/CAS first, then repairs only mutable operating
// catalogues and quote caches. No balance, purchase counter or business receipt is reset.
func (s *GameService) ApplyGrowth75Economy(ctx context.Context, a *model.AdminPrincipal) error {
	ctx = context.WithValue(ctx, constUtil.AdminServerScopeContextKey, s.OriginalServerID())
	var plan growth74Plan
	if err := json.Unmarshal(growth74Parameters, &plan); err != nil {
		return err
	}
	for _, key := range []string{"ordinary", "reputation", "trial", "bounty", "guild"} {
		source := plan.Stores[key]
		target := map[string]string{"reputation": "popularity", "trial": "point", "bounty": "score"}[key]
		if target == "" {
			target = key
		}
		if err := s.growth74Shop(ctx, a, target, source.Level, source.Products); err != nil {
			return fmt.Errorf("%s：%w", source.Name, err)
		}
	}
	if err := s.growth74Shop(ctx, a, "ggo", 1, plan.CreditShop.Products); err != nil {
		return fmt.Errorf("信用商店货架：%w", err)
	}
	if err := s.growth75MemoryShop(ctx, a, plan.MemoryChanges); err != nil {
		return fmt.Errorf("回忆圣坛货架：%w", err)
	}
	if err := s.growth75PetShop(ctx, a, plan.PetMarket); err != nil {
		return fmt.Errorf("精灵商店货架：%w", err)
	}
	if err := s.growth75VIP7Shop(ctx, a, plan.VIP7Once); err != nil {
		return fmt.Errorf("VIP7等级包：%w", err)
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, a, true); err != nil {
			return err
		}
		sources := map[string]db.P4CommerceRule{}
		for _, kind := range []string{"ordinary", "popularity", "point", "score", "guild", "memory", "ggo"} {
			source, err := s.AdminOrdinaryShopSource(tx, kind)
			if err != nil {
				return err
			}
			sources[kind] = source
		}
		rules, err := s.growth75SyncShopWindows(tx, a, sources)
		if err != nil {
			return fmt.Errorf("已发布商店排期快照：%w", err)
		}
		if err := s.growth75SyncShopQuotes(tx, a, rules, sources); err != nil {
			return fmt.Errorf("角色已发报价与配额快照：%w", err)
		}
		return nil
	})
}
func (s *GameService) growth75MemoryShop(ctx context.Context, a *model.AdminPrincipal, p growth75Memory) error {
	shop, err := s.AdminShop(ctx, "memory")
	if err != nil {
		return err
	}
	for _, v := range append([]model.AdminShopProduct(nil), shop.Items...) {
		if len(v.Rewards) != 1 {
			continue
		}
		id := v.Rewards[0].ResourceID
		offer, changed := p.Materials[id]
		if strings.HasPrefix(id, "frag_hero_") {
			h := s.table("Hero", "id", strings.TrimPrefix(id, "frag_"))
			quality := tableNumber(h, "quality")
			switch {
			case quality >= 23:
				if id != "frag_hero_001" && id != "frag_hero_003" && id != "frag_hero_005" {
					v.Enabled = false
				} else {
					offer = growth75Offer{p.RedQuantity, p.RedPrice}
				}
			case quality >= 20:
				offer = growth75Offer{p.OrangeQuantity, p.OrangePrice}
			default:
				offer = growth75Offer{p.PurpleQuantity, p.PurplePrice}
			}
			changed = true
		}
		if !changed {
			continue
		}
		if offer.Quantity > 0 && offer.Price > 0 {
			v.Rewards[0].Amount = offer.Quantity
			v.Cost = model.AdminResourceAmount{ResourceID: "light", Amount: offer.Price}
		}
		v.QuantityTiers = nil
		v.Source = "成长投放75：按日预算的小额圣坛兑换"
		shop, err = s.AdminSaveShopProduct(ctx, a, "memory", v.ID, model.AdminShopProductSaveRequest{AdminShopProduct: v, Version: shop.Version}, false)
		if err != nil {
			return err
		}
	}
	settings := shop.Settings
	settings.Enabled = p.Enabled
	settings.Unlimited = true
	_, err = s.AdminSaveShopSettings(ctx, a, "memory", model.AdminShopSettingsSaveRequest{AdminShopSettings: settings, Version: shop.Version})
	return err
}
func (s *GameService) growth75PetShop(ctx context.Context, a *model.AdminPrincipal, p growth75PetMarket) error {
	shop, err := s.AdminShop(ctx, "pet-market")
	if err != nil {
		return err
	}
	for _, v := range append([]model.AdminShopProduct(nil), shop.Items...) {
		if len(v.Rewards) != 1 {
			continue
		}
		id := v.Rewards[0].ResourceID
		offer, changed := p.Materials[id]
		if strings.HasPrefix(id, "frag_pet_") {
			frag := s.table("Frag", "id", id)
			pet := s.table("Pet", "id", frag["target"])
			price := p.FragmentPrices[pet["quality"]]
			if price < 1 {
				return fmt.Errorf("精灵碎片币价缺失：%s", id)
			}
			offer = growth75Offer{p.FragmentQuantity, price}
			changed = true
		}
		if !changed {
			continue
		}
		if offer.Quantity < 1 || offer.Price < 1 {
			return constUtil.ErrAdminInput
		}
		v.Rewards[0].Amount = offer.Quantity
		v.Cost = model.AdminResourceAmount{ResourceID: "coinpet", Amount: offer.Price}
		v.QuantityTiers = nil
		v.Source = "成长投放75：按实际分解币价的周商店"
		shop, err = s.AdminSaveShopProduct(ctx, a, "pet-market", v.ID, model.AdminShopProductSaveRequest{AdminShopProduct: v, Version: shop.Version}, false)
		if err != nil {
			return err
		}
	}
	settings := shop.Settings
	settings.Enabled = p.Enabled
	settings.Unlimited = true
	_, err = s.AdminSaveShopSettings(ctx, a, "pet-market", model.AdminShopSettingsSaveRequest{AdminShopSettings: settings, Version: shop.Version})
	return err
}

func growth75ShopForm(form model.AdminActivityForm, source db.P4CommerceRule) (model.AdminActivityForm, error) {
	form.Options.ShopSource = string(adminJSON(source))
	var rules p4CommerceRules
	if err := json.Unmarshal(source.Rules, &rules); err != nil {
		return form, err
	}
	form.Products = []model.AdminActivityProduct{}
	for _, p := range rules.Products {
		if !p.isEnabled() {
			continue
		}
		form.Products = append(form.Products, model.AdminActivityProduct{ID: p.ID, Name: p.Name, Index: int64(p.Index), Rewards: adminTypedResources(p.Rewards), Cost: adminTypedResourceOne(p.Cost), DailyLimit: int64(p.DailyLimit), TotalLimit: int64(p.TotalLimit), Stock: int64(p.Stock), MinLevel: int64(p.MinLevel), MinVip: int64(p.MinVIP), DiscountBps: int64(p.DiscountBps), Weight: p.Weight})
	}
	return form, nil
}
func growth75ChangedAudit(tx *gorm.DB, a *model.AdminPrincipal, target string, before, after []byte) error {
	return adminAudit(tx, a, "growth75.economy.sync", target, fmt.Sprintf("explicit operating config revision; before=%s; after=%s; balances, bought counters, refresh quota and immutable receipts retained", p6Digest(before), p6Digest(after)))
}
func growth75CAS(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return constUtil.ErrAdminConfigConflict
	}
	return nil
}

func (s *GameService) growth75SyncShopWindows(tx *gorm.DB, a *model.AdminPrincipal, sources map[string]db.P4CommerceRule) (map[string]db.P4CommerceRule, error) {
	byID := map[string]db.P4CommerceRule{}
	for _, source := range sources {
		byID[source.ID] = source
	}
	if !gameSchemaHasTable(tx, &db.AdminActivityOccurrence{}) {
		return byID, nil
	}
	var occurrences []db.AdminActivityOccurrence
	if err := tx.Where("server_id=? AND mode=? AND archived=?", s.OriginalServerID(), "shop-window", false).Order("id").Find(&occurrences).Error; err != nil {
		return nil, err
	}
	if len(occurrences) > 5000 {
		return nil, constUtil.ErrAdminConfiguration
	}
	versions := map[string]string{}
	{
		var definitions []db.AdminActivityDefinition
		if err := tx.Where("mode=? AND archived=?", "shop-window", false).Order("id").Limit(5001).Find(&definitions).Error; err != nil {
			return nil, err
		}
		if len(definitions) > 5000 {
			return nil, constUtil.ErrAdminConfiguration
		}
		for _, before := range definitions {
			var form model.AdminActivityForm
			if json.Unmarshal(before.FormJSON, &form) != nil {
				return nil, constUtil.ErrAdminConfiguration
			}
			source, ok := sources[form.Options.ShopKey]
			if !ok {
				continue
			}
			updated, err := growth75ShopForm(form, source)
			if err != nil {
				return nil, err
			}
			raw := adminJSON(updated)
			next := before
			if !bytes.Equal(raw, before.FormJSON) {
				next.FormJSON = raw
				next.Revision++
				next.UpdatedAtMillis = time.Now().UnixMilli()
				if err = growth75CAS(tx.Model(&db.AdminActivityDefinition{}).Where("id=? AND revision=? AND form_json=?", before.ID, before.Revision, before.FormJSON).Updates(map[string]any{"form_json": raw, "revision": next.Revision, "updated_at_millis": next.UpdatedAtMillis})); err != nil {
					return nil, fmt.Errorf("模板%s修订CAS: %w", before.ID, err)
				}
				if err = growth75ChangedAudit(tx, a, "shop-template/"+before.ID, before.FormJSON, raw); err != nil {
					return nil, err
				}
			}
			versions[next.ID] = adminVersion(next)
		}
	}
	occurrenceVersions := map[string]string{}
	for _, before := range occurrences {
		var form model.AdminActivityForm
		if json.Unmarshal(before.FormJSON, &form) != nil {
			return nil, constUtil.ErrAdminConfiguration
		}
		source, ok := sources[form.Options.ShopKey]
		if !ok {
			continue
		}
		updated, err := growth75ShopForm(form, source)
		if err != nil {
			return nil, err
		}
		raw := adminJSON(updated)
		next := before
		version := before.TemplateVersion
		if v := versions[before.TemplateID]; v != "" {
			version = v
		}
		if !bytes.Equal(raw, before.FormJSON) || version != before.TemplateVersion {
			next.FormJSON = raw
			next.TemplateVersion = version
			next.Revision++
			if err = growth75CAS(tx.Model(&db.AdminActivityOccurrence{}).Where("id=? AND revision=? AND form_json=?", before.ID, before.Revision, before.FormJSON).Updates(map[string]any{"form_json": raw, "template_version": version, "revision": next.Revision})); err != nil {
				return nil, fmt.Errorf("排期%s修订CAS: %w", before.ID, err)
			}
			if err = growth75ChangedAudit(tx, a, "shop-occurrence/"+before.ID, before.FormJSON, raw); err != nil {
				return nil, err
			}
		}
		occurrenceVersions[before.ID] = adminVersion(next)
		if before.ConsumerKey != "" && before.Timing != "character-day" {
			if err = s.growth75SyncNativeShop(tx, a, before.ConsumerKey, source); err != nil {
				return nil, err
			}
			source.ID = before.ConsumerKey
			byID[source.ID] = source
		}
	}
	if gameSchemaHasTable(tx, &db.AdminCharacterActivityInstance{}) {
		var instances []db.AdminCharacterActivityInstance
		if err := tx.Where("server_id=? AND mode=?", s.OriginalServerID(), "shop-window").Find(&instances).Error; err != nil {
			return nil, err
		}
		if len(instances) > 10000 {
			return nil, constUtil.ErrAdminConfiguration
		}
		for _, before := range instances {
			var form model.AdminActivityForm
			if json.Unmarshal(before.FormJSON, &form) != nil {
				return nil, constUtil.ErrAdminConfiguration
			}
			source, ok := sources[form.Options.ShopKey]
			if !ok {
				continue
			}
			updated, err := growth75ShopForm(form, source)
			if err != nil {
				return nil, err
			}
			raw := adminJSON(updated)
			version := before.SourceVersion
			if v := occurrenceVersions[before.OccurrenceID]; v != "" {
				version = v
			}
			if !bytes.Equal(raw, before.FormJSON) || version != before.SourceVersion {
				if err = growth75CAS(tx.Model(&db.AdminCharacterActivityInstance{}).Where("player_id=? AND occurrence_id=? AND source_version=? AND form_json=?", before.PlayerID, before.OccurrenceID, before.SourceVersion, before.FormJSON).Updates(map[string]any{"form_json": raw, "source_version": version})); err != nil {
					return nil, fmt.Errorf("个人实例%d/%s修订CAS: %w", before.PlayerID, before.OccurrenceID, err)
				}
				if err = growth75ChangedAudit(tx, a, "shop-character/"+strconv.FormatInt(before.PlayerID, 10)+"/"+before.OccurrenceID, before.FormJSON, raw); err != nil {
					return nil, err
				}
			}
			if err = s.growth75SyncNativeShop(tx, a, before.ConsumerKey, source); err != nil {
				return nil, err
			}
			source.ID = before.ConsumerKey
			byID[source.ID] = source
		}
	}
	return byID, nil
}
func (s *GameService) growth75SyncNativeShop(tx *gorm.DB, a *model.AdminPrincipal, key string, source db.P4CommerceRule) error {
	var before db.P4CommerceRule
	if err := tx.First(&before, "id=?", key).Error; err != nil {
		return err
	}
	next := before
	next.Rules = append([]byte(nil), source.Rules...)
	next.Source = source.Source
	if len(before.Frozen) > 0 {
		frozen := before
		frozen.Frozen = nil // Snapshot content must never recursively embed the prior frozen document.
		frozen.Rules = next.Rules
		frozen.Source = next.Source
		next.Frozen = adminJSON(frozen)
	}
	if _, err := s.validateP4Commerce(next); err != nil {
		return err
	}
	if bytes.Equal(before.Rules, next.Rules) && bytes.Equal(before.Frozen, next.Frozen) && before.Source == next.Source {
		return nil
	}
	query := tx.Model(&db.P4CommerceRule{}).Where("id=? AND rules=? AND source=? AND enabled=? AND start_at=? AND end_at=? AND claim_end_at=?", key, before.Rules, before.Source, before.Enabled, before.StartAt, before.EndAt, before.ClaimEndAt)
	// SQL NULL and an empty byte document have different types in SQLite.
	// Compare the value actually read; coalescing to text breaks an otherwise exact CAS.
	if before.Frozen == nil {
		query = query.Where("frozen IS NULL")
	} else {
		query = query.Where("frozen=?", before.Frozen)
	}
	result := query.Updates(map[string]any{"rules": next.Rules, "frozen": next.Frozen, "source": next.Source})
	if err := growth75CAS(result); err != nil {
		return fmt.Errorf("原商店消费者%s修订CAS: %w", key, err)
	}
	return growth75ChangedAudit(tx, a, "shop-consumer/"+key, adminJSON(before), adminJSON(next))
}

func (s *GameService) growth75SyncShopQuotes(tx *gorm.DB, a *model.AdminPrincipal, byID, sources map[string]db.P4CommerceRule) error {
	var ids []int64
	q := tx.Model(&db.Profile{}).Where("server_id=?", s.OriginalServerID())
	if s.OriginalServerID() == 1 {
		q = tx.Model(&db.Profile{}).Where("server_id IN ?", []int64{0, 1})
	}
	if err := q.Pluck("id", &ids).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	// Quote caches are owner-scoped; the global operating source is already validated.
	sourceProducts := map[string]map[string]p4Product{}
	for id, source := range byID {
		var r p4CommerceRules
		if json.Unmarshal(source.Rules, &r) != nil {
			return constUtil.ErrAdminConfiguration
		}
		m := map[string]p4Product{}
		for _, p := range r.Products {
			m[p.ID] = p
		}
		sourceProducts[id] = m
	}
	quoteProducts := map[string]map[string]p4Product{}
	for id, source := range byID {
		if source.Kind == "ggo" {
			quoteProducts[ggoQuoteRuleID(id)] = sourceProducts[id]
		}
	}
	if gameSchemaHasTable(tx, &db.P4CommerceState{}) {
		var states []db.P4CommerceState
		if err := tx.Where("player_id IN ?", ids).Find(&states).Error; err != nil {
			return err
		}
		for _, before := range states {
			raw := before.Payload
			if products, ok := quoteProducts[before.RuleID]; ok {
				var state ggoQuoteState
				if json.Unmarshal(raw, &state) != nil {
					return constUtil.ErrAdminConfiguration
				}
				if err := growth75RepriceGGO(&state, products); err != nil {
					return err
				}
				raw = adminJSON(state)
			} else if source, ok := byID[before.RuleID]; ok && source.Kind == "memory" {
				var state p4CommerceProgress
				if json.Unmarshal(raw, &state) != nil {
					return constUtil.ErrAdminConfiguration
				}
				growth75RepriceMemory(&state, sourceProducts[before.RuleID])
				raw = adminJSON(state)
			} else {
				continue
			}
			if bytes.Equal(raw, before.Payload) {
				continue
			}
			if err := growth75CAS(tx.Model(&db.P4CommerceState{}).Where("player_id=? AND rule_id=? AND payload=?", before.PlayerID, before.RuleID, before.Payload).Update("payload", raw)); err != nil {
				return err
			}
			if err := growth75ChangedAudit(tx, a, "shop-quote/"+strconv.FormatInt(before.PlayerID, 10)+"/"+before.RuleID, before.Payload, raw); err != nil {
				return err
			}
		}
	}
	var pet db.P9PetMarketRule
	if err := tx.First(&pet, "id=?", constUtil.P9MarketRuleID).Error; err != nil {
		return err
	}
	var petConfig p9MarketConfig
	if json.Unmarshal(pet.Input, &petConfig) != nil {
		return constUtil.ErrAdminConfiguration
	}
	var pets []db.P9PetMarketState
	if err := tx.Where("player_id IN ?", ids).Find(&pets).Error; err != nil {
		return err
	}
	for _, before := range pets {
		next := before
		if err := s.growth75RepricePet(&next, pet.Input, petConfig); err != nil {
			return err
		}
		if bytes.Equal(before.Input, next.Input) && bytes.Equal(before.Selected, next.Selected) {
			continue
		}
		if err := growth75CAS(tx.Model(&db.P9PetMarketState{}).Where("player_id=? AND week_key=? AND input_hash=? AND selected_hash=? AND generation=? AND refresh_used=? AND bought=?", before.PlayerID, before.WeekKey, before.InputHash, before.SelectedHash, before.Generation, before.RefreshUsed, before.Bought).Updates(map[string]any{"input": next.Input, "input_hash": next.InputHash, "selected": next.Selected, "selected_hash": next.SelectedHash})); err != nil {
			return err
		}
		if err := growth75ChangedAudit(tx, a, "pet-quote/"+strconv.FormatInt(before.PlayerID, 10), adminJSON(before), adminJSON(next)); err != nil {
			return err
		}
	}
	if !gameSchemaHasTable(tx, &db.LocalPlayActivityOperation{}) {
		return nil
	}
	var caches []db.LocalPlayActivityOperation
	if err := tx.Where("player_id IN ? AND action=? AND activity_id IN ?", ids, constUtil.CalendarQuota75Action, []string{constUtil.CalendarQuota75Pet, constUtil.CalendarQuota75GGO}).Find(&caches).Error; err != nil {
		return err
	}
	for _, before := range caches {
		var raw []byte
		if before.ActivityID == constUtil.CalendarQuota75Pet {
			var next db.P9PetMarketState
			if json.Unmarshal(before.Response, &next) != nil || next.PlayerID != before.PlayerID {
				return constUtil.ErrAdminConfiguration
			}
			if err := s.growth75RepricePet(&next, pet.Input, petConfig); err != nil {
				return err
			}
			raw = adminJSON(next)
		} else {
			var products map[string]p4Product
			for id, source := range byID {
				if source.Kind == "ggo" && strings.HasPrefix(before.RequestID, "calendar75:"+constUtil.CalendarQuota75GGO+":"+p6Digest([]byte(id))[:24]+":") {
					products = sourceProducts[id]
					break
				}
			}
			if products == nil {
				continue
			}
			var next ggoQuoteState
			if json.Unmarshal(before.Response, &next) != nil {
				return constUtil.ErrAdminConfiguration
			}
			if err := growth75RepriceGGO(&next, products); err != nil {
				return err
			}
			raw = adminJSON(next)
		}
		if bytes.Equal(raw, before.Response) {
			continue
		}
		if err := growth75CAS(tx.Model(&db.LocalPlayActivityOperation{}).Where("player_id=? AND request_id=? AND action=? AND activity_id=? AND response=?", before.PlayerID, before.RequestID, before.Action, before.ActivityID, before.Response).Update("response", raw)); err != nil {
			return err
		}
		if err := growth75ChangedAudit(tx, a, "shop-calendar/"+strconv.FormatInt(before.PlayerID, 10)+"/"+before.RequestID, before.Response, raw); err != nil {
			return err
		}
	}
	return nil
}
func growth75RepriceMemory(state *p4CommerceProgress, products map[string]p4Product) {
	for i := range state.Offers {
		if product, ok := products[state.Offers[i].ProductID]; ok {
			product.QuantityTiers = nil
			state.Offers[i].Quote = &product
		}
	}
}
func growth75RepriceGGO(state *ggoQuoteState, products map[string]p4Product) error {
	if state.Generation < 1 || len(state.Slots) != 6 || state.SlotBought == nil {
		return constUtil.ErrAdminConfiguration
	}
	for i := range state.Slots {
		slot := &state.Slots[i]
		product, ok := products[slot.SourceProductID]
		if !ok {
			return constUtil.ErrAdminConfiguration
		}
		product.ID = slot.ID
		product.Index = int32(i + 1)
		product.SourceProductID = slot.SourceProductID
		product.QuantityTiers = nil
		slot.Product = product
	}
	return nil
}
func (s *GameService) growth75RepricePet(state *db.P9PetMarketState, input []byte, cfg p9MarketConfig) error {
	var old p9MarketConfig
	if state.InputHash != p6Digest(state.Input) || json.Unmarshal(state.Input, &old) != nil {
		return constUtil.ErrAdminConfiguration
	}
	selected, _, err := s.p9MarketInventory(state, &old)
	if err != nil {
		return err
	}
	offers := map[string]p9MarketOffer{}
	for _, v := range cfg.Offers {
		offers[v.ID] = v
	}
	for i, v := range selected {
		next, ok := offers[v.ID]
		if !ok {
			return constUtil.ErrAdminConfiguration
		}
		next.SlotID = v.SlotID
		next.QuantityTiers = nil
		selected[i] = next
	}
	state.Input = append([]byte(nil), input...)
	state.InputHash = p6Digest(state.Input)
	state.Selected = adminJSON(selected)
	state.SelectedHash = p6Digest(state.Selected)
	return nil
}

// Reprice the existing VIP7 product, retaining every old membership claim.
func (s *GameService) growth75VIP7Shop(ctx context.Context, a *model.AdminPrincipal, p growth75VIP7) error {
	if p.Price < 2 || p.TotalLimit != 1 {
		return constUtil.ErrAdminConfiguration
	}
	shop, err := s.AdminShop(ctx, "vip")
	if err != nil {
		return err
	}
	var chosen *model.AdminShopProduct
	for i := range shop.Items {
		if shop.Items[i].MinVip == 7 {
			if chosen != nil {
				return fmt.Errorf("VIP7等级礼包存在多个商品，请明确稳定商品归属")
			}
			chosen = &shop.Items[i]
		}
	}
	if chosen == nil {
		return fmt.Errorf("原VIP7等级礼包缺失，禁止另建身份重置限购")
	}
	v := *chosen
	rewards := []model.AdminResourceAmount{}
	for _, r := range v.Rewards {
		if r.ResourceID != "item_063" && !s.growth74RedGear(r.ResourceID, map[string]bool{}) {
			rewards = append(rewards, r)
		}
	}
	v.Rewards = append(rewards, model.AdminResourceAmount{ResourceID: "item_063", Amount: 1})
	v.Cost = model.AdminResourceAmount{ResourceID: "diamond", Amount: p.Price}
	v.TotalLimit = 1
	v.Enabled = true
	v.Source = "成长投放75 · VIP7等级礼包，每角色总限1；所有历史购买计入"
	_, err = s.AdminSaveShopProduct(ctx, a, "vip", v.ID, model.AdminShopProductSaveRequest{AdminShopProduct: v, Version: shop.Version}, false)
	return err
}
