package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// InitializeP2 只首次写入兼容服规则，保留运营修改及历史转盘库存/记录。
func (s *GameService) InitializeP2(ctx context.Context) error {
	if err := db.GetIns().WithContext(ctx).AutoMigrate(&db.KingRecruitRules{}, &db.KingRecruitState{}, &db.RedP2Receipt{}, &db.RedP2Exchange{}, &db.RedP2ExchangeState{}, &db.RedP2BoxReward{}, &db.TurntableGlobal{}, &db.TurntableRules{}, &db.TurntableState{}, &db.TurntableRecord{}); err != nil {
		return err
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		val := func(key string) int64 {
			return int64(tableNumber(s.table("GameConfigure", "StaticName", key), "NameValue"))
		}
		cfg := db.GameplayPool{ID: "recruit_3", Name: "王者招募", Source: "APK价格/门槛/冷却/十抽保底；本地轮换阵营、每红卡权重400/橙卡100", Enabled: true, MinLevel: int32(val("WarriorRecruitCostType3Level")), SingleCost: s.table("GameConfigure", "StaticName", "WarriorRecruitCostType3")["NameValue"], TenCost: s.table("GameConfigure", "StaticName", "WarriorRecruitCostType3NumType2")["NameValue"], FreeSeconds: val("WarriorRecruitCostType3Time") * 3600, FreePerDay: -1, GuaranteeEvery: int32(val("WarriorRecruitCostType3Must")), CoinPerDraw: int32(val("BuyCoinCountOfRecruitType3"))}
		created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected > 0 {
			var index int32
			for _, hero := range s.Tables["Hero"] {
				if hero["heroClass"] != "0" || (hero["quality"] != "20" && hero["quality"] != "23") || (hero["space"] != "1" && hero["space"] != "2" && hero["space"] != "3") {
					continue
				}
				index++
				weight := int64(100)
				if hero["quality"] == "23" {
					weight = 400
				}
				if err := tx.Create(&db.GameplayPoolReward{PoolID: cfg.ID, ID: index, ResourceID: hero["id"], Amount: 1, Weight: weight, Guaranteed: hero["quality"] == "23"}).Error; err != nil {
					return err
				}
			}
		}
		zone := time.FixedZone("UTC+8", 8*3600)
		rules := db.KingRecruitRules{ID: 1, DailyLimit: int32(val("WarriorRecruitCostType3MaxTimes")), LuckLimit: int32(val("WarriorRecruitCostType3LuckyLimit")), LuckPerDraw: int32(val("WarriorRecruitCostType3LuckyPerValue")), RotationMillis: val("WarriorRecruitCostType3TimeSwitch") * 3600000, EpochMillis: time.Date(2026, 10, 13, 0, 0, 0, 0, zone).UnixMilli(), WheelPool: constUtil.P2RedWheelPool, Source: "APK上限50/幸运500/每抽10/48小时；本地1-3阵营轮换及幸运整卡选项"}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rules).Error; err != nil {
			return err
		}
		// 自建兑换目录默认关闭，奖励由后续运营配置；原红箱使用选项保留。
		for index, spec := range []struct{ id, cost string }{{"item_023", "item_020,400"}, {"item_119", "item_021,100"}, {"equip_037", "item_021,100"}, {"equip_038", "item_021,100"}, {"equip_039", "item_021,100"}, {"equip_040", "item_021,100"}} {
			cfg := db.RedP2Exchange{ID: int32(index + 1), ResourceID: spec.id, Amount: 1, Cost: spec.cost, DailyLimit: 1, Stock: 0, MinLevel: 20, Enabled: false, Source: "本地兼容服兑换目录、价格及每日1次；原item_020/021用途和红色ID"}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error; err != nil {
				return err
			}
		}
		// 原表随机箱 result 是候选列表，原服各候选概率未知；本地每箱单张、等权。
		for _, spec := range []struct {
			box    string
			heroes []string
		}{
			{"item_361", []string{"hero_001", "hero_002", "hero_065"}},
			{"item_362", []string{"hero_003", "hero_004", "hero_066"}},
			{"item_363", []string{"hero_005", "hero_006", "hero_067"}},
		} {
			for _, hero := range spec.heroes {
				cfg := db.RedP2BoxReward{BoxID: spec.box, ResourceID: hero, Amount: 1, Weight: 1, Source: "原随机红箱候选ID；本地单张等权，原服概率未知"}
				if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error; err != nil {
					return err
				}
			}
		}
		return s.initializeRedWheel(tx, zone)
	})
}

// redP2Mutation 在玩家锁内完成扣费、奖励、原始响应及回执，重放不会重复生成实例。
func (s *GameService) redP2Mutation(ctx context.Context, id int64, operation string, stamp int64, request, response proto.Message, action func(*gorm.DB, *pb.Player, *model.GameState) error) error {
	if stamp <= 0 {
		return errors.New("请求时间戳无效")
	}
	requestBytes, err := proto.MarshalOptions{Deterministic: true}.Marshal(request)
	if err != nil {
		return err
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		var receipt db.RedP2Receipt
		err := tx.Where("player_id=? AND operation=? AND timestamp=?", id, operation, stamp).First(&receipt).Error
		if err == nil {
			if !bytes.Equal(receipt.Request, requestBytes) {
				return errors.New("请求时间戳冲突")
			}
			if err := proto.Unmarshal(receipt.Response, response); err != nil {
				return err
			}
			// A retired or subsequently grown whole-warrior projection cannot overwrite current client inventory.
			var awarded *pb.ResultObject
			switch reply := response.(type) {
			case *pb.SCCombine:
				// 合成回包含完整实例；装备锻造、重置或回收后同样不能重放旧快照。
				_, game, err := decodeProfile(&row)
				if err != nil {
					return err
				}
				return s.validateP5GrowthReceipt(game, reply)
			case *pb.SCWarriorRecruitLuckyReward:
				awarded = reply.GetGet()
			}
			if len(awarded.GetWars()) > 0 {
				_, game, err := decodeProfile(&row)
				if err != nil {
					return err
				}
				for _, awarded := range awarded.GetWars() {
					current, err := p5Warrior(game, awarded.GetGuid())
					if err != nil || !proto.Equal(current, awarded) {
						return errors.New(constUtil.P5GrowthReceiptChanged)
					}
				}
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		player, game, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		if err = action(tx, player, game); err != nil {
			return err
		}
		if err = saveProfile(tx, &row, player, game); err != nil {
			return err
		}
		encoded, err := proto.Marshal(response)
		if err != nil {
			return err
		}
		return tx.Create(&db.RedP2Receipt{PlayerID: id, Operation: operation, Timestamp: stamp, Request: requestBytes, Response: encoded}).Error
	})
}

// DispatchP2 供统一入口先分发新增原协议；未匹配消息保留原业务路径。
func (s *GameService) DispatchP2(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	var messages []proto.Message
	var err error
	switch b := request.(type) {
	case *pb.CSWarriorRecruitGetPreview:
		messages, err = s.kingRecruitPreview(ctx, id, b)
	case *pb.CSWarriorRecruitLuckyReward:
		messages, err = s.kingRecruitLucky(ctx, id, b, stamp)
	case *pb.CSCombine:
		if !strings.HasPrefix(b.GetTargetId(), "hero_") && !strings.HasPrefix(b.GetTargetId(), "equip_") {
			return nil, false, nil
		}
		messages, err = s.redCombine(ctx, id, b, stamp)
	case *pb.CSWarriorFragGet:
		messages, err = s.redFragGet(ctx, id, b)
	case *pb.CSWarriorFragSell:
		messages, err = s.redFragSell(ctx, id, b, stamp)
	case *pb.CSActivityShopInfo:
		if b.GetActivityId() != constUtil.P2RedExchangeActivity {
			return nil, false, nil
		}
		messages, err = s.redExchangeInfo(ctx, id)
	case *pb.CSGetActivityShopReward:
		if b.GetActivityId() != constUtil.P2RedExchangeActivity {
			return nil, false, nil
		}
		messages, err = s.redExchangeBuy(ctx, id, b, stamp)
	default:
		return nil, false, nil
	}
	return messages, true, err
}

func (s *GameService) p2TurntablePoolID(tx *gorm.DB) string {
	if scoped := localPlayTurntablePoolID(tx); scoped != "" {
		return scoped
	}
	fallback := "turntable_20261011"
	if tx.Migrator().HasTable(&db.KingRecruitRules{}) {
		var cfg db.KingRecruitRules
		if tx.First(&cfg, 1).Error == nil && (cfg.WheelPool == constUtil.P2RedWheelPool || cfg.WheelPool == constUtil.TurntableScreenshotPool) {
			fallback = cfg.WheelPool
		}
	}
	key, err := s.AdminScheduledTurntablePool(tx, LocalPlayNow(tx.Statement.Context).UnixMilli(), fallback)
	if err != nil {
		return "invalid-admin-scheduled-wheel"
	}
	return key
}

func (s *GameService) kingConfiguration(tx *gorm.DB, now time.Time) (*db.KingRecruitRules, int32, int64, error) {
	var rules db.KingRecruitRules
	if err := tx.First(&rules, 1).Error; err != nil {
		return nil, 0, 0, err
	}
	if rules.DailyLimit != 50 || rules.LuckLimit != 500 || rules.LuckPerDraw != 10 || rules.RotationMillis != 172800000 {
		return nil, 0, 0, errors.New("王者运营规则与原客户端不匹配")
	}
	cycle := max(int64(0), (now.UnixMilli()-rules.EpochMillis)/rules.RotationMillis)
	return &rules, int32(cycle%3 + 1), rules.EpochMillis + (cycle+1)*rules.RotationMillis, nil
}

func kingState(tx *gorm.DB, id int64, now time.Time) (*db.KingRecruitState, error) {
	state := &db.KingRecruitState{PlayerID: id}
	err := tx.First(state, id).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err := growthRecruitKingDay(tx, state, now); err != nil {
		return nil, err
	}
	if state.Draws < 0 || state.Luck < 0 || state.Luck > 500 {
		return nil, errors.New("王者招募存档无效")
	}
	return state, nil
}

func (s *GameService) kingRewards(rewards []db.GameplayPoolReward, space int32) ([]db.GameplayPoolReward, []string) {
	var eligible []db.GameplayPoolReward
	var red []string
	for _, row := range rewards {
		hero := s.table("Hero", "id", row.ResourceID)
		if hero["space"] != strconv.Itoa(int(space)) {
			continue
		}
		eligible = append(eligible, row)
		if hero["quality"] == "23" {
			red = append(red, row.ResourceID)
		}
	}
	return eligible, red
}

func (s *GameService) kingRecruitPreview(ctx context.Context, id int64, b *pb.CSWarriorRecruitGetPreview) ([]proto.Message, error) {
	result := &pb.SCWarriorRecruitGetPreview{}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		_, rewards, err := s.loadGameplayPool(tx, fmt.Sprintf("recruit_%d", b.GetCostType()))
		if err != nil {
			return err
		}
		if b.GetCostType() == 3 {
			_, space, _, err := s.kingConfiguration(tx, LocalPlayNow(ctx))
			if err != nil {
				return err
			}
			rewards, _ = s.kingRewards(rewards, space)
		}
		for _, row := range rewards {
			result.Rewards = append(result.Rewards, row.ResourceID)
		}
		return nil
	})
	return []proto.Message{result}, err
}

func (s *GameService) kingRecruitLucky(ctx context.Context, id int64, b *pb.CSWarriorRecruitLuckyReward, stamp int64) ([]proto.Message, error) {
	result := &pb.SCWarriorRecruitLuckyReward{}
	err := s.redP2Mutation(ctx, id, constUtil.P2KingLuckyOperation, stamp, b, result, func(tx *gorm.DB, player *pb.Player, g *model.GameState) error {
		cfg, rewards, err := s.loadGameplayPool(tx, "recruit_3")
		if err != nil {
			return err
		}
		if player.GetLevel() < cfg.MinLevel {
			return errors.New("王者招募等级不足")
		}
		rules, space, _, err := s.kingConfiguration(tx, LocalPlayNow(ctx))
		if err != nil {
			return err
		}
		_, red := s.kingRewards(rewards, space)
		valid := false
		for _, key := range red {
			valid = valid || key == b.GetReward()
		}
		if !valid {
			return errors.New("幸运奖励不在当前展示卡池")
		}
		state, err := kingState(tx, id, LocalPlayNow(ctx))
		if err != nil {
			return err
		}
		if state.Luck < rules.LuckLimit {
			return errors.New("幸运值不足")
		}
		gain, err := s.GrantFixedReward(player, g, b.GetReward()+",1")
		if err != nil {
			return err
		}
		result.Get = gain
		state.Luck -= rules.LuckLimit
		return tx.Save(state).Error
	})
	return []proto.Message{result}, err
}
