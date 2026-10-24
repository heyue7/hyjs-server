package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

type p4GuildProduct struct {
	p4Product
	ContributionCost int32 `json:"contributionCost"`
	GuildLevel       int32 `json:"guildLevel"`
}
type p4GuildRules struct {
	LiveEditable bool             `json:"liveEditable,omitempty"`
	Products     []p4GuildProduct `json:"products"`
}

// seedP4GuildShop publishes a separate default-off catalog for the original union shop8.
func (s *GameService) seedP4GuildShop(tx *gorm.DB, now time.Time) error {
	local := now.In(time.FixedZone("UTC+8", 8*3600))
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, local.Location())
	key := constUtil.P4GuildShopPrefix + start.Format("200601")
	var count int64
	if err := tx.Model(&db.P4CommerceRule{}).Where("id=?", key).Count(&count).Error; err != nil || count > 0 {
		return err
	}
	if !strings.Contains(s.table("Item", "itemId", "item_005")["resource"], "gonghuishangdian_Get") {
		return errors.New("原公会商店解放石来源缺失")
	}
	product := p4BaseProduct("p4Guild_1", "item_005,50", "item_311,1", 1, 20)
	product.DailyLimit = 1
	product.TotalLimit = 3
	raw, _ := json.Marshal(p4GuildRules{Products: []p4GuildProduct{{p4Product: product, ContributionCost: 1, GuildLevel: 1}}})
	cfg := db.P4CommerceRule{ID: key, ShopType: constUtil.P4GuildShopType, Kind: "guild", Enabled: false, StartAt: start.UnixMilli(), EndAt: start.AddDate(0, 1, 0).UnixMilli(), ClaimEndAt: start.AddDate(0, 1, 0).UnixMilli(), Rules: raw, Source: constUtil.P4GuildShopSource}
	if _, err := s.p4GuildShopRules(cfg); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error
}

func (s *GameService) p4GuildShopRules(cfg db.P4CommerceRule) (p4GuildRules, error) {
	var rules p4GuildRules
	if _, err := s.validateP4Commerce(cfg); err != nil {
		return rules, err
	}
	if cfg.Kind != "guild" || cfg.ShopType != constUtil.P4GuildShopType || json.Unmarshal(cfg.Rules, &rules) != nil {
		return rules, errors.New("原公会商店目录无效")
	}
	for _, product := range rules.Products {
		cost, err := s.p4ParseCost(product.Cost)
		if err != nil || cost == nil || product.ContributionCost < 0 || product.GuildLevel < 1 || product.GuildLevel > 100 || product.TierID || product.DiscountBps != 10000 {
			return rules, errors.New("原公会商店双成本或等级配置无效")
		}
		resources, err := p1RewardResources(product.Rewards)
		if err != nil || len(resources) != 1 {
			return rules, errors.New("原公会商店每商品仅显示一个原资源")
		}
	}
	return rules, nil
}

// p4GuildShopProjection keeps exact ordered costs expected by original costType3.
func (s *GameService) p4GuildShopProjection(tx *gorm.DB, p *pb.Player, cfg db.P4CommerceRule, rules p4CommerceRules, state *p4CommerceProgress, guild *db.P13Guild) ([]*pb.ShopItem, error) {
	items, _, err := s.p4Projection(tx, p, cfg, rules, state)
	if err != nil {
		return nil, err
	}
	custom, err := s.p4GuildShopRules(cfg)
	if err != nil {
		return nil, err
	}
	byID := map[string]p4GuildProduct{}
	for _, product := range custom.Products {
		byID[product.ID] = product
	}
	for _, item := range items {
		_, stable, quoted := adminLiveShopParseSKU(item.GetId())
		if !quoted {
			stable = item.GetId()
		}
		product := byID[stable]
		if rules.LiveEditable {
			item.Id = proto.String(adminLiveP4SKU(cfg, product.p4Product, struct {
				ContributionCost int32
				GuildLevel       int32
			}{product.ContributionCost, product.GuildLevel}))
		}
		badge, err := s.p4ParseCost(product.Cost)
		if err != nil {
			return nil, err
		}
		costs := []*pb.BaseResource{badge, {Id: proto.String("contribution"), Num: proto.Int32(product.ContributionCost)}}
		item.ExchangeList = costs
		item.Price = costs
		item.Condition = &pb.ShopCondition{Type: proto.String("unionLevel"), Value: proto.Int64(int64(product.GuildLevel))}
		switch {
		case p.GetLevel() < product.MinLevel || p.GetVip() < product.MinVIP || guild.Level < product.GuildLevel:
			item.CanBuy = proto.Int32(0)
		case item.GetDayLeftTimes() <= 0 || item.GetTotalLeftTimes() <= 0:
			item.CanBuy = proto.Int32(2)
		default:
			item.CanBuy = proto.Int32(1)
		}
	}
	return items, nil
}

// p4GuildShop locks the existing P13 gate before Profile to preserve guild mutation ordering.
// The dual costs, actual usable contribution, reward, quota and original response commit together.
func (s *GameService) p4GuildShop(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, error) {
	var response proto.Message
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if !gameSchemaHasTable(tx, &db.P13GuildPolicy{}) {
			return errors.New(constUtil.P13GuildClosed)
		}
		var policy db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
			return err
		}
		var profile db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
			return err
		}
		player, game, err := decodeProfileForCalendarUpdate(tx, &profile)
		if err != nil {
			return err
		}
		member, guild, err := p13GuildNeedMember(tx, id)
		if err != nil {
			return err
		}
		domain := "commerce:8:" + p6Digest([]byte(member.GuildID))[:32]
		buy, purchasing := request.(*pb.CSShopBuy)
		if purchasing {
			if buy.GetShopType() != constUtil.P4GuildShopType || buy.GetBuyCount() < 1 || buy.GetBuyCount() > 100 {
				return errors.New("原公会商店购买数量须为1至100")
			}
			// An earlier purchase belongs to its actual guild; a delayed old request cannot buy again after changing guild.
			var other int64
			if err = tx.Model(&db.P4Receipt{}).Where("player_id=? AND timestamp=? AND domain LIKE ? AND domain<>?", id, stamp, "commerce:8:%", domain).Count(&other).Error; err != nil {
				return err
			}
			if other > 0 {
				return errors.New("原公会已变更，请重新读取商店")
			}
			prior, err := p4ReadReceipt(tx, id, domain, stamp, request)
			if err != nil {
				return err
			}
			if prior != nil {
				response = prior
				return nil
			}
		} else if query, ok := request.(*pb.CSShopProductList); !ok || query.GetShopType() != constUtil.P4GuildShopType {
			return errors.New("原公会商店请求类型无效")
		}
		if !policy.ManagementEnabled || !policy.EconomyEnabled {
			return errors.New(constUtil.P13GuildEconomyClosed)
		}
		cfg, rules, row, state, err := s.loadP4Commerce(tx, id, "", constUtil.P4GuildShopType, now)
		if err != nil {
			return err
		}
		custom, err := s.p4GuildShopRules(cfg)
		if err != nil {
			return err
		}
		var gain *pb.ResultObject
		var costs []*pb.BaseResource
		if purchasing {
			var product *p4GuildProduct
			_, stable, quoted := adminLiveShopParseSKU(buy.GetShopItemId())
			if !quoted {
				stable = buy.GetShopItemId()
			}
			for i := range custom.Products {
				if custom.Products[i].ID == stable {
					product = &custom.Products[i]
				}
			}
			if product == nil || !product.isEnabled() || shopHasDirectStamina(product.Rewards) || player.GetLevel() < product.MinLevel || player.GetVip() < product.MinVIP || guild.Level < product.GuildLevel {
				return errors.New("原公会商店商品或实际公会等级未满足")
			}
			if custom.LiveEditable && buy.GetShopItemId() != adminLiveP4SKU(cfg, product.p4Product, struct {
				ContributionCost int32
				GuildLevel       int32
			}{product.ContributionCost, product.GuildLevel}) {
				return errors.New("商品已调整，请刷新商店后重新购买")
			}
			count := buy.GetBuyCount()
			key := p4SlotKey(cfg, state, product.p4Product)
			if state.Bought[key] < 0 || state.Total[product.ID] < 0 || int64(state.Bought[key])+int64(count) > int64(product.DailyLimit) || int64(state.Total[product.ID])+int64(count) > math.MaxInt32 || product.TotalLimit > 0 && int64(state.Total[product.ID])+int64(count) > int64(product.TotalLimit) {
				return errors.New("原公会商店日或月限购已用完")
			}
			if product.Stock >= 0 {
				var stock db.P4CommerceStock
				err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("rule_id=? AND product_id=?", cfg.ID, product.ID).First(&stock).Error
				if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				if stock.Used < 0 || int64(stock.Used)+int64(count) > int64(product.Stock) {
					return errors.New("共享库存不足")
				}
				stock.RuleID, stock.ProductID, stock.Used = cfg.ID, product.ID, stock.Used+count
				if err := tx.Save(&stock).Error; err != nil {
					return err
				}
			}
			badge, err := s.p4ParseCost(product.Cost)
			if err != nil {
				return err
			}
			if int64(badge.GetNum())*int64(count) > math.MaxInt32 || int64(product.ContributionCost)*int64(count) > math.MaxInt32 {
				return errors.New("批量公会商店成本溢出")
			}
			badge.Num = proto.Int32(badge.GetNum() * count)
			contributionCost := product.ContributionCost * count
			requiredContribution := int64(contributionCost)
			if badge.GetId() == "contribution" {
				requiredContribution += int64(badge.GetNum())
			}
			if int64(player.GetContribution()) < requiredContribution {
				return errors.New("可用公会贡献不足")
			}
			source := fmt.Sprintf("p4:guild:%d:%d", id, stamp)
			if err = s.p4Deduct(tx, id, player, game, badge, source, now); err != nil {
				return err
			}
			player.Contribution = proto.Int32(player.GetContribution() - contributionCost)
			// Member.Contribution is the original cumulative ranking value and remains unchanged.
			if contributionCost > 0 {
				if err = s.RecordP1Event(tx, id, source+":contribution", "spend:contribution", int64(contributionCost), now); err != nil {
					return err
				}
			}
			rewards, err := p4ScaleRewards(product.Rewards, count)
			if err != nil {
				return err
			}
			gain, err = s.GrantFixedReward(player, game, rewards)
			if err != nil {
				return err
			}
			costs = []*pb.BaseResource{badge, {Id: proto.String("contribution"), Num: proto.Int32(contributionCost)}}
			state.Bought[key] += count
			state.Total[product.ID] += count
		}
		items, err := s.p4GuildShopProjection(tx, player, cfg, rules, state, guild)
		if err != nil {
			return err
		}
		if err = saveP4Commerce(tx, row, state); err != nil {
			return err
		}
		if purchasing {
			response = &pb.SCShopBuy{ShopType: proto.Int32(constUtil.P4GuildShopType), ShopItemList: items, Get: gain, Cost: costs, Contribution: player.Contribution}
			if err = p4SaveReceipt(tx, id, domain, stamp, request, response); err != nil {
				return err
			}
		} else {
			response = &pb.SCShopProductList{ShopType: proto.Int32(constUtil.P4GuildShopType), ShopItemList: items, Contribution: player.Contribution}
		}
		return saveProfile(tx.WithContext(context.WithValue(tx.Statement.Context, constUtil.GameRequestContextKey, nil)), &profile, player, game)
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	if _, purchasing := request.(*pb.CSShopBuy); purchasing {
		return s.shopInventorySync(ctx, id, []proto.Message{response})
	}
	return []proto.Message{response}, nil
}
