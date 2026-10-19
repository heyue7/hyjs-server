package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func loadP3VIPShop(tx *gorm.DB, id int64, product string, now time.Time) (db.VIPShopProduct, error) {
	var result db.VIPShopProduct
	key := "vip-shop:" + product + ":" + p3Day(now)
	var frozen db.PaymentActivityState
	err := tx.First(&frozen, "player_id = ? AND activity_id = ?", id, key).Error
	if err == nil {
		err = json.Unmarshal([]byte(frozen.Snapshot), &result)
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		if err = tx.First(&result, "id = ?", product).Error; err != nil {
			return result, err
		}
		raw, e := json.Marshal(result)
		if e != nil {
			return result, e
		}
		err = tx.Create(&db.PaymentActivityState{PlayerID: id, ActivityID: key, Snapshot: string(raw)}).Error
	}
	if err != nil {
		return result, err
	}
	if result.ID != product || !result.Enabled || result.MinVIP < 1 || result.MinVIP > 12 || result.Cost <= 0 || result.Cost > 100000 || result.Source == "" {
		return result, errors.New(constUtil.P3PaymentConfigError)
	}
	return result, nil
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
		// 原会员商品卡直接读取 Price[0] 和 Discount[0]，两者都返回当天冻结报价。
		// 当前目录无额外折扣；缺少 Discount 会使原 UI 在挂载商品前抛异常。
		price := &pb.BaseResource{Id: proto.String("diamond"), Num: proto.Int32(int32(cfg.Cost))}
		items = append(items, &pb.ShopItem{Id: proto.String(cfg.ID), Item: reward, Price: []*pb.BaseResource{price}, Discount: []*pb.BaseResource{proto.Clone(price).(*pb.BaseResource)}, ExchangeList: []*pb.BaseResource{proto.Clone(price).(*pb.BaseResource)}, CanBuy: proto.Int32(can), DayLeftTimes: proto.Int32(left), DayTotalTimes: proto.Int32(1), Condition: &pb.ShopCondition{Type: proto.String("vip"), Value: proto.Int64(cfg.MinVIP)}})
	}
	return items, nil
}

// vipShop 原商店shopType2入口；首次列表冻结当天价格、资格和奖励，一日一份。
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
		cfg, e := loadP3VIPShop(tx, id, req.GetShopItemId(), now)
		if e != nil {
			return e
		}
		_, _, paid, e := PaymentTotals(tx, id, 0, now.UnixMilli()+1)
		if e != nil {
			return e
		}
		if paid == 0 || int64(p.GetVip()) < cfg.MinVIP || p.GetDiamond() < cfg.Cost {
			return errors.New(constUtil.P3PaymentUnavailable)
		}
		p.Diamond = proto.Int64(p.GetDiamond() - cfg.Cost)
		gain, e := s.p3Claim(tx, p, g, "vip-shop:"+cfg.ID, p3Day(now), cfg.Reward, now)
		if e != nil {
			return e
		}
		if e = s.RecordP1Event(tx, id, fmt.Sprintf("vip-shop:%s:%s", cfg.ID, p3Day(now)), constUtil.P1EventDiamondSpent, cfg.Cost, now); e != nil {
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
		response = &pb.SCShopBuy{ShopType: proto.Int32(2), ShopItemList: items, Get: gain, Cost: []*pb.BaseResource{{Id: proto.String("diamond"), Num: proto.Int32(int32(cfg.Cost))}}}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}
