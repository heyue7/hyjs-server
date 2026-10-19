package service

import (
	"context"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"math"
	"strconv"
	"strings"
	"time"
)

// InitializeShop 只创建首次目录，奖励和限购热改不会被重启覆盖。
func (s *GameService) InitializeShop(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.ShopProduct{}, &db.ShopState{}, &db.ShopReceipt{}); err != nil {
		return err
	}
	return tx.Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		// 未恢复原商品映射和份额的目录保留为关闭模板，等待后台配置。
		for i, seed := range []struct {
			id, resource, cost string
			amount             int32
		}{
			{"itemShopId_8", "item_006", "diamond,20", 1},
			{"itemShopId_9", "item_007", "diamond,30", 5},
			{"itemShopId_10", "item_008", "diamond,50", 1},
			{"itemShopId_11", "item_012", "diamond,15", 5},
			{"itemShopId_13", "item_005", "diamond,20", 1},
			{"itemShopId_14", "item_022", "diamond,5", 1},
			{"itemShopId_16", "item_024", "diamond,88", 1},
			{"itemShopId_12", constUtil.P5WantedItemID, "diamond,200", 1},
		} {
			cfg := db.ShopProduct{ID: seed.id, ResourceID: seed.resource, Amount: seed.amount, Cost: seed.cost, DailyLimit: 20, MinLevel: 1, Priority: int32(i + 1), Enabled: false, Source: "APK固定商品位价格；本地服材料/饰品箱目录、每份数量和日限购20次"}
			if seed.resource == constUtil.P5WantedItemID {
				cfg.DailyLimit, cfg.Source = 10, constUtil.P5WantedShopSource
				// 原悬赏购买回调已明确资源、商品位和固定价格，继续保留。
				cfg.Enabled = true
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// shopItem 校验整份配置后返回原客户端可显示的目录投影。
func (s *GameService) shopItem(tx *gorm.DB, player *pb.Player, cfg *db.ShopProduct, day string) (*pb.ShopItem, error) {
	if cfg.Amount < 1 || cfg.Amount > 10000 || cfg.DailyLimit < 1 || cfg.DailyLimit > 10000 || cfg.MinLevel < 1 || cfg.MinVIP < 0 || s.table("Item", "itemId", cfg.ResourceID) == nil {
		return nil, errors.New("商店目录配置无效")
	}
	cost, err := parsePoolCost(cfg.Cost)
	if err != nil {
		return nil, err
	}
	// 普通道具页从原表计算显示价格，只允许完整固定价格档，避免暗中多扣。
	var maxCount int64
	for _, row := range s.Tables["ItemShopPrice"] {
		if row["shopItemId"] != cfg.ID {
			continue
		}
		if row["price"] != cfg.Cost {
			return nil, errors.New("当前客户端仅支持原表固定商品价格")
		}
		rangeParts := strings.Split(row["count"], "-")
		last, e := strconv.ParseInt(rangeParts[len(rangeParts)-1], 10, 32)
		if e != nil {
			return nil, e
		}
		if last > maxCount {
			maxCount = last
		}
	}
	if int64(cfg.DailyLimit) > maxCount {
		return nil, errors.New("商店限购超出原客户端价格档")
	}
	var state db.ShopState
	if err = tx.Where("player_id = ? AND product_id = ? AND day = ?", player.GetPlayerId(), cfg.ID, day).First(&state).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if state.Bought < 0 {
		return nil, errors.New("商店购买存档无效")
	}
	left := max(0, cfg.DailyLimit-state.Bought)
	canBuy := left
	if player.GetLevel() < cfg.MinLevel || player.GetVip() < cfg.MinVIP {
		canBuy = 0
	}
	return &pb.ShopItem{Id: proto.String(cfg.ID), Item: []*pb.BaseResource{{Id: proto.String(cfg.ResourceID), Num: proto.Int32(cfg.Amount)}}, Price: []*pb.BaseResource{cost}, ExchangeList: []*pb.BaseResource{cost}, Priority: proto.Int32(cfg.Priority), CanBuy: proto.Int32(canBuy), DayLeftTimes: proto.Int32(left), DayTotalTimes: proto.Int32(cfg.DailyLimit), Condition: &pb.ShopCondition{Type: proto.String("level"), Value: proto.Int64(int64(cfg.MinLevel))}}, nil
}

func (s *GameService) shopList(tx *gorm.DB, player *pb.Player, day string) ([]*pb.ShopItem, error) {
	if err := s.requireShopAvailability(tx, "ordinary", LocalPlayNow(tx.Statement.Context)); err != nil {
		if errors.Is(err, constUtil.ErrP4CommerceNotOpen) {
			return []*pb.ShopItem{}, nil
		}
		return nil, err
	}
	var products []db.ShopProduct
	if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("enabled = ?", true).Order("priority,id").Find(&products).Error; err != nil {
		return nil, err
	}
	result := []*pb.ShopItem{}
	for i := range products {
		item, err := s.shopItem(tx, player, &products[i], day)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if s.P4Ready {
		extra, err := s.p4OrdinaryProjection(tx, player, LocalPlayNow(tx.Statement.Context))
		if err != nil {
			return nil, err
		}
		result = append(result, extra...)
	}
	return result, nil
}

// shopProducts 查询只读取每日计数，访问页面不会产生购买或奖励。
func (s *GameService) shopProducts(ctx context.Context, id int64, b *pb.CSShopProductList) ([]proto.Message, error) {
	result := &pb.SCShopProductList{ShopType: proto.Int32(b.GetShopType())}
	if b.GetShopType() != 1 {
		return []proto.Message{result}, nil
	}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var row db.Profile
		if err := tx.First(&row, id).Error; err != nil {
			return err
		}
		player, _, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		if err := s.requireShopAvailability(tx, "ordinary", LocalPlayNow(ctx)); err != nil {
			return err
		}
		day := time.Now().In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02")
		result.ShopItemList, err = s.shopList(tx, player, day)
		return err
	})
	return []proto.Message{result}, err
}

// shopBuy 将扣费、资源入库、日限购和重试回执放在一个行锁事务。
func (s *GameService) shopBuy(ctx context.Context, id int64, b *pb.CSShopBuy, stamp int64) ([]proto.Message, error) {
	if b.GetShopType() != 1 || b.GetBuyCount() < 1 || b.GetBuyCount() > 10000 || b.GetShopItemId() == "" || stamp < 1 {
		return nil, errors.New("商店购买参数无效")
	}
	var result *pb.SCShopBuy
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		player, game, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		var receipt db.ShopReceipt
		if err = tx.First(&receipt, "player_id = ?", id).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if receipt.Timestamp > stamp {
			return errors.New("商店购买请求已过期")
		}
		if receipt.Timestamp == stamp {
			var request pb.CSShopBuy
			if proto.Unmarshal(receipt.Request, &request) != nil || !proto.Equal(&request, b) {
				return errors.New("商店购买时间戳冲突")
			}
			result = &pb.SCShopBuy{}
			return proto.Unmarshal(receipt.Response, result)
		}
		var cfg db.ShopProduct
		if err = tx.Clauses(clause.Locking{Strength: "SHARE"}).First(&cfg, "id = ? AND enabled = ?", b.GetShopItemId(), true).Error; err != nil {
			return err
		}
		if err := s.requireShopAvailability(tx, "ordinary", LocalPlayNow(ctx)); err != nil {
			return err
		}
		day := time.Now().In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02")
		item, err := s.shopItem(tx, player, &cfg, day)
		if err != nil {
			return err
		}
		if item.GetCanBuy() < b.GetBuyCount() {
			return errors.New("商店等级、会员资格或每日购买次数不足")
		}
		cost, err := parsePoolCost(cfg.Cost)
		if err != nil {
			return err
		}
		total := int64(cost.GetNum()) * int64(b.GetBuyCount())
		amount := int64(cfg.Amount) * int64(b.GetBuyCount())
		if total > math.MaxInt32 || amount > math.MaxInt32 {
			return errors.New("商店数量超出协议范围")
		}
		cost.Num = proto.Int32(int32(total))
		if err = deductPoolCost(player, game, cost); err != nil {
			return err
		}
		gain, err := s.chapterBoxGain(player, game, fmt.Sprintf("%s,%d", cfg.ResourceID, amount))
		if err != nil {
			return err
		}
		state := db.ShopState{PlayerID: id, ProductID: cfg.ID, Day: day, Bought: cfg.DailyLimit - item.GetDayLeftTimes() + b.GetBuyCount()}
		if err = tx.Save(&state).Error; err != nil {
			return err
		}
		list, err := s.shopList(tx, player, day)
		if err != nil {
			return err
		}
		result = &pb.SCShopBuy{ShopType: proto.Int32(1), ShopItemList: list, Get: gain, Cost: []*pb.BaseResource{cost}}
		// 仅统计真正支付并到账的通缉令；回执重放在此分支之前返回。
		if cfg.ResourceID == "item_004" {
			if err = RecordOpenServerCounter(tx, id, constUtil.OpenServerActivityCounterBuyAttackToken, int32(amount)); err != nil {
				return err
			}
		}
		if err = saveProfile(tx, &row, player, game); err != nil {
			return err
		}
		receipt.PlayerID = id
		receipt.Timestamp = stamp
		if receipt.Request, err = proto.Marshal(b); err != nil {
			return err
		}
		if receipt.Response, err = proto.Marshal(result); err != nil {
			return err
		}
		return tx.Save(&receipt).Error
	})
	return []proto.Message{result}, err
}
