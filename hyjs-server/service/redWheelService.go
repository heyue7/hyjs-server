package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// initializeRedWheel 建独立奖池和历史空间，通过原大转盘承载红武本地投放。
func (s *GameService) initializeRedWheel(tx *gorm.DB, zone *time.Location) error {
	cfg := db.GameplayPool{ID: constUtil.P2RedWheelPool, Name: "红武转盘（本地服）", Source: "原大转盘50/488钻、10000金币；本地红武奖位、权重、库存与周期，独立于限时神装", Enabled: true, MinLevel: 20, SingleCost: "diamond,50", TenCost: "diamond,488", FreeSeconds: 86400, FreePerDay: -1, CoinPerDraw: 10000, StartAt: time.Date(2026, 10, 13, 0, 0, 0, 0, zone).UnixMilli(), EndAt: time.Date(2026, 11, 13, 0, 0, 0, 0, zone).UnixMilli(), RewardEndAt: time.Date(2026, 11, 20, 0, 0, 0, 0, zone).UnixMilli(), InitialDiamondPool: 10000, JackpotPerMillion: 200, PoolContributionBps: 200}
	created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg)
	if created.Error != nil {
		return created.Error
	}
	if created.RowsAffected > 0 {
		for index, spec := range []struct {
			id     string
			amount int32
			weight int64
			stock  int32
		}{{"item_005", 20, 3000, 0}, {"item_008", 2, 2500, 0}, {"item_001", 2, 1800, 0}, {"item_020", 5, 1200, 0}, {"item_021", 10, 800, 0}, {"item_119", 1, 400, 100}, {"item_023", 1, 200, 100}, {"equip_037", 1, 80, 100}, {"equip_041", 1, 20, 50}} {
			if err := tx.Create(&db.GameplayPoolReward{PoolID: cfg.ID, ID: int32(index), ResourceID: spec.id, Amount: spec.amount, Weight: spec.weight, Stock: spec.stock}).Error; err != nil {
				return err
			}
		}
		rules := db.TurntableRules{PoolID: cfg.ID, ScorePerDraw: 1, TopPrize: "item_021,100", AdvancePrize: "item_021,20", AdvancePlaces: 10, Description: "本地兼容服红武投放，复用原大转盘。单抽50钻、十抽488钻；每24小时免费单抽，每抽10000金币。九奖位权重3000/2500/1800/1200/800/400/200/80/20；红武equip_037库存100、equip_041库存50，盒子各100，其他不限量。王者之魂/神装精华可经明确本地兑换目录换原红箱/红装备。大奖0.02%及2%钻石返池沿原表，排名奖本地配置。活动2026-10-13至11-13，展示至11-20，日期不会随重启延期；与20资质限时神装礼包分开。"}
		if err := tx.Create(&rules).Error; err != nil {
			return err
		}
	}
	if err := tx.First(&cfg, "id=?", constUtil.P2RedWheelPool).Error; err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.TurntableGlobal{PoolID: cfg.ID, DiamondPool: cfg.InitialDiamondPool, UsedStock: []byte("{}")}).Error
}

func (s *GameService) redExchangeItems(tx *gorm.DB, player *pb.Player, game *model.GameState) ([]*pb.ActivityShopItem, error) {
	var catalog []db.RedP2Exchange
	if err := tx.Where("enabled=?", true).Order("id").Find(&catalog).Error; err != nil {
		return nil, err
	}
	var items []*pb.ActivityShopItem
	day := time.Now().In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02")
	for _, cfg := range catalog {
		cost, e := parsePoolCost(cfg.Cost)
		if e != nil {
			return nil, e
		}
		if cfg.Amount < 1 || cfg.DailyLimit < 1 || cfg.Stock < 0 || cfg.Used < 0 {
			return nil, errors.New("红装兑换配置无效")
		}
		var st db.RedP2ExchangeState
		e = tx.Where("player_id=? AND product_id=? AND day=?", player.GetPlayerId(), cfg.ID, day).First(&st).Error
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return nil, e
		}
		left := max(0, cfg.DailyLimit-st.Bought)
		if player.GetLevel() < cfg.MinLevel {
			left = 0
		}
		if cfg.Stock > 0 {
			left = min(left, max(0, cfg.Stock-cfg.Used))
		}
		status := int32(1)
		if left == 0 {
			status = 2
		} else if simpleOwned(player, game, cost.GetId()) < int64(cost.GetNum()) {
			status = 0
		}
		items = append(items, &pb.ActivityShopItem{Status: proto.Int32(status), Name: proto.String("本地红色兑换"), Index: proto.Int32(cfg.ID), GetResource: []*pb.BaseResource{{Id: proto.String(cfg.ResourceID), Num: proto.Int32(cfg.Amount)}}, CostResource: []*pb.BaseResource{cost}, Total: proto.Int32(cfg.DailyLimit), Left: proto.Int32(left), PickOneOrGetAll: proto.Int32(0)})
	}
	return items, nil
}

func (s *GameService) redExchangeInfo(ctx context.Context, id int64) ([]proto.Message, error) {
	result := &pb.SCActivityShopInfo{ActivityId: proto.String(constUtil.P2RedExchangeActivity), Param: proto.String("本地兼容红卡红装兑换")}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var row db.Profile
		if e := tx.First(&row, id).Error; e != nil {
			return e
		}
		p, g, e := decodeProfileForCalendarUpdate(tx, &row)
		if e != nil {
			return e
		}
		var cfg db.GameplayPool
		if e = tx.First(&cfg, "id=?", constUtil.P2RedWheelPool).Error; e != nil {
			return e
		}
		result.StartTime = proto.Int64(cfg.StartAt)
		result.EndTime = proto.Int64(cfg.EndAt)
		result.ShopItemList, e = s.redExchangeItems(tx, p, g)
		return e
	})
	return []proto.Message{result}, err
}

func (s *GameService) redExchangeBuy(ctx context.Context, id int64, b *pb.CSGetActivityShopReward, stamp int64) ([]proto.Message, error) {
	result := &pb.SCGetActivityShopReward{}
	err := s.redP2Mutation(ctx, id, constUtil.P2ExchangeOperation, stamp, b, result, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		return s.redExchangeApply(tx, id, b, stamp, result, p, g)
	})
	return []proto.Message{result}, err
}

// redExchangeApply reuses the original P2 deduction, stock and daily state in the caller transaction.
// The legacy P2 wrapper continues to own its original receipt namespace.
func (s *GameService) redExchangeApply(tx *gorm.DB, id int64, b *pb.CSGetActivityShopReward, stamp int64, result *pb.SCGetActivityShopReward, p *pb.Player, g *model.GameState) error {
	var period db.GameplayPool
	if e := tx.First(&period, "id=?", constUtil.P2RedWheelPool).Error; e != nil {
		return e
	}
	now := time.Now().UnixMilli()
	if !period.Enabled || now < period.StartAt || now >= period.EndAt {
		return errors.New("红色兑换活动未开放")
	}
	var cfg db.RedP2Exchange
	if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cfg, b.GetIndex()).Error; e != nil {
		return e
	}
	if !cfg.Enabled || p.GetLevel() < cfg.MinLevel || cfg.Amount < 1 || cfg.DailyLimit < 1 {
		return errors.New("红装兑换未开放")
	}
	cost, e := parsePoolCost(cfg.Cost)
	if e != nil {
		return e
	}
	if len(b.GetCostResources()) > 0 && (len(b.GetCostResources()) != 1 || !proto.Equal(b.GetCostResources()[0], cost)) {
		return errors.New("兑换价格已改变，请刷新")
	}
	if len(b.GetGetResources()) > 0 && (len(b.GetGetResources()) != 1 || b.GetGetResources()[0].GetId() != cfg.ResourceID || b.GetGetResources()[0].GetNum() != cfg.Amount) {
		return errors.New("兑换奖励选项已改变，请刷新")
	}
	day := time.Now().In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02")
	state := db.RedP2ExchangeState{PlayerID: id, ProductID: cfg.ID, Day: day}
	e = tx.First(&state, "player_id=? AND product_id=? AND day=?", id, cfg.ID, day).Error
	if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
		return e
	}
	if state.Bought < 0 || state.Bought >= cfg.DailyLimit || cfg.Stock < 0 || cfg.Used < 0 || (cfg.Stock > 0 && cfg.Used >= cfg.Stock) {
		return errors.New("今日兑换次数或库存不足")
	}
	if e = deductPoolCost(p, g, cost); e != nil {
		return e
	}
	gain, e := s.GrantFixedReward(p, g, fmt.Sprintf("%s,%d", cfg.ResourceID, cfg.Amount))
	if e != nil {
		return e
	}
	state.Bought++
	if e = tx.Save(&state).Error; e != nil {
		return e
	}
	if cfg.Stock > 0 {
		cfg.Used++
		if e = tx.Save(&cfg).Error; e != nil {
			return e
		}
	}
	if tx.Migrator().HasTable(&db.EventFact{}) {
		if e = s.RecordP1Cost(tx, id, fmt.Sprintf("p2:exchange:%d", stamp), []*pb.BaseResource{cost}, LocalPlayNow(tx.Statement.Context)); e != nil {
			return e
		}
	}
	result.Get = gain
	result.CostResources = []*pb.BaseResource{cost}
	result.ShopItemList, e = s.redExchangeItems(tx, p, g)
	return e
}

// 原 UIActivity 通过下划线前缀 limitExchange 进入23号兑换页，OnSailItem 使用原买入协议。
func (s *GameService) redExchangeTab(ctx context.Context, id int64) (*pb.ActivityTab, error) {
	tx := db.GetIns().WithContext(ctx)
	if !tx.Migrator().HasTable(&db.RedP2Exchange{}) {
		return nil, nil
	}
	var count int64
	if e := tx.Model(&db.RedP2Exchange{}).Where("enabled=?", true).Count(&count).Error; e != nil {
		return nil, e
	}
	if count == 0 {
		return nil, nil
	}
	var cfg db.GameplayPool
	if e := tx.First(&cfg, "id=?", constUtil.P2RedWheelPool).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, e
	}
	now := time.Now().UnixMilli()
	if !cfg.Enabled || now < cfg.StartAt || now >= cfg.EndAt {
		return nil, nil
	}
	var row db.Profile
	if e := tx.First(&row, id).Error; e != nil {
		return nil, e
	}
	p, _, e := decodeProfile(&row)
	if e != nil {
		return nil, e
	}
	if p.GetLevel() < 20 {
		return nil, nil
	}
	return &pb.ActivityTab{ActivityId: proto.String(constUtil.P2RedExchangeActivity), Name: proto.String("红卡红装兑换（本地服）"), Order: proto.Int32(40), Status: proto.Int32(0), Param: proto.String("原限时兑换页；本地目录/价格/日限")}, nil
}
