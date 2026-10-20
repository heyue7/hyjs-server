package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm/clause"
	"math"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func vipShopCostResource(cfg db.VIPShopProduct) string {
	if cfg.CostResource == "" {
		return "diamond"
	}
	return cfg.CostResource
}
func vipShopSKU(cfg db.VIPShopProduct) string {
	return constUtil.VIPShopQuotePrefix + p6Digest(adminJSON(cfg))[:constUtil.LiveShopQuoteHashLength] + ":" + cfg.ID
}

// Read current operational content; retain the old first-view row solely as a
// compatibility witness for unversioned requests from an older client screen.
func loadP3VIPShop(tx *gorm.DB, id int64, product string, now time.Time) (db.VIPShopProduct, error) {
	var result db.VIPShopProduct
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&result, "id=?", product).Error; err != nil {
		return result, err
	}
	if result.ID != product || !result.Enabled || shopHasDirectStamina(result.Reward) || result.MinVIP < 1 || result.MinVIP > 12 || result.Cost <= 0 || result.Cost > math.MaxInt32 || result.Source == "" {
		return result, errors.New(constUtil.P3PaymentConfigError)
	}
	key := "vip-shop:" + product + ":" + p3Day(now)
	var frozen db.PaymentActivityState
	err := tx.First(&frozen, "player_id=? AND activity_id=?", id, key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = tx.Create(&db.PaymentActivityState{PlayerID: id, ActivityID: key, Snapshot: string(adminJSON(result))}).Error
	}
	return result, err
}
func vipShopQuotedProduct(tx *gorm.DB, id int64, sku string, now time.Time) (db.VIPShopProduct, error) {
	product := sku
	quoted := strings.HasPrefix(sku, constUtil.VIPShopQuotePrefix)
	if quoted {
		_, rest, _ := strings.Cut(sku, ":")
		_, product, _ = strings.Cut(rest, ":")
		if product == "" {
			return db.VIPShopProduct{}, constUtil.ErrShopQuoteChanged
		}
	}
	cfg, err := loadP3VIPShop(tx, id, product, now)
	if err != nil {
		return cfg, err
	}
	if quoted {
		if sku != vipShopSKU(cfg) {
			return cfg, constUtil.ErrShopQuoteChanged
		}
		return cfg, nil
	}
	var witness db.PaymentActivityState
	if err = tx.First(&witness, "player_id=? AND activity_id=?", id, "vip-shop:"+product+":"+p3Day(now)).Error; err != nil {
		return cfg, err
	}
	var old db.VIPShopProduct
	if json.Unmarshal([]byte(witness.Snapshot), &old) != nil || vipShopSKU(old) != vipShopSKU(cfg) {
		return cfg, constUtil.ErrShopQuoteChanged
	}
	return cfg, nil
}
func (s *GameService) p3VIPShopList(tx *gorm.DB, p *pb.Player, now time.Time) ([]*pb.ShopItem, error) {
	var products []db.VIPShopProduct
	if err := tx.Where("enabled = ?", true).Order("id").Find(&products).Error; err != nil {
		return nil, err
	}
	_, _, paid, err := PaymentTotals(tx, p.GetPlayerId(), 0, now.UnixMilli()+1)
	if err != nil {
		return nil, err
	}
	var items []*pb.ShopItem
	for _, live := range products {
		if shopHasDirectStamina(live.Reward) {
			continue
		}
		cfg, e := loadP3VIPShop(tx, p.GetPlayerId(), live.ID, now)
		if e != nil {
			return nil, e
		}
		reward, e := p3Resources(cfg.Reward)
		if e != nil {
			return nil, e
		}
		bought, e := p3ClaimExists(tx, p.GetPlayerId(), "vip-shop:"+cfg.ID, p3Day(now))
		if e != nil {
			return nil, e
		}
		left := int32(1)
		if bought {
			left = 0
		}
		can := left
		if paid == 0 || int64(p.GetVip()) < cfg.MinVIP {
			can = 0
		}
		// 原会员商品卡直接读取 Price[0] 和 Discount[0]，两者都返回当前报价。
		// 当前目录无额外折扣；缺少 Discount 会使原 UI 在挂载商品前抛异常。
		price, e := s.p4ParseCost(fmt.Sprintf("%s,%d", vipShopCostResource(cfg), cfg.Cost))
		if e != nil {
			return nil, e
		}
		items = append(items, &pb.ShopItem{Id: proto.String(vipShopSKU(cfg)), Item: reward, Price: []*pb.BaseResource{price}, Discount: []*pb.BaseResource{proto.Clone(price).(*pb.BaseResource)}, ExchangeList: []*pb.BaseResource{proto.Clone(price).(*pb.BaseResource)}, CanBuy: proto.Int32(can), DayLeftTimes: proto.Int32(left), DayTotalTimes: proto.Int32(1), Condition: &pb.ShopCondition{Type: proto.String("vip"), Value: proto.Int64(cfg.MinVIP)}})
	}
	return items, nil
}

// vipShop 原商店shopType2入口；报价实时读取，每日购买记录仍使用稳定商品身份。
func (s *GameService) vipShop(ctx context.Context, id int64, request proto.Message, now time.Time) ([]proto.Message, error) {
	var response proto.Message
	err := s.p3Profile(ctx, id, func(tx *gorm.DB, row *db.Profile, p *pb.Player, g *model.GameState) error {
		if err := s.requireShopAvailability(tx, "vip", now); err != nil {
			return err
		}
		if _, ok := request.(*pb.CSShopProductList); ok {
			items, e := s.p3VIPShopList(tx, p, now)
			response = &pb.SCShopProductList{ShopType: proto.Int32(2), ShopItemList: items}
			return e
		}
		req, ok := request.(*pb.CSShopBuy)
		if !ok || req.GetShopType() != 2 || req.GetBuyCount() != 1 {
			return p3Error()
		}
		cfg, e := vipShopQuotedProduct(tx, id, req.GetShopItemId(), now)
		if e != nil {
			return e
		}
		_, _, paid, e := PaymentTotals(tx, id, 0, now.UnixMilli()+1)
		if e != nil {
			return e
		}
		if paid == 0 || int64(p.GetVip()) < cfg.MinVIP {
			return errors.New(constUtil.P3PaymentUnavailable)
		}
		cost, e := s.p4ParseCost(fmt.Sprintf("%s,%d", vipShopCostResource(cfg), cfg.Cost))
		if e != nil {
			return e
		}
		if e = s.p4Deduct(tx, id, p, g, cost, fmt.Sprintf("vip-shop:%s:%s", cfg.ID, p3Day(now)), now); e != nil {
			return e
		}
		gain, e := s.p3Claim(tx, p, g, "vip-shop:"+cfg.ID, p3Day(now), cfg.Reward, now)
		if e != nil {
			return e
		}
		if e = s.RecordP1Event(tx, id, fmt.Sprintf("vip-shop:%s:%s", cfg.ID, p3Day(now)), constUtil.P1EventPurchase, 1, now); e != nil {
			return e
		}
		if e = saveProfile(tx, row, p, g); e != nil {
			return e
		}
		items, e := s.p3VIPShopList(tx, p, now)
		if e != nil {
			return e
		}
		response = &pb.SCShopBuy{ShopType: proto.Int32(2), ShopItemList: items, Get: gain, Cost: []*pb.BaseResource{cost}}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if _, ok := request.(*pb.CSShopBuy); ok {
		return s.shopInventorySync(ctx, id, []proto.Message{response})
	}
	return []proto.Message{response}, nil
}
