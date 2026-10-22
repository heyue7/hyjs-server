package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/elog"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// InitializeTurntable 只为新活动写种子，重启保留 SQL 调整、库存和玩家回执。
func (s *GameService) InitializeTurntable(ctx context.Context) error {
	if err := db.InitGameplayPoolSchema(ctx, db.GetIns()); err != nil {
		return err
	}
	if err := db.GetIns().WithContext(ctx).AutoMigrate(&db.TurntableGlobal{}, &db.TurntableState{}, &db.TurntableRules{}, &db.TurntableRecord{}, &db.TurntablePersonal{}, &db.TurntableDrawReceipt{}); err != nil {
		return err
	}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		zone := time.FixedZone("UTC+8", 8*3600)
		val := func(key string) string { return s.table("GameConfigure", "StaticName", key)["NameValue"] }
		number := func(key string) int64 { v, _ := strconv.ParseInt(val(key), 10, 64); return v }
		probability, err := strconv.ParseFloat(val("BigTurnBigRewardRate"), 64)
		if err != nil {
			return err
		}
		contribution, err := strconv.ParseFloat(val("BigTurnOneTimeReturn"), 64)
		if err != nil {
			return err
		}
		// 奖位权重、库存、首池、免费间隔、积分和排名奖励是本地服选择；日期固定且不滚动延期。
		cfg := db.GameplayPool{ID: "turntable_20261011", Name: "幸运转盘", Source: "APK单抽50/十抽488、每抽10000金币、200ppm大奖与2%返池；其余为本地SQL规则", Enabled: true, MinLevel: 1,
			SingleCost: "diamond," + strconv.FormatInt(number("BigTurnOneTimeCost"), 10), TenCost: "diamond," + strconv.FormatInt(number("BigTurnTenTimeCost"), 10),
			FreeSeconds: 86400, FreePerDay: -1, CoinPerDraw: int32(number("BigTurnOneTimeGetCoin")),
			StartAt: time.Date(2026, 10, 11, 0, 0, 0, 0, zone).UnixMilli(), EndAt: time.Date(2026, 11, 11, 0, 0, 0, 0, zone).UnixMilli(), RewardEndAt: time.Date(2026, 11, 18, 0, 0, 0, 0, zone).UnixMilli(),
			InitialDiamondPool: 10000, JackpotPerMillion: int32(math.Round(probability * 1000000)), PoolContributionBps: int32(math.Round(contribution * 10000))}
		created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected > 0 {
			resources := []string{"item_011", "item_010", "item_009", "item_008", "item_005", "item_016", "item_001", "equip_001", "equip_005"}
			amounts := []int32{5, 3, 2, 1, 20, 1, 2, 1, 1}
			weights := []int64{3000, 2500, 1800, 1200, 800, 400, 200, 80, 20}
			stocks := []int32{0, 0, 0, 0, 0, 1000, 500, 100, 50}
			for i, key := range resources {
				r := db.GameplayPoolReward{PoolID: cfg.ID, ID: int32(i), ResourceID: key, Amount: amounts[i], Weight: weights[i], Stock: stocks[i]}
				if err := tx.Create(&r).Error; err != nil {
					return err
				}
			}
			if err := tx.Create(&db.TurntableRules{PoolID: cfg.ID, ScorePerDraw: 1, TopPrize: "item_008,100", AdvancePrize: "item_008,20", AdvancePlaces: 10,
				Description: "本地服规则：每24小时可免费单抽一次；每抽获得10000金币及1积分，付费单抽50钻、十抽488钻。付费钻石的2%进入全服共享钻石池（小数累计）；每抽大奖率0.02%，中奖获得当时整个钻石池。九种普通奖励按SQL权重抽取，有限库存由全服共享，售罄移出普通奖池。积分相同按玩家ID升序；活动结束后自动冻结真实排名，第一名100极品精炼石，第2至10名20极品精炼石直接入库，重复查询不重复发奖。未参与玩家不上榜。活动为2026-10-11至2026-11-11（UTC+8），排名展示至2026-11-18。"}).Error; err != nil {
				return err
			}
		}
		// 全局行仅在首次创建时继承首池；已有运营配置也能安全补建迁移所需全局行。
		if err := tx.Where("id = ?", cfg.ID).First(&cfg).Error; err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.TurntableGlobal{PoolID: cfg.ID, DiamondPool: cfg.InitialDiamondPool, UsedStock: []byte("{}")}).Error
	})
	if err != nil {
		return err
	}
	if err := s.initializeTurntableScreenshotPool(ctx); err != nil {
		return err
	}
	// 服务重启时补结算已经结束的固定活动，离线赢家也能获得奖励。
	return s.SettleTurntable(ctx)
}

// initializeTurntableScreenshotPool 建立独立普通奖池版本；旧期库存、个人冻结及回执继续引用旧ID。
func (s *GameService) initializeTurntableScreenshotPool(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var existing db.GameplayPool
		if err := tx.First(&existing, "id=?", constUtil.TurntableScreenshotPool).Error; err == nil {
			// 只迁移本轮尚未发布的原partial种子；人工改过的奖位不会被启动覆盖，旧私期/红武ID不在此范围。
			if existing.Source == constUtil.TurntableScreenshotPartialSource || existing.Source == constUtil.TurntableScreenshotSQLPartialSource {
				if err := tx.Model(&db.GameplayPoolReward{}).Where("pool_id=? AND id=3 AND resource_id=? AND amount=? AND stock=?", existing.ID, "item_008", 1, 0).Updates(map[string]any{"resource_id": "pet_066", "amount": 1, "stock": 2}).Error; err != nil {
					return err
				}
				if err := tx.Model(&db.GameplayPoolReward{}).Where("pool_id=? AND id=4 AND resource_id=? AND amount=?", existing.ID, "item_005", 20).Updates(map[string]any{"resource_id": "item_027", "amount": 3}).Error; err != nil {
					return err
				}
				var rules db.TurntableRules
				if err := tx.First(&rules, "pool_id=?", existing.ID).Error; err != nil {
					return err
				}
				if err := tx.Model(&rules).Update("description", rules.Description+constUtil.TurntableScreenshotDescription).Error; err != nil {
					return err
				}
				if err := tx.Model(&existing).Update("source", constUtil.TurntableScreenshotSource).Error; err != nil {
					return err
				}
			}
			// 已有新源仅安全补建缺失全局行，不覆盖其库存、奖池或运营字段。
			return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.TurntableGlobal{PoolID: existing.ID, DiamondPool: existing.InitialDiamondPool, UsedStock: []byte("{}")}).Error
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var pool db.GameplayPool
		var rewards []db.GameplayPoolReward
		var rules db.TurntableRules
		// 原普通源关闭时仍可安全建关闭的新源；不通过只允许开放池的读取器重新打开运营开关。
		if err := tx.First(&pool, "id=?", "turntable_20261011").Error; err != nil {
			return err
		}
		if err := tx.Where("pool_id=?", pool.ID).Order("id").Find(&rewards).Error; err != nil {
			return err
		}
		if len(rewards) != 9 {
			return errors.New("截图普通源缺少原九个奖位")
		}
		if err := tx.First(&rules, "pool_id=?", pool.ID).Error; err != nil {
			return err
		}
		pool.ID = constUtil.TurntableScreenshotPool
		pool.Name = "幸运转盘"
		pool.Source = constUtil.TurntableScreenshotSource
		created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&pool)
		if created.Error != nil || created.RowsAffected == 0 {
			return created.Error
		}
		// 原Prefab的poss为顺时针0～8，第9位为独立钻石大奖；右上按用户最终选择发暗之幽梦兽。
		confirmed := map[int32]struct {
			resource string
			amount   int32
		}{0: {"item_340", 1}, 1: {"coin", 50000}, 2: {"item_001", 3}, 3: {"pet_066", 1}, 4: {"item_027", 3}, 5: {"item_138", 3}, 6: {"MineItem_079", 1}, 7: {"item_127", 3}, 8: {"item_139", 3}}
		for i := range rewards {
			rewards[i].PoolID = pool.ID
			if value, ok := confirmed[rewards[i].ID]; ok {
				rewards[i].ResourceID, rewards[i].Amount = value.resource, value.amount
			}
			// 图中分子/分母是剩余/总库存；沿既有全服池库存模型，不据此新增每日或个人限额。
			if rewards[i].ID == 0 {
				rewards[i].Stock = 3
			}
			if rewards[i].ID == 3 {
				rewards[i].Stock = 2
			}
			if rewards[i].ID == 6 {
				rewards[i].Stock = 8
			}
			if err := tx.Create(&rewards[i]).Error; err != nil {
				return err
			}
		}
		rules.PoolID = pool.ID
		rules.Description += constUtil.TurntableScreenshotDescription
		if err := tx.Create(&rules).Error; err != nil {
			return err
		}
		return tx.Create(&db.TurntableGlobal{PoolID: pool.ID, DiamondPool: pool.InitialDiamondPool, UsedStock: []byte("{}")}).Error
	})
}

// SettleTurntable 在固定结束时间后补齐排名发奖，可由启动和维护循环共同调用。
func (s *GameService) SettleTurntable(ctx context.Context) error {
	// 所有已建转盘独立按期结算；当前显示池切换不能搁置旧参与者排名奖励。
	var globals []db.TurntableGlobal
	if err := db.GetIns().WithContext(ctx).Order("pool_id").Find(&globals).Error; err != nil {
		return err
	}
	for _, candidate := range globals {
		err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := ServerGameClockGuardTransaction(tx); err != nil {
				return err
			}

			var global db.TurntableGlobal
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("pool_id=?", candidate.PoolID).First(&global).Error; err != nil {
				return err
			}
			if global.Settled {
				return nil
			}
			var current db.GameplayPool
			if err := tx.First(&current, "id=?", global.PoolID).Error; err != nil {
				return err
			}
			settleNow, err := localPlayCycleNow(tx, current.ID, time.Now())
			if err != nil {
				return err
			}
			if !current.Enabled || settleNow.UnixMilli() < current.EndAt {
				return nil
			}
			cfg, _, rules, err := s.turntableConfigForPool(tx, global.PoolID)
			if err != nil {
				return err
			}
			return s.settleTurntable(tx, &global, cfg, rules, settleNow.UnixMilli())
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// ServeTurntableSettlement 使用服务生命周期上下文定期结算，结束时及时停止 SQL 操作。
func (s *GameService) ServeTurntableSettlement(ctx context.Context) error {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.SettleTurntable(ctx); err != nil && ctx.Err() == nil {
				elog.Warn("turntable settlement failed", zap.Error(err))
			}
		}
	}
}

// turntableConfig 在操作事务内重新读 SQL，校验所有九个候选项，错误配置不会偶发扣费。
func (s *GameService) turntableConfig(tx *gorm.DB) (*db.GameplayPool, []db.GameplayPoolReward, *db.TurntableRules, error) {
	return s.turntableConfigForPool(tx, s.p2TurntablePoolID(tx))
}

func (s *GameService) turntableConfigForPool(tx *gorm.DB, key string) (*db.GameplayPool, []db.GameplayPoolReward, *db.TurntableRules, error) {
	var rules db.TurntableRules
	cfg, rewards, err := s.loadGameplayPool(tx, key)
	if err != nil {
		return nil, nil, nil, err
	}
	if err = tx.Where("pool_id = ?", cfg.ID).First(&rules).Error; err != nil {
		return nil, nil, nil, err
	}
	if err = s.validateTurntableConfiguration(cfg, rewards, &rules); err != nil {
		return nil, nil, nil, err
	}
	return cfg, rewards, &rules, nil
}

// validateTurntableConfiguration validates the original consumer contract without requiring the operating switch to be enabled.
// The same pure checks describe a closed draft and guard the actual draw transaction.
func (s *GameService) validateTurntableConfiguration(cfg *db.GameplayPool, rewards []db.GameplayPoolReward, rules *db.TurntableRules) error {
	if cfg == nil || rules == nil {
		return errors.New("转盘运营规则无效")
	}
	// Keep loadGameplayPool's static boundaries while leaving its enabled/read behavior unchanged.
	if cfg.MinLevel < 1 || cfg.CoinPerDraw < 0 || cfg.FreeSeconds < 1 || cfg.FreeSeconds > 31536000 || cfg.FreePerDay < -1 || cfg.GuaranteeEvery < 0 || cfg.GuaranteeEvery > 10000 || len(rewards) == 0 {
		return errors.New("奖池运营配置无效")
	}
	var total int64
	for _, reward := range rewards {
		if reward.Weight <= 0 || reward.Weight > 1000000000000 || reward.Amount <= 0 || reward.Stock < 0 || total > math.MaxInt64-reward.Weight {
			return errors.New("奖励数量或概率权重无效")
		}
		total += reward.Weight
	}

	// Pure personal wheels have no legacy global ranking; nonempty legacy
	// rules retain their complete original validation and settlement checks.
	hasLegacyRanking := rules.TopPrize != "" || rules.AdvancePrize != "" || rules.AdvancePlaces != 0
	if len(rewards) != 9 || cfg.StartAt <= 0 || cfg.EndAt <= cfg.StartAt || cfg.RewardEndAt < cfg.EndAt || cfg.JackpotPerMillion < 0 || cfg.JackpotPerMillion > 1000000 || cfg.PoolContributionBps < 0 || cfg.PoolContributionBps > 10000 || cfg.InitialDiamondPool < 0 || cfg.InitialDiamondPool > math.MaxInt32 || rules.ScorePerDraw <= 0 || hasLegacyRanking && (rules.AdvancePlaces < 1 || rules.AdvancePlaces > 1000) {
		return errors.New("转盘运营规则无效")
	}
	for index, value := range []string{cfg.SingleCost, cfg.TenCost} {
		cost, e := parsePoolCost(value)
		// 原界面直接读取APK价格，没有服务器价格同步字段，热配置只能保留相同显示值。
		prices := []int32{50, 488}
		if e != nil || cost.GetId() != "diamond" || cost.GetNum() != prices[index] || cfg.CoinPerDraw != 10000 {
			return errors.New("原客户端转盘仅支持单抽50钻、十抽488钻及每抽10000金币")
		}
	}
	for i, r := range rewards {
		if r.ID != int32(i) || !s.turntableResource(r.ResourceID) || (strings.HasPrefix(r.ResourceID, "equip_") && r.Amount > 10) || (strings.HasPrefix(r.ResourceID, "pet_") && r.Amount > 100) {
			return errors.New("转盘必须配置0至8九个有效奖位")
		}
	}
	if _, err := turntablePityRules(rules.PrizePityRules, cfg.JackpotPerMillion); err != nil {
		return err
	}
	if !hasLegacyRanking {
		return nil
	}
	for _, value := range []string{rules.TopPrize, rules.AdvancePrize} {
		cost, e := parsePoolCost(value)
		if e != nil || !s.turntableResource(cost.GetId()) || strings.HasPrefix(cost.GetId(), "equip_") || strings.HasPrefix(cost.GetId(), "pet_") {
			return errors.New("转盘排名奖励配置无效")
		}
	}
	return nil
}

func (s *GameService) turntableResource(key string) bool {
	if key == "coin" || key == "diamond" {
		return true
	}
	if strings.HasPrefix(key, "pet_") {
		return s.table("Pet", "id", key) != nil
	}
	if strings.HasPrefix(key, "MineItem_") {
		return s.table("MineSkill", "MineItemID", key) != nil
	}
	if strings.HasPrefix(key, "item_") {
		return s.table("Item", "itemId", key) != nil
	}
	if strings.HasPrefix(key, "equip_") {
		row := s.table("Equip", "id", key)
		if row == nil {
			return false
		}
		price, e := strconv.ParseInt(row["sellPrice"], 10, 32)
		return e == nil && price >= 0
	}
	return false
}

// turntableGain 让原精灵奖励复用P9实例创建；调用方已持有真实玩家行锁并在同一抽取事务内。
func (s *GameService) turntableGain(tx *gorm.DB, player *pb.Player, game *model.GameState, content string) (*pb.ResultObject, error) {
	if !strings.HasPrefix(content, "pet_") {
		return s.openServerGain(player, game, content)
	}
	parts := strings.Split(content, ",")
	if len(parts) != 2 || !s.turntableResource(parts[0]) {
		return nil, errors.New("转盘精灵奖励配置无效")
	}
	amount, err := strconv.ParseInt(parts[1], 10, 32)
	if err != nil || amount <= 0 || amount > 100 {
		return nil, errors.New("转盘精灵奖励数量无效")
	}
	pets, err := s.P9GrantPet(tx, player, parts[0], int32(amount))
	if err != nil {
		return nil, err
	}
	return &pb.ResultObject{Pets: pets}, nil
}

// turntableRanks 只纳入发生过抽奖且仍有真实角色的参与者，同积分按玩家ID稳定排序。
func (s *GameService) turntableRanks(tx *gorm.DB, global *db.TurntableGlobal, rules *db.TurntableRules) ([]*pb.BigTurnRankItem, error) {
	if turntablePersonalOwner(tx) > 0 || rules.TopPrize == "" && rules.AdvancePrize == "" && rules.AdvancePlaces == 0 {
		return []*pb.BigTurnRankItem{}, nil
	}
	if global.Settled {
		var ranks []*pb.BigTurnRankItem
		if err := json.Unmarshal(global.FinalRanks, &ranks); err != nil {
			return nil, err
		}
		return ranks, nil
	}
	var states []db.TurntableState
	if err := tx.Where("pool_id = ? AND score > 0", global.PoolID).Order("score DESC, player_id ASC").Find(&states).Error; err != nil {
		return nil, err
	}
	// Frozen legacy scores prevent subsequent private draws from entering the retired shared contest.
	if tx.Migrator().HasTable(&db.TurntablePersonal{}) {
		for i := range states {
			var personal db.TurntablePersonal
			err := tx.First(&personal, "pool_id=? AND player_id=?", global.PoolID, states[i].PlayerID).Error
			if err == nil {
				states[i].Score = personal.LegacyScore
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
		}
	}
	sort.SliceStable(states, func(i, j int) bool {
		if states[i].Score == states[j].Score {
			return states[i].PlayerID < states[j].PlayerID
		}
		return states[i].Score > states[j].Score
	})
	ranks := []*pb.BigTurnRankItem{}
	for _, st := range states {
		if st.Score <= 0 {
			continue
		}
		var row db.Profile
		if err := tx.First(&row, st.PlayerID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return nil, err
		}
		player, _, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return nil, err
		}
		if player.GetName() == "" {
			continue
		}
		rank := int32(len(ranks) + 1)
		reward := ""
		if rank == 1 {
			reward = rules.TopPrize
		} else if rank <= rules.AdvancePlaces {
			reward = rules.AdvancePrize
		}
		ranks = append(ranks, &pb.BigTurnRankItem{Pid: proto.Int64(st.PlayerID), Rank: proto.Int32(rank), PlayerName: proto.String(player.GetName()), Score: proto.Int32(st.Score), Rewards: proto.String(reward)})
	}
	return ranks, nil
}

// settleTurntable 持有全局锁时冻结排名与发奖，所有赢家存档与已结算标记一起提交。
func (s *GameService) settleTurntable(tx *gorm.DB, global *db.TurntableGlobal, cfg *db.GameplayPool, rules *db.TurntableRules, now int64) error {
	if global.Settled || now < cfg.EndAt {
		return nil
	}
	if turntablePersonalOwner(tx) > 0 || rules.TopPrize == "" && rules.AdvancePrize == "" && rules.AdvancePlaces == 0 {
		// No legacy ranking means no reward queue or rank snapshot is created.
		global.Settled = true
		global.FinalRanks = []byte("[]")
		return saveTurntableRuntimeGlobal(tx, global)
	}
	ranks, err := s.turntableRanks(tx, global, rules)
	if err != nil {
		return err
	}
	for _, rank := range ranks {
		if rank.GetRewards() == "" {
			continue
		}
		var row db.Profile
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, rank.GetPid()).Error; err != nil {
			return err
		}
		player, g, e := decodeProfileForCalendarUpdate(tx, &row)
		if e != nil {
			return e
		}
		if _, e = s.turntableGain(tx, player, g, rank.GetRewards()); e != nil {
			return e
		}
		if err = saveProfile(tx, &row, player, g); err != nil {
			return err
		}
	}
	global.FinalRanks, err = json.Marshal(ranks)
	if err != nil {
		return err
	}
	global.Settled = true
	return tx.Save(global).Error
}

// turntableRead 使用统一全局→玩家锁序；结束后的任意查询也会完成唯一一次真实排名发奖。
func (s *GameService) turntableRead(ctx context.Context, id int64, kind string) ([]proto.Message, error) {
	ctx = context.WithValue(ctx, constUtil.PersonalTurntableOwnerKey, id)
	var result proto.Message
	var synced *pb.Player
	var items []*pb.SimpleItem
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		runtime, err := s.turntableRuntimeGlobal(tx, s.p2TurntablePoolID(tx))
		if err != nil {
			return err
		}
		global := *runtime

		cfg, rewards, rules, err := s.turntableConfig(tx)
		if err != nil {
			return err
		}
		now := LocalPlayNow(ctx).UnixMilli()
		if err = s.settleTurntable(tx, &global, cfg, rules, now); err != nil {
			return err
		}
		player, g, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		synced = player
		items = g.Items
		var state db.TurntableState
		if err = tx.Where("pool_id = ? AND player_id = ?", cfg.ID, id).First(&state).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		switch kind {
		case "info":
			stock := map[int32]int32{}
			if err = json.Unmarshal(global.UsedStock, &stock); err != nil {
				return err
			}
			items := []*pb.BigTurnRewardItem{}
			for _, r := range rewards {
				used := stock[r.ID]
				if used < 0 {
					return errors.New("转盘库存存档无效")
				}
				surplus := int32(0)
				if r.Stock > 0 {
					surplus = max(0, r.Stock-used)
				}
				items = append(items, &pb.BigTurnRewardItem{Id: proto.Int32(r.ID), Item: &pb.SimpleItem{Id: proto.String(r.ResourceID), Num: proto.Int32(r.Amount)}, Surplus: proto.Int32(surplus), AllCount: proto.Int32(r.Stock)})
			}
			records, e := turntableRecords(tx, cfg.ID)
			if e != nil {
				return e
			}
			free := int32(0)
			if player.GetLevel() >= cfg.MinLevel && now >= cfg.StartAt && now < cfg.EndAt && !global.Settled && turntableFree(cfg, &state, LocalPlayNow(ctx)) {
				free = 1
			}
			result = &pb.SCBigTurnTableInfo{BoughtCoins: proto.Int64(state.BoughtCoins), DiamondPool: proto.Int64(global.DiamondPool), RewardItems: items, IsFree: proto.Int32(free), EndTime: proto.Int64(cfg.EndAt), NextFreeTime: proto.Int64(state.NextFree), Score: proto.Int32(state.Score), OtherPlayerRewards: records, RewardEndTime: proto.Int64(cfg.RewardEndAt), ServerTime: proto.Int64(now)}
		case "rank":
			ranks, e := s.turntableRanks(tx, &global, rules)
			if e != nil {
				return e
			}
			result = &pb.SCBigTurnRank{Ranks: ranks}
		case "record":
			records, e := turntableRecords(tx, cfg.ID)
			if e != nil {
				return e
			}
			result = &pb.SCBigTurnRecord{OtherPlayerRewards: records}
		case "desc":
			result = &pb.SCBigTurnDesc{DescriStr: proto.String("个人幸运转盘：每人独立拥有奖品库存、钻石大奖池和抽奖记录。单抽50钻、十抽488钻，每抽赠送10000金币；免费机会以界面倒计时为准。付费返入本人大奖池，中奖领取本人池中钻石。库存售罄的奖品退出本人奖池；不参与全服排名，也不发放排名奖。"), TopPrizeId: proto.String(""), AdvancePrizeId: proto.String("")}
		default:
			return errors.New("未知转盘查询")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// 查询回调没有 AddResultObj，完整余额可同步结束时直接入库的排名奖励。
	return []proto.Message{result, &pb.SCItemList{ItemList: items}, &pb.SCPlayer{Player: synced}}, nil
}

func (s *GameService) bigTurnInfo(ctx context.Context, id int64) ([]proto.Message, error) {
	return s.turntableRead(ctx, id, "info")
}
func (s *GameService) bigTurnRank(ctx context.Context, id int64) ([]proto.Message, error) {
	return s.turntableRead(ctx, id, "rank")
}
func (s *GameService) bigTurnRecord(ctx context.Context, id int64) ([]proto.Message, error) {
	return s.turntableRead(ctx, id, "record")
}
func (s *GameService) bigTurnDesc(ctx context.Context, id int64) ([]proto.Message, error) {
	return s.turntableRead(ctx, id, "desc")
}

// turntableActivityList 按有效窗口公布转盘、基金与累计登录入口。
func (s *GameService) turntableActivityList(ctx context.Context, id int64) ([]proto.Message, error) {
	ctx = context.WithValue(ctx, constUtil.PersonalTurntableOwnerKey, id)
	result := &pb.SCActivityList{}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		var cfg db.GameplayPool
		if err := tx.Where("id = ?", s.p2TurntablePoolID(tx)).First(&cfg).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			// 未配置转盘是合法运营状态，其余活动仍继续组装。
			return nil
		} else if err != nil {
			return err
		}
		now := LocalPlayNow(ctx).UnixMilli()
		if !cfg.Enabled || now < cfg.StartAt || now >= cfg.RewardEndAt {
			return nil
		}
		player, _, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		if player.GetLevel() < cfg.MinLevel {
			return nil
		}
		// 展示只读实际奖位和规则，缺少或损坏的单个活动不曝光，也不补造奖励。
		var rewards []db.GameplayPoolReward
		if err := tx.Where("pool_id=?", cfg.ID).Order("id").Find(&rewards).Error; err != nil {
			return err
		}
		var rules db.TurntableRules
		if err := tx.First(&rules, "pool_id=?", cfg.ID).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		} else if err != nil {
			return err
		}
		if err := s.validateTurntableConfiguration(&cfg, rewards, &rules); err != nil {
			elog.Warn("activity omitted from directory", zap.String("activity", cfg.ID), zap.Error(err))
			return nil
		}
		runtime, err := s.turntableRuntimeGlobal(tx, cfg.ID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		} else if err != nil {
			return err
		}
		global := *runtime
		stock := map[int32]int32{}
		if global.DiamondPool < 0 || json.Unmarshal(global.UsedStock, &stock) != nil || stock == nil {
			return nil
		}
		for _, used := range stock {
			if used < 0 {
				return nil
			}
		}
		result.ActivityList = []*pb.ActivityTab{{ActivityId: proto.String("bigTurnTable_20261011"), Status: proto.Int32(0), Order: proto.Int32(1), Name: proto.String(cfg.Name), Param: proto.String("")}}
		result.Information = proto.String(rules.Description)
		return nil
	})
	if err != nil {
		return nil, err
	}
	activities, err := s.AvailableActivities(ctx, id)
	if err != nil {
		return nil, err
	}
	result.ActivityList = append(result.ActivityList, activities...)
	redExchange, err := s.redExchangeTab(ctx, id)
	if err != nil {
		return nil, err
	}
	if redExchange != nil {
		covered, err := s.P4ResourceExchangeCoversRed(ctx, id, LocalPlayNow(ctx))
		if err != nil {
			return nil, err
		}
		if !covered {
			result.ActivityList = append(result.ActivityList, redExchange)
		}
	}
	return []proto.Message{result}, nil
}

func turntableFree(cfg *db.GameplayPool, st *db.TurntableState, now time.Time) bool {
	day := now.In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02")
	used := st.FreeUsed
	if st.FreeDay != day {
		used = 0
	}
	return now.UnixMilli() >= st.NextFree && (cfg.FreePerDay < 0 || used < cfg.FreePerDay)
}

func turntableRecords(tx *gorm.DB, key string) ([]string, error) {
	var records []db.TurntableRecord
	query := tx.Where("pool_id = ?", key)
	if owner := turntablePersonalOwner(tx); owner > 0 {
		query = query.Where("player_id=?", owner)
	}
	if err := query.Order("id DESC").Limit(100).Find(&records).Error; err != nil {
		return nil, err
	}
	result := []string{}
	// 两个原客户端页面都会反转列表；服务端将最近100条按旧到新发送。
	for index := len(records) - 1; index >= 0; index-- {
		r := records[index]
		kind := "1"
		if r.Jackpot {
			kind = "0"
		}
		name := strings.NewReplacer(";", " ", ",", " ", "\n", " ", "\r", " ").Replace(r.PlayerName)
		date := time.UnixMilli(r.CreatedAtMillis).In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02 15:04:05")
		result = append(result, fmt.Sprintf("%s;%s;%s,%d;%s", kind, name, r.ResourceID, r.Amount, date))
	}
	return result, nil
}

// playBigTurn 将扣费、奖池返还、有限库存、奖励实例、积分和防重回执放在同一 SQL 事务。
func (s *GameService) playBigTurnOnce(ctx context.Context, id int64, b *pb.CSPlayBigTurn, stamp int64) ([]proto.Message, error) {
	ctx = context.WithValue(ctx, constUtil.PersonalTurntableOwnerKey, id)
	var result *pb.SCPlayBigTurn
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		player, g, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		prior, err := turntableReplayFrozen(tx, id, b, stamp)
		if err != nil {
			return err
		}
		if prior != nil {
			if err := s.ValidateCopyAwardReceiptCurrent(tx, id, g, &pb.SCCopyFightEnd{Gain: prior.GetRewards()}); err != nil {
				return err
			}
			result = prior
			return nil
		}
		runtime, err := s.turntableRuntimeGlobal(tx, s.p2TurntablePoolID(tx))
		if err != nil {
			return err
		}
		global := *runtime
		st := db.TurntableState{PoolID: global.PoolID, PlayerID: id}
		if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&st).Error; err != nil {
			return err
		}
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("pool_id = ? AND player_id = ?", global.PoolID, id).First(&st).Error; err != nil {
			return err
		}
		// 先检查已提交回执，活动关闭或 SQL 调整后仍能返回原抽取内容。
		if stamp <= 0 || stamp < st.Timestamp {
			return errors.New("转盘请求时间戳已过期")
		}
		if stamp == st.Timestamp && len(st.Response) > 0 {
			if st.RequestCount != b.GetCount() {
				return errors.New("转盘请求时间戳冲突")
			}
			result = &pb.SCPlayBigTurn{}
			if err := proto.Unmarshal(st.Response, result); err != nil {
				return err
			}
			return s.ValidateCopyAwardReceiptCurrent(tx, id, g, &pb.SCCopyFightEnd{Gain: result.GetRewards()})
		}
		cfg, rewards, rules, err := s.turntableConfig(tx)
		if err != nil {
			return err
		}
		now := LocalPlayNow(ctx)
		count := b.GetCount()
		if count != 1 && count != 10 {
			return errors.New("转盘次数无效")
		}
		if now.UnixMilli() < cfg.StartAt || now.UnixMilli() >= cfg.EndAt || global.Settled {
			return errors.New("转盘抽奖活动未开始或已结束")
		}
		if player.GetLevel() < cfg.MinLevel {
			return errors.New("转盘等级不足")
		}
		if global.DiamondPool < 0 || global.DiamondPool > math.MaxInt32 || global.ContributionRemainder < 0 || global.ContributionRemainder >= 10000 || st.Score < 0 || st.BoughtCoins < 0 {
			return errors.New("转盘数值存档无效")
		}
		stock := map[int32]int32{}
		if err = json.Unmarshal(global.UsedStock, &stock); err != nil {
			return err
		}
		// 先确认普通奖池足够完成整包请求，避免售罄时通过免费重试将大奖变成唯一结果。
		availableCount := int64(0)
		unlimited := false
		for _, r := range rewards {
			used := stock[r.ID]
			if used < 0 {
				return errors.New("转盘库存存档无效")
			}
			if r.Stock == 0 {
				unlimited = true
			} else {
				availableCount += int64(max(0, r.Stock-used))
			}
		}
		if !unlimited && availableCount < int64(count) {
			return errors.New("转盘普通奖池库存不足")
		}
		result = &pb.SCPlayBigTurn{Count: proto.Int32(count), IsDiamondPool: proto.Int32(0), Rewards: &pb.ResultObject{}}
		free := count == 1 && turntableFree(cfg, &st, now)
		if free {
			day := now.In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02")
			if st.FreeDay != day {
				st.FreeUsed = 0
				st.FreeDay = day
			}
			if st.FreeUsed == math.MaxInt32 {
				return errors.New("转盘免费次数溢出")
			}
			st.FreeUsed++
			st.NextFree = now.UnixMilli() + cfg.FreeSeconds*1000
		} else {
			price := cfg.SingleCost
			if count == 10 {
				price = cfg.TenCost
			}
			cost, e := parsePoolCost(price)
			if e != nil {
				return e
			}
			if e = deductPoolCost(player, g, cost); e != nil {
				return e
			}
			result.Cost = append(result.Cost, cost)
			contribution := int64(cost.GetNum())*int64(cfg.PoolContributionBps) + global.ContributionRemainder
			if global.DiamondPool > math.MaxInt32-contribution/10000 {
				return errors.New("转盘钻石池溢出")
			}
			global.DiamondPool += contribution / 10000
			global.ContributionRemainder = contribution % 10000
		}
		coins := int64(cfg.CoinPerDraw) * int64(count)
		if coins > math.MaxInt32 || st.BoughtCoins > math.MaxInt64-coins || int64(st.Score)+int64(rules.ScorePerDraw)*int64(count) > math.MaxInt32 {
			return errors.New("转盘积分或金币溢出")
		}
		if coins > 0 {
			gain, e := s.turntableGain(tx, player, g, fmt.Sprintf("coin,%d", coins))
			if e != nil {
				return e
			}
			result.Rewards.Simps = append(result.Rewards.Simps, gain.Simps...)
		}

		pityRules, err := turntablePityRules(rules.PrizePityRules, cfg.JackpotPerMillion)
		if err != nil {
			return err
		}
		pityEnabled := false
		for _, rule := range pityRules {
			pityEnabled = pityEnabled || rule.Enabled
		}
		pityProgress := map[int32]turntablePityProgress{}
		if st.PrizePityState != "" {
			if err = json.Unmarshal([]byte(st.PrizePityState), &pityProgress); err != nil || pityProgress == nil {
				return errors.New(constUtil.TurntablePityStateInvalid)
			}
		}
		turntablePityPauseDisabled(pityRules, pityProgress)
		for n := int32(0); n < count; n++ {
			selectedPity := int32(-1)
			if pityEnabled {
				dice, e := rand.Int(rand.Reader, big.NewInt(1000000))
				if e != nil {
					return e
				}
				selectedPity, e = turntablePityDraw(pityRules, pityProgress, rewards, stock, global.DiamondPool, cfg.JackpotPerMillion, dice.Int64())
				if e != nil {
					return e
				}
			}
			jackpot := selectedPity == 9
			if !pityEnabled && global.DiamondPool > 0 && cfg.JackpotPerMillion > 0 {
				dice, e := rand.Int(rand.Reader, big.NewInt(1000000))
				if e != nil {
					return e
				}
				jackpot = dice.Int64() < int64(cfg.JackpotPerMillion)
			}
			resource := ""
			amount := int32(0)
			rewardID := int32(9)
			if jackpot {
				resource = "diamond"
				amount = int32(global.DiamondPool)
				global.DiamondPool = 0
				result.IsDiamondPool = proto.Int32(1)
			} else {
				available := []db.GameplayPoolReward{}
				for _, r := range rewards {
					used := stock[r.ID]
					if used < 0 {
						return errors.New("转盘库存存档无效")
					}
					excluded := false
					if pityEnabled {
						for _, rule := range pityRules {
							if rule.Enabled && rule.PrizeID == r.ID {
								excluded = true
							}
						}
					}
					if (r.Stock == 0 || used < r.Stock) && (!excluded || selectedPity == r.ID) && (selectedPity < 0 || selectedPity == r.ID) {
						available = append(available, r)
					}
				}
				chosen, e := choosePoolReward(available, false)
				if e != nil {
					return errors.New("转盘普通奖池库存不足")
				}
				resource = chosen.ResourceID
				amount = chosen.Amount
				rewardID = chosen.ID
				if chosen.Stock > 0 {
					stock[chosen.ID]++
				}
			}
			if strings.HasPrefix(resource, "equip_") && int64(len(g.Equipment))+int64(amount) > int64(player.GetEquipBagSize()) {
				return errors.New("装备背包已满")
			}
			// 复用公共固定奖励消费者，MineItem进入真实宝石库存；旧金币/道具/装备沿原分支。
			gain, e := s.turntableGain(tx, player, g, fmt.Sprintf("%s,%d", resource, amount))
			if e != nil {
				return e
			}
			result.Rewards.Simps = append(result.Rewards.Simps, gain.Simps...)
			result.Rewards.Equips = append(result.Rewards.Equips, gain.Equips...)
			result.Rewards.Pets = append(result.Rewards.Pets, gain.Pets...)
			// 十抽列表含每次真实奖励，基础金币由客户端标题单独显示，大奖使用原界面图标别名。
			displayID := resource
			if jackpot {
				displayID = "diamondBigReward"
			}
			result.IdNumStr = append(result.IdNumStr, fmt.Sprintf("%s,%d", displayID, amount))
			result.RewardItemId = proto.Int32(rewardID)
			if e = tx.Create(&db.TurntableRecord{PoolID: cfg.ID, PlayerID: id, PlayerName: player.GetName(), ResourceID: resource, Amount: amount, Jackpot: jackpot, CreatedAtMillis: now.UnixMilli()}).Error; e != nil {
				return e
			}
		}
		if pityEnabled || len(pityProgress) > 0 {
			raw, e := json.Marshal(pityProgress)
			if e != nil {
				return e
			}
			st.PrizePityState = string(raw)
		}
		st.PlayerName = player.GetName()
		st.Score += rules.ScorePerDraw * count
		st.BoughtCoins += coins
		st.Timestamp = stamp
		st.RequestCount = count
		st.Response, err = proto.Marshal(result)
		if err != nil {
			return err
		}
		if err = localPlayTurntableSaveReceipt(tx, id, b, stamp, result); err != nil {
			return err
		}
		if err = tx.Create(&db.TurntableDrawReceipt{PlayerID: id, Stamp: stamp, PoolID: cfg.ID, Count: count, Response: st.Response}).Error; err != nil {
			return err
		}
		// 记录回执中的实际钻石费用；同次大奖返钻不能缩减消费事实。
		if len(result.Cost) > 0 && tx.Migrator().HasTable(&db.EventFact{}) {
			if err = s.RecordP1Cost(tx, id, fmt.Sprintf("p2:wheel:%s:%d", cfg.ID, stamp), result.Cost, now); err != nil {
				return err
			}
		}
		global.UsedStock, err = json.Marshal(stock)
		if err != nil {
			return err
		}
		if err = saveProfile(tx, &row, player, g); err != nil {
			return err
		}
		if err = tx.Save(&st).Error; err != nil {
			return err
		}
		return saveTurntableRuntimeGlobal(tx, &global)
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{result}, nil
}
