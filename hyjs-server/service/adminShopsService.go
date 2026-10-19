package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strconv"
	"strings"
	"time"
)

// AdminShopDefinitions lists original consumers, with controls restricted to fields each consumes.
func AdminShopDefinitions() []model.AdminShopDefinition {
	base := []string{"rewards", "cost", "dailyLimit", "totalLimit", "stock", "minLevel", "minVip", "priority", "weight", "discountBps"}
	out := []model.AdminShopDefinition{}
	for _, v := range []struct{ id, label string }{{"ordinary", "普通商店"}, {"memory", "记忆圣坛"}, {"popularity", "声望商店商品"}, {"popularity-rank", "声望商店奖励"}, {"point", "攻略点商店"}, {"score", "积分商店"}, {"guild", "公会商店"}, {"pet-market", "精灵商店"}, {"union-fight", "公会战商店"}, {"vip", "会员礼包"}} {
		d := model.AdminShopDefinition{ID: v.id, Label: v.label, Fields: append([]string{}, base...), SettingsFields: []string{"unlimited", "enabled", "startAt", "endAt", "claimEndAt", "source"}, CanCreate: true, Notes: "保存后立即用于后续购买；历史交易、库存使用和限购保留。"}
		switch v.id {
		case "ordinary":
			d.Fields = append(d.Fields, "enabled")
			d.Notes = "商品、价格与材料保存后生效；原阶梯价格保留到该商品被显式修改。"
		case "memory":
			d.SettingsFields = append(d.SettingsFields, "slots")
		case "popularity-rank":
			d.Fields = append(d.Fields, "maxRank")
			d.Notes = "对应游戏声望商店的“奖励”页签。配置奖励道具、声望价格与最高排名门槛；每日/整期各1次，保留原排名证明。保存后生效，历史购买和排名资格保留。"
		case "guild":
			d.Fields = append(d.Fields, "contributionCost", "guildLevel")
		case "pet-market":
			d.Fields = []string{"rewards", "cost", "stock", "weight"}
			d.SettingsFields = []string{"unlimited", "startAt", "endAt", "claimEndAt", "enabled", "slots"}
			d.Notes = "变更立即更新当前可见商品，已用库存及刷新次数保留。原150钻刷新/每周10次和分解规则保留。"
		case "union-fight":
			d.Fields = []string{"rewards", "cost", "stock", "priority", "day", "buyCondition", "refreshMillis", "enabled", "source"}
			d.SettingsFields = []string{"unlimited", "startAt", "endAt", "claimEndAt"}
			d.Notes = "按公会战日期设置商品，保存后更新当前场次后续购买；已用库存和携带上限保留。"
		case "vip":
			d.Fields = []string{"rewards", "cost", "minVip", "enabled", "source"}
			d.SettingsFields = []string{"unlimited", "startAt", "endAt", "claimEndAt"}
			d.Notes = "对应游戏商城的“会员礼包”（原VIP商店，shopType=2）。空列表表示尚无已启用商品；请自行配置奖励、价格及会员条件后启用。保存后立即更新后续购买，已领次数保持。"
		}
		out = append(out, d)
	}
	return out
}
func AdminShopValid(id string) bool {
	for _, d := range AdminShopDefinitions() {
		if d.ID == id {
			return true
		}
	}
	return false
}

type adminShopStorage struct {
	Availability *db.AdminShopAvailability
	Legacy       []db.ShopProduct
	VIP          []db.VIPShopProduct
	Period       *db.P4CommerceRule
	Pet          *db.P9PetMarketRule
	Union        []db.P13UnionFightShopSource
}

func adminShopBaseProduct(id, reward, cost string) model.AdminShopProduct {
	return model.AdminShopProduct{ID: id, Rewards: adminTypedResources(reward), Cost: adminTypedResourceOne(cost), Enabled: true, Stock: -1, DailyLimit: 1, MinLevel: 1, Weight: 1, DiscountBps: 10000}
}
func adminShopP4Product(p p4Product) model.AdminShopProduct {
	v := adminShopBaseProduct("period:"+p.ID, p.Rewards, p.Cost)
	v.Name = p.Name
	v.DailyLimit = int64(p.DailyLimit)
	v.TotalLimit = int64(p.TotalLimit)
	v.Stock = int64(p.Stock)
	v.MinLevel = int64(p.MinLevel)
	v.MinVip = int64(p.MinVIP)
	v.Priority = int64(p.Priority)
	v.Weight = p.Weight
	v.DiscountBps = int64(p.DiscountBps)
	v.MaxRank = p.MaxRank
	return v
}
func (s *GameService) adminShopName(v *model.AdminShopProduct) {
	if v.Name != "" && !strings.Contains(v.Name, "name_") {
		return
	}
	v.Name = "待命名商品"
	if len(v.Rewards) > 0 && s.AdminCatalogueData != nil {
		for _, r := range append(append([]model.AdminItemDTO{}, s.AdminCatalogueData.Items...), s.AdminCatalogueData.UnionFightItems...) {
			if r.ID == v.Rewards[0].ResourceID {
				v.Name = r.Name
				return
			}
		}
	}
}

// adminReadShop clones the query builder for every source, preserving row locks without leaking tables.
func (s *GameService) adminReadShop(tx *gorm.DB, key string) (*model.AdminShopResponse, *adminShopStorage, error) {
	tx = tx.Session(&gorm.Session{})
	raw := &adminShopStorage{}
	out := &model.AdminShopResponse{ID: key, Items: []model.AdminShopProduct{}}
	if !AdminShopValid(key) {
		return nil, nil, constUtil.ErrAdminInput
	}
	if key == "ordinary" {
		if err := tx.Order("priority,id").Find(&raw.Legacy).Error; err != nil {
			return nil, nil, err
		}
		for _, p := range raw.Legacy {
			v := adminShopBaseProduct("legacy:"+p.ID, fmt.Sprintf("%s,%d", p.ResourceID, p.Amount), p.Cost)
			v.DailyLimit = int64(p.DailyLimit)
			v.MinLevel = int64(p.MinLevel)
			v.MinVip = int64(p.MinVIP)
			v.Priority = int64(p.Priority)
			v.Enabled = p.Enabled
			v.Source = p.Source
			out.Items = append(out.Items, v)
		}
	}
	switch key {
	case "vip":
		if err := tx.Order("id").Find(&raw.VIP).Error; err != nil {
			return nil, nil, err
		}
		for _, p := range raw.VIP {
			v := adminShopBaseProduct(p.ID, p.Reward, fmt.Sprintf("%s,%d", vipShopCostResource(p), p.Cost))
			v.MinVip = p.MinVIP
			v.Source = p.Source
			v.Enabled = p.Enabled
			out.Items = append(out.Items, v)
		}
	case "pet-market":
		var row db.P9PetMarketRule
		if err := tx.First(&row, "id=?", constUtil.P9MarketRuleID).Error; err != nil {
			return nil, nil, err
		}
		raw.Pet = &row
		var cfg p9MarketConfig
		if json.Unmarshal(row.Input, &cfg) != nil {
			return nil, nil, constUtil.ErrAdminConfiguration
		}
		out.Settings = model.AdminShopSettings{Enabled: row.Enabled, Slots: int64(cfg.Slots), Source: row.Source}
		for _, p := range cfg.Offers {
			v := adminShopBaseProduct(p.ID, fmt.Sprintf("%s,%d", p.Resource, p.Amount), fmt.Sprintf("%s,%d", p.Currency, p.Price))
			v.Stock = int64(p.Stock)
			v.Weight = p.Weight
			out.Items = append(out.Items, v)
		}
	case "union-fight":
		if err := tx.Order("day DESC,priority,shop_item_id").Find(&raw.Union).Error; err != nil {
			return nil, nil, err
		}
		for _, p := range raw.Union {
			v := adminShopBaseProduct(p.Day+"@"+p.ShopItemID, fmt.Sprintf("%s,%d", p.ItemID, p.Quantity), fmt.Sprintf("%s,%d", p13UFShopCostResource(p), p.Price))
			v.Stock = p.BuyCount
			v.Day = p.Day
			v.Priority = p.Priority
			v.BuyCondition = p.BuyCondition
			v.RefreshMillis = p.RefreshMillis
			v.Enabled = p.Enabled
			v.Source = p.Source
			out.Items = append(out.Items, v)
		}
		if s.AdminCatalogueData != nil {
			out.Resources = append(out.Resources, s.AdminCatalogueData.UnionFightItems...)
		}
	default:
		var row db.P4CommerceRule
		err := tx.Where("kind=? AND id NOT LIKE ?", key, "shop_schedule_%").Order("start_at DESC,id DESC").First(&row).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, err
		}
		if err == nil {
			raw.Period = &row
			out.Settings = model.AdminShopSettings{Enabled: row.Enabled, StartAt: row.StartAt, EndAt: row.EndAt, ClaimEndAt: row.ClaimEndAt, Source: row.Source, PeriodID: row.ID, Frozen: false}
			var rules p4CommerceRules
			if json.Unmarshal(row.Rules, &rules) != nil {
				return nil, nil, constUtil.ErrAdminConfiguration
			}
			out.Settings.Slots = int64(rules.Slots)
			for _, p := range rules.Products {
				out.Items = append(out.Items, adminShopP4Product(p))
			}
			if key == "guild" {
				var gr p4GuildRules
				_ = json.Unmarshal(row.Rules, &gr)
				for i := range out.Items {
					for _, p := range gr.Products {
						if out.Items[i].ID == "period:"+p.ID {
							out.Items[i].ContributionCost = int64(p.ContributionCost)
							out.Items[i].GuildLevel = int64(p.GuildLevel)
						}
					}
				}
			}
		}
	}
	for i := range out.Items {
		s.adminShopName(&out.Items[i])
		if key == "union-fight" {
			for _, r := range out.Resources {
				if len(out.Items[i].Rewards) > 0 && r.ID == out.Items[i].Rewards[0].ResourceID {
					out.Items[i].Name = r.Name
				}
			}
		}
	}
	availability, err := AdminShopAvailability(tx, key)
	if err != nil {
		return nil, nil, err
	}
	raw.Availability = availability
	if availability != nil {
		out.Settings.Unlimited = availability.Unlimited
		if !availability.Unlimited || availability.StartAt > 0 {
			out.Settings.StartAt, out.Settings.EndAt, out.Settings.ClaimEndAt = availability.StartAt, availability.EndAt, availability.ClaimEndAt
		}
	}
	availabilityRevision := int64(0)
	if availability != nil {
		availabilityRevision = availability.Revision
	}
	out.Version = adminVersion(struct {
		Source               *adminShopStorage
		AvailabilityRevision int64
	}{raw, availabilityRevision})
	return out, raw, nil
}
func (s *GameService) AdminShop(ctx context.Context, key string) (*model.AdminShopResponse, error) {
	out, _, err := s.adminReadShop(db.GetIns().WithContext(ctx), key)
	return out, err
}

func (s *GameService) adminShopMutation(ctx context.Context, actor *model.AdminPrincipal, key, version, action string, fn func(*gorm.DB, *adminShopStorage) error) (*model.AdminShopResponse, error) {
	var out *model.AdminShopResponse
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// All shop editors share one server row lock, including initially empty catalogs.
		if tx.Migrator().HasTable(&db.ServerGameClock{}) {
			var clock db.ServerGameClock
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&clock, "server_id=?", s.serverGameClockID()).Error; err != nil {
				return err
			}
		}
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		current, raw, err := s.adminReadShop(tx.Clauses(clause.Locking{Strength: "UPDATE"}), key)
		if err != nil {
			return err
		}
		if current.Version != version {
			return constUtil.ErrAdminConfigConflict
		}
		if err = s.ArchiveP4PopularityRankOffer(tx, raw.Period); err != nil {
			return err
		}
		if err = fn(tx, raw); err != nil {
			return err
		}
		if err = adminAudit(tx, actor, "shop."+action, key, "原商品配置更新；保留购买与库存历史"); err != nil {
			return err
		}
		out, _, err = s.adminReadShop(tx, key)
		return err
	})
	return out, err
}

func (s *GameService) AdminSaveShopProduct(ctx context.Context, actor *model.AdminPrincipal, key, id string, req model.AdminShopProductSaveRequest, create bool) (*model.AdminShopResponse, error) {
	return s.adminShopMutation(ctx, actor, key, req.Version, "product.save", func(tx *gorm.DB, raw *adminShopStorage) error {
		p := req.AdminShopProduct
		if create {
			id = "op_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
			if key == "popularity-rank" {
				id = constUtil.P4PopularityRankProductPrefix + id
			}
		}
		reward := ""
		if key != "union-fight" {
			var err error
			reward, err = s.adminTypedResourceSpec(p.Rewards, false)
			if err != nil {
				return err
			}
		}
		cost := fmt.Sprintf("%s,%d", p.Cost.ResourceID, p.Cost.Amount)
		switch key {
		case "vip":
			if p.MinVip < 1 || p.MinVip > 12 {
				return constUtil.ErrAdminInput
			}
			if _, err := s.p4ParseCost(cost); err != nil {
				return err
			}
			row := db.VIPShopProduct{ID: id, MinVIP: p.MinVip, CostResource: p.Cost.ResourceID, Cost: p.Cost.Amount, Reward: reward, Enabled: p.Enabled, Source: p.Source}
			if !create {
				found := false
				for _, old := range raw.VIP {
					if old.ID == id {
						found = true
					}
				}
				if !found {
					return fmt.Errorf("%w: 商品已不存在，请刷新", constUtil.ErrAdminInput)
				}
			}
			return tx.Save(&row).Error
		case "pet-market":
			if len(p.Rewards) != 1 {
				return constUtil.ErrAdminInput
			}
			var cfg p9MarketConfig
			_ = json.Unmarshal(raw.Pet.Input, &cfg)
			offer := p9MarketOffer{ID: id, Resource: p.Rewards[0].ResourceID, Amount: int32(p.Rewards[0].Amount), Currency: p.Cost.ResourceID, Price: int32(p.Cost.Amount), Stock: int32(p.Stock), Weight: p.Weight}
			found := false
			for i := range cfg.Offers {
				if cfg.Offers[i].ID == id {
					cfg.Offers[i] = offer
					found = true
				}
			}
			if create {
				cfg.Offers = append(cfg.Offers, offer)
			} else if !found {
				return fmt.Errorf("%w: 商品已不存在，请刷新", constUtil.ErrAdminInput)
			}
			if err := s.validateP9MarketConfig(&cfg); err != nil {
				return fmt.Errorf("%w: %v", constUtil.ErrAdminInput, err)
			}
			raw.Pet.Input = adminJSON(cfg)
			return tx.Save(raw.Pet).Error
		case "union-fight":
			if len(p.Rewards) != 1 {
				return constUtil.ErrAdminInput
			}
			item := s.table("UnionFightItem", "UFItemId", p.Rewards[0].ResourceID)
			cap, err := strconv.ParseInt(item["ItemCatchMaxNum"], 10, 64)
			if err != nil || p.Rewards[0].Amount > cap {
				return constUtil.ErrAdminInput
			}
			productID := id
			if !create {
				day, keyID, ok := strings.Cut(id, "@")
				if !ok || day != p.Day {
					return constUtil.ErrAdminInput
				}
				productID = keyID
				found := false
				for _, old := range raw.Union {
					if old.Day == day && old.ShopItemID == keyID {
						found = true
					}
				}
				if !found {
					return fmt.Errorf("%w: 商品已不存在，请刷新", constUtil.ErrAdminInput)
				}
			}
			if p.Cost.ResourceID != "unionFightToken" {
				if _, err := s.p4ParseCost(cost); err != nil {
					return err
				}
			}
			row := db.P13UnionFightShopSource{CostResource: p.Cost.ResourceID, Day: p.Day, ShopItemID: productID, Enabled: p.Enabled, ItemID: p.Rewards[0].ResourceID, Price: p.Cost.Amount, Quantity: p.Rewards[0].Amount, BuyCount: p.Stock, BuyCondition: p.BuyCondition, Priority: p.Priority, RefreshMillis: p.RefreshMillis, PublishedAt: time.Now().UnixMilli(), Source: p.Source}
			return tx.Save(&row).Error
		default:
			if strings.HasPrefix(id, "legacy:") {
				if key != "ordinary" || create || len(p.Rewards) != 1 {
					return constUtil.ErrAdminInput
				}
				originalID := strings.TrimPrefix(id, "legacy:")
				var row *db.ShopProduct
				for i := range raw.Legacy {
					if raw.Legacy[i].ID == originalID {
						row = &raw.Legacy[i]
					}
				}
				if row == nil {
					return fmt.Errorf("%w: 商品已不存在，请刷新", constUtil.ErrAdminInput)
				}
				row.LiveEditable = true
				row.ResourceID = p.Rewards[0].ResourceID
				row.Amount = int32(p.Rewards[0].Amount)
				row.Cost = cost
				row.MinLevel = int32(p.MinLevel)
				row.MinVIP = int32(p.MinVip)
				row.DailyLimit = int32(p.DailyLimit)
				row.Priority = int32(p.Priority)
				row.Enabled = p.Enabled
				if p.Source != "" {
					row.Source = p.Source
				}
				if _, err := s.shopItem(tx, &pb.Player{}, row, "0000-00-00"); err != nil {
					return fmt.Errorf("%w: %v", constUtil.ErrAdminInput, err)
				}
				return tx.Save(row).Error
			}
			row := raw.Period
			if row == nil {
				return fmt.Errorf("%w: 请先保存商店设置，再添加商品", constUtil.ErrAdminInput)
			}

			// A closed original rank seed has no issued window yet. Operators may
			// edit its catalogue directly; valid internal dates do not enable it.
			if !row.Enabled && (row.StartAt <= 0 || row.EndAt <= row.StartAt) {
				start := LocalPlayNow(ctx)
				row.StartAt, row.EndAt, row.ClaimEndAt = start.UnixMilli(), start.AddDate(0, 1, 0).UnixMilli(), start.AddDate(0, 1, 0).UnixMilli()
			}
			originalID := strings.TrimPrefix(id, "period:")
			var rules p4CommerceRules
			_ = json.Unmarshal(row.Rules, &rules)
			rules.LiveEditable = true
			if key == "popularity-rank" && rules.RankPublishedAt <= 0 {
				rules.RankPublishedAt = min(time.Now().UnixMilli(), row.StartAt-1)
			}
			found := -1
			nextIndex := int32(1)
			for i, old := range rules.Products {
				nextIndex = max(nextIndex, old.Index+1)
				if old.ID == originalID {
					found = i
				}
			}
			if !create && found < 0 {
				return fmt.Errorf("%w: 商品已不存在，请刷新", constUtil.ErrAdminInput)
			}
			value := p4Product{ID: originalID, Name: p.Name, Index: nextIndex, Rewards: reward, Cost: cost, Priority: int32(p.Priority), DailyLimit: int32(p.DailyLimit), TotalLimit: int32(p.TotalLimit), Stock: int32(p.Stock), MinLevel: int32(p.MinLevel), MinVIP: int32(p.MinVip), Weight: p.Weight, DiscountBps: int32(p.DiscountBps), MaxRank: p.MaxRank}
			if found >= 0 {
				value.Index = rules.Products[found].Index
				// An explicit product edit uses the configured linear price. Unedited
				// original tier products keep their original tier contract.
				value.TierID = false
				rules.Products[found] = value
			} else {
				rules.Products = append(rules.Products, value)
			}
			if key == "guild" {
				var gr p4GuildRules
				_ = json.Unmarshal(row.Rules, &gr)
				gr.LiveEditable = true
				entry := p4GuildProduct{p4Product: value, ContributionCost: int32(p.ContributionCost), GuildLevel: int32(p.GuildLevel)}
				if found >= 0 {
					gr.Products[found] = entry
				} else {
					gr.Products = append(gr.Products, entry)
				}
				row.Rules = adminJSON(gr)
				if _, err := s.p4GuildShopRules(*row); err != nil {
					return fmt.Errorf("%w: %v", constUtil.ErrAdminInput, err)
				}
			} else {
				row.Rules = adminJSON(rules)
				if _, err := s.validateP4Commerce(*row); err != nil {
					return fmt.Errorf("%w: %v", constUtil.ErrAdminInput, err)
				}
			}
			return tx.Save(row).Error
		}
	})
}

func (s *GameService) AdminDeleteShopProduct(ctx context.Context, actor *model.AdminPrincipal, key, id, version string) (*model.AdminShopResponse, error) {
	return s.adminShopMutation(ctx, actor, key, version, "product.remove", func(tx *gorm.DB, raw *adminShopStorage) error {
		switch key {
		case "vip":
			return tx.Model(&db.VIPShopProduct{}).Where("id=?", id).Update("enabled", false).Error
		case "union-fight":
			day, productID, ok := strings.Cut(id, "@")
			if !ok {
				return constUtil.ErrAdminInput
			}
			return tx.Model(&db.P13UnionFightShopSource{}).Where("day=? AND shop_item_id=?", day, productID).Update("enabled", false).Error
		case "pet-market":
			var cfg p9MarketConfig
			_ = json.Unmarshal(raw.Pet.Input, &cfg)
			offers := []p9MarketOffer{}
			for _, p := range cfg.Offers {
				if p.ID != id {
					offers = append(offers, p)
				}
			}
			if len(offers) == len(cfg.Offers) {
				return fmt.Errorf("%w: 商品已不存在，请刷新", constUtil.ErrAdminInput)
			}
			cfg.Offers = offers
			if err := s.validateP9MarketConfig(&cfg); err != nil {
				return fmt.Errorf("%w: %v", constUtil.ErrAdminInput, err)
			}
			raw.Pet.Input = adminJSON(cfg)
			return tx.Save(raw.Pet).Error
		default:
			if key == "ordinary" && strings.HasPrefix(id, "legacy:") {
				return tx.Model(&db.ShopProduct{}).Where("id=?", strings.TrimPrefix(id, "legacy:")).Update("enabled", false).Error
			}
			row := raw.Period
			if row == nil {
				return fmt.Errorf("%w: 商品已不存在，请刷新", constUtil.ErrAdminInput)
			}

			// Decode the product objects as raw entries so guild/rank fields remain intact.
			var document map[string]json.RawMessage
			_ = json.Unmarshal(row.Rules, &document)
			var products []json.RawMessage
			_ = json.Unmarshal(document["products"], &products)
			kept := []json.RawMessage{}
			for _, entry := range products {
				var p p4Product
				_ = json.Unmarshal(entry, &p)
				if p.ID != strings.TrimPrefix(id, "period:") {
					kept = append(kept, entry)
				}
			}
			if len(kept) == len(products) {
				return fmt.Errorf("%w: 商品已不存在，请刷新", constUtil.ErrAdminInput)
			}
			document["products"] = adminJSON(kept)
			document["liveEditable"] = adminJSON(true)
			row.Rules = adminJSON(document)
			if len(kept) == 0 {
				row.Enabled = false
			} else if _, err := s.validateP4Commerce(*row); err != nil {
				return fmt.Errorf("%w: %v", constUtil.ErrAdminInput, err)
			}
			return tx.Save(row).Error
		}
	})
}

func (s *GameService) AdminSaveShopSettings(ctx context.Context, actor *model.AdminPrincipal, key string, req model.AdminShopSettingsSaveRequest) (*model.AdminShopResponse, error) {
	return s.adminShopMutation(ctx, actor, key, req.Version, "settings", func(tx *gorm.DB, raw *adminShopStorage) error {
		// Operating hours are mutable independently from accepted product terms.
		hours := db.AdminShopAvailability{ShopKey: key, Revision: 1}
		if raw.Availability != nil {
			hours = *raw.Availability
			hours.Revision++
		}
		hours.Unlimited, hours.StartAt, hours.EndAt, hours.ClaimEndAt = req.Unlimited, req.StartAt, req.EndAt, req.ClaimEndAt
		if err := tx.Save(&hours).Error; err != nil {
			return err
		}
		if key == "vip" || key == "union-fight" {
			return nil
		}
		if key == "pet-market" {
			var cfg p9MarketConfig
			_ = json.Unmarshal(raw.Pet.Input, &cfg)
			cfg.Slots = int(req.Slots)
			if err := s.validateP9MarketConfig(&cfg); err != nil {
				return fmt.Errorf("%w: %v", constUtil.ErrAdminInput, err)
			}
			raw.Pet.Input = adminJSON(cfg)
			raw.Pet.Enabled = req.Enabled
			return tx.Save(raw.Pet).Error
		}
		row := raw.Period
		create := row == nil
		if row == nil {
			shopType := map[string]int32{"ordinary": 1, "memory": 3, "popularity": 4, "popularity-rank": 4, "point": 5, "score": 7, "guild": 8}[key]
			rules := p4CommerceRules{Products: []p4Product{}}
			if key == "memory" {
				rules.FreeCap = 10
				rules.FreeSeconds = 7200
				rules.ResourceRefreshCap = 15
				rules.RefreshItem = "item_013,1"
				rules.RefreshCost = "diamond,20"
				rules.Slots = 1
			}
			row = &db.P4CommerceRule{Kind: key, ShopType: shopType, Rules: adminJSON(rules)}
		}
		if create {
			row.ID = "p4:" + key + ":" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
			row.Frozen = nil
			row.Enabled = false
		} else {
			row.Enabled = req.Enabled
		}
		// Native source dates remain a valid frozen snapshot; actual business hours
		// come from the separate switch and never create a fresh quota implicitly.
		if req.StartAt > 0 && req.EndAt > req.StartAt && req.ClaimEndAt >= req.EndAt {
			row.StartAt, row.EndAt, row.ClaimEndAt = req.StartAt, req.EndAt, req.ClaimEndAt
		} else if create || row.StartAt <= 0 || row.EndAt <= row.StartAt {
			start := LocalPlayNow(ctx)
			if raw.Period != nil && start.UnixMilli() <= raw.Period.StartAt {
				start = time.UnixMilli(raw.Period.StartAt + 1)
			}
			row.StartAt, row.EndAt, row.ClaimEndAt = start.UnixMilli(), start.AddDate(0, 1, 0).UnixMilli(), start.AddDate(0, 1, 0).UnixMilli()
		}
		if key == "popularity-rank" {
			row.Source = constUtil.P4PopularityRankSource
		} else {
			row.Source = req.Source
		}
		var document map[string]json.RawMessage
		_ = json.Unmarshal(row.Rules, &document)
		document["liveEditable"] = adminJSON(true)
		if key == "memory" {
			document["slots"] = adminJSON(req.Slots)
		}
		if key == "popularity-rank" && (create || len(document["rankPublishedAt"]) == 0) {
			document["rankPublishedAt"] = adminJSON(min(time.Now().UnixMilli(), row.StartAt-1))
		}
		row.Rules = adminJSON(document)
		var rules p4CommerceRules
		_ = json.Unmarshal(row.Rules, &rules)
		if len(rules.Products) > 0 || row.Enabled {
			if _, err := s.validateP4Commerce(*row); err != nil {
				return fmt.Errorf("%w: %v", constUtil.ErrAdminInput, err)
			}
		}
		if create {
			return tx.Create(row).Error
		}
		return tx.Save(row).Error
	})
}

// Public quotes bind the displayed resource/reward/limits to a stable product.
// Receipts continue to compare the original unmodified request bytes first.
func adminLiveShopParseSKU(value string) (string, string, bool) {
	parts := strings.SplitN(value, ":", 3)
	if len(parts) != 3 || (parts[0] != "p4q" && parts[0] != "p4qt" && parts[0] != "s1q") || len(parts[1]) != 16 || parts[2] == "" {
		return "", value, false
	}
	return parts[1], parts[2], true
}
func adminLiveP4SKU(cfg db.P4CommerceRule, product p4Product, extra any) string {
	prefix := "p4q"
	if product.TierID {
		prefix = "p4qt"
	}
	hash := p6Digest(adminJSON(struct {
		Rule    string
		Product p4Product
		Extra   any
	}{cfg.ID, product, extra}))[:16]
	return prefix + ":" + hash + ":" + product.ID
}
func adminLiveLegacySKU(row db.ShopProduct) string {
	return "s1q:" + p6Digest(adminJSON(row))[:16] + ":" + row.ID
}
