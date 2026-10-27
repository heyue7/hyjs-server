package service

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"strings"
	"time"
)

//go:embed data/growth74_parameters.json
var growth74Parameters []byte

type growth74Product struct {
	Name, Reward, Currency, Period string
	Quantity, Price, Limit, Weight int64
	MinimumPlayerLevel             int64 `json:"minimum_player_level"`
	Menu                           int64 `json:"menu"`
	ContributionCost               int64 `json:"contribution_cost"`
}
type growth74Store struct {
	Name     string
	Level    int64
	Products []growth74Product
}
type growth74Plan struct {
	Stores        map[string]growth74Store
	CreditShop    struct{ Products []growth74Product } `json:"credit_shop"`
	MemoryChanges growth75Memory                       `json:"memory_changes"`
	VIP7Once      growth75VIP7                         `json:"vip7_once75"`
	PetMarket     growth75PetMarket                    `json:"pet_market_policy"`
}

// ApplyGrowth74 is an explicit deployment command. Normal startup never applies it.
// It reuses the same validated, audited operators as the admin and requires maintenance.
func (s *GameService) ApplyGrowth74(ctx context.Context) error {
	var policy db.ClientUpdatePolicy
	d := db.GetIns().WithContext(ctx)
	if e := d.First(&policy, 1).Error; e != nil {
		return e
	}
	if !policy.Maintenance {
		return fmt.Errorf("先关服再应用成长投放")
	}
	var u db.AdminUser
	if e := d.Where("enabled=? AND role=?", true, constUtil.AdminRoleAdmin).Order("id").First(&u).Error; e != nil {
		return e
	}
	a := &model.AdminPrincipal{UserID: u.ID, Username: u.Username + "/成长部署", Role: u.Role}
	var done int64
	if e := d.Model(&db.AdminAudit{}).Where("action=? AND target=?", "growth74.applied", fmt.Sprint(s.OriginalServerID())).Count(&done).Error; e != nil {
		return e
	}
	if done > 0 {
		return nil
	}

	ctx = context.WithValue(ctx, constUtil.AdminServerScopeContextKey, s.OriginalServerID())
	var p growth74Plan
	if e := json.Unmarshal(growth74Parameters, &p); e != nil {
		return e
	}
	for _, key := range []string{"ordinary", "reputation", "trial", "bounty", "guild"} {
		store := p.Stores[key]
		shop := map[string]string{"reputation": "popularity", "trial": "point", "bounty": "score"}[key]
		if shop == "" {
			shop = key
		}
		if e := s.growth74Shop(ctx, a, shop, store.Level, store.Products); e != nil {
			return fmt.Errorf("%s: %w", store.Name, e)
		}
	}
	if e := s.growth74Shop(ctx, a, "ggo", 1, p.CreditShop.Products); e != nil {
		return fmt.Errorf("信用商店: %w", e)
	}
	if e := s.growth74Extra(ctx, a); e != nil {
		return e
	}
	if e := s.growth74Field(ctx, a); e != nil {
		return e
	}
	cfg, e := AdminTerminalSettings(ctx, a)
	if e != nil {
		return e
	}
	cfg.SignupPoints = 5000
	cfg.Weekly = []model.TerminalWeeklyRule{{ID: "growth-weekly-648", Kind: "weekly", Weekday: 1, Time: "00:00", Points: 648, Enabled: true}, {ID: "growth-monthly-3000", Kind: "monthly", Day: 15, Time: "00:00", Points: 3000, Enabled: true}}
	if _, e = AdminSaveTerminalSettings(ctx, a, *cfg); e != nil {
		return e
	}
	tasks, e := s.AdminDailyTasks69(ctx, a)
	if e != nil {
		return e
	}
	tr := model.AdminDailyTask69SaveRequest{Version: tasks.Version, Reason: "成长投放：每日最终档128超频点数"}
	for _, v := range tasks.Items {
		r := v.Reward
		if r.ResourceID == "item_9711" {
			r = adminTypedResourceOne(dailyTask69DefaultReward(v.Type))
		}
		tr.Items = append(tr.Items, model.AdminDailyTask69Reward{ID: v.ID, Reward: r})
	}
	for _, b := range tasks.Boxes {
		r := []model.AdminResourceAmount{}
		for _, v := range b.Rewards {
			if v.ResourceID != "item_9711" {
				r = append(r, v)
			}
		}
		if b.Score == 120 {
			r = append(r, model.AdminResourceAmount{ResourceID: "item_9711", Amount: 128})
		}
		tr.Boxes = append(tr.Boxes, model.AdminDailyTaskBoxReward74{ID: b.ID, Rewards: r})
	}
	if _, e = s.AdminSaveDailyTasks69(ctx, a, tr); e != nil {
		return e
	}
	if e = s.growth74Activities(ctx, a); e != nil {
		return e
	}
	if e = s.growth74TestDay(ctx, a); e != nil {
		return e
	}
	if e = s.growth74Release(ctx, a); e != nil {
		return e
	}
	return adminAudit(d, a, "growth74.applied", fmt.Sprint(s.OriginalServerID()), "V12投放及全部开服窗口至少14天；保留历史，游戏仍处于维护")
}

func growth74Packed(id string, n int64) []model.AdminResourceAmount {
	if id == "NEW_OS_SELECTOR" {
		id = "item_9760"
	}
	if id == "NEW_KING_SELECTOR" {
		id = "item_9761"
	}
	pack := int64(0)
	box := ""
	if id == "hero_exp_004" && n > 100 {
		pack = 5
		box = "item_9762"
	}
	if id == "trea_010" && n > 100 {
		pack = 50
		box = "item_9763"
	}
	if pack == 0 {
		return []model.AdminResourceAmount{{ResourceID: id, Amount: n}}
	}
	r := []model.AdminResourceAmount{{ResourceID: box, Amount: n / pack}}
	if n%pack > 0 {
		r = append(r, model.AdminResourceAmount{ResourceID: id, Amount: n % pack})
	}
	return r
}
func (s *GameService) growth74Shop(ctx context.Context, a *model.AdminPrincipal, key string, level int64, products []growth74Product) error {
	shop, e := s.AdminShop(ctx, key)
	if e != nil {
		return e
	}
	used := map[string]bool{}
	for i, v := range products {
		id, matchErr := s.growth75StableShopID(shop.Items, used, v)
		if matchErr != nil {
			return matchErr
		}
		beforeIDs := map[string]bool{}
		for _, old := range shop.Items {
			beforeIDs[old.ID] = true
		}
		currency := v.Currency
		if currency == "credit" {
			currency = "item_9710"
		}
		if currency == "trial" {
			currency = "point"
		}
		q := model.AdminShopProduct{ID: id, Name: v.Name, Rewards: growth74Packed(v.Reward, v.Quantity), Cost: model.AdminResourceAmount{ResourceID: currency, Amount: v.Price}, Stock: -1, MinLevel: level, Priority: int64(100 + i), Weight: 1, DiscountBps: 10000, Enabled: true, Source: "成长投放V12 / 2026-10-26"}
		if key == "point" && v.Menu > 0 {
			q.Priority = v.Menu*1000 + int64(100+i)
		}
		if v.MinimumPlayerLevel > q.MinLevel {
			q.MinLevel = v.MinimumPlayerLevel
		}
		if v.Period == "day" {
			q.DailyLimit = v.Limit
		} else {
			q.TotalLimit = v.Limit
			q.DailyLimit = v.Limit
		}
		if key == "guild" {
			q.Cost.ResourceID = "item_311"
			q.ContributionCost = max(int64(1), v.ContributionCost)
			q.GuildLevel = 1
			q.DailyLimit = 1
		}
		if key == "point" && (v.Reward == "item_119" || v.Reward == "item_118") {
			q.MinLevel = 60
		}
		if key == "ggo" {
			q.Weight = v.Weight
			q.TotalLimit = 0
			q.DailyLimit = 10000
			if v.Period == "week" {
				q.AccountWeeklyLimit = v.Limit
			} else {
				q.DailyLimit = v.Limit
			}
			// Quote.SlotBought limits each displayed slot; source stock is shared across all roles.
			q.Stock = -1
		}
		next, err := s.AdminSaveShopProduct(ctx, a, key, id, model.AdminShopProductSaveRequest{AdminShopProduct: q, Version: shop.Version}, id == "")
		if err != nil {
			return fmt.Errorf("%s: %w", v.Name, err)
		}
		shop = next
		if id != "" {
			used[id] = true
		} else {
			created := ""
			for _, row := range shop.Items {
				if !beforeIDs[row.ID] {
					if created != "" {
						return fmt.Errorf("商店新增商品标识不唯一")
					}
					created = row.ID
				}
			}
			if created == "" {
				return fmt.Errorf("商店新增商品标识缺失")
			}
			used[created] = true
		}
	}
	for _, old := range append([]model.AdminShopProduct(nil), shop.Items...) {
		if used[old.ID] || !old.Enabled {
			continue
		}
		old.Enabled = false
		shop, e = s.AdminSaveShopProduct(ctx, a, key, old.ID, model.AdminShopProductSaveRequest{AdminShopProduct: old, Version: shop.Version}, false)
		if e != nil {
			return e
		}
	}
	settings := shop.Settings
	settings.Enabled = true
	settings.Unlimited = true
	settings.Source = "成长投放V12"
	if key == "point" || key == "guild" {
		settings.Unlimited = false
		settings.StartAt = 1
		settings.EndAt = 253402214400000
		settings.ClaimEndAt = settings.EndAt
	}
	if key == "ggo" {
		settings.Slots = 6
		settings.Refresh = &model.AdminShopRefreshSettings{Enabled: true, FreeCap: 0, PaidCap: 15, Cost: model.AdminResourceAmount{ResourceID: "item_9710", Amount: 10}, Period: "day"}
	}
	_, e = s.AdminSaveShopSettings(ctx, a, key, model.AdminShopSettingsSaveRequest{AdminShopSettings: settings, Version: shop.Version})
	if e != nil {
		return fmt.Errorf("%s营业与刷新设置: %w", key, e)
	}
	return nil
}

func (s *GameService) growth74TestDay(ctx context.Context, a *model.AdminPrincipal) error {
	var realm db.AdminGameServer
	if e := db.GetIns().WithContext(ctx).First(&realm, s.OriginalServerID()).Error; e != nil {
		return e
	}
	zone := time.FixedZone("Asia/Shanghai", 8*3600)
	open := time.UnixMilli(realm.OpenAt).In(zone)
	target := time.Date(open.Year(), open.Month(), open.Day()+6, 0, 0, 0, 0, zone)
	now := time.Now().In(zone)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, zone)
	if e := s.growth74PrepareTestCalendar(ctx, a, target.AddDate(0, 0, -6).UnixMilli()); e != nil {
		return e
	}
	want := int64(target.Sub(today) / (24 * time.Hour))
	clock, e := s.AdminServerGameClock(ctx, a)
	if e != nil {
		return e
	}
	// Use the original clock transaction and live-battle guard. No OS/accounting clock change.
	for clock.OffsetDays != want {
		delta := int64(1)
		if clock.OffsetDays > want {
			delta = -1
		}
		clock, e = s.AdminChangeServerGameClock(ctx, a, model.ServerGameClockChangeRequest{DeltaDays: delta, Revision: clock.Revision, RequestID: fmt.Sprintf("growth74-day7-%d", clock.Revision)})
		if e != nil {
			return e
		}
	}
	return nil
}

func growth74Source(s string) bool { return strings.Contains(s, "成长投放V12") }
