package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"
)

// InitializeGameplayPools 首次建立SQL运营配置，已有配置始终保留，不随重启覆盖。
func (s *GameService) InitializeGameplayPools(ctx context.Context) error {
	if err := db.InitGameplayPoolSchema(ctx, db.GetIns()); err != nil {
		return err
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		for _, kind := range []int32{1, 2} {
			prefix := fmt.Sprintf("WarriorRecruitCostType%d", kind)
			val := func(key string) string { return s.table("GameConfigure", "StaticName", key)["NameValue"] }
			number := func(key string) int64 { v, _ := strconv.ParseInt(val(key), 10, 64); return v }
			cfg := db.GameplayPool{ID: fmt.Sprintf("recruit_%d", kind), Name: "本地服普通招募", Source: "APK价格与冷却；本地服奖池权重和保底范围", Enabled: true, MinLevel: int32(number(prefix + "Level")), SingleCost: val(prefix), TenCost: val(prefix + "NumType2"), FreeSeconds: number(prefix+"Time") * 60, FreePerDay: int32(number(prefix + "FreeTimes")), GuaranteeEvery: int32(number(prefix + "Must")), CoinPerDraw: int32(number(fmt.Sprintf("BuyCoinCountOfRecruitType%d", kind)))}
			weights := map[string]int64{"12": 65, "13": 30, "18": 5}
			guaranteed := "18"
			if kind == 2 {
				cfg.Name = "本地服强者招募"
				cfg.FreeSeconds = number(prefix+"Time") * 3600
				cfg.FreePerDay = -1
				cfg.SingleTicket = val(prefix + "Item")
				cfg.TenTicket = val(prefix + "NumType2Item")
				weights = map[string]int64{"13": 15, "18": 75, "20": 10}
				guaranteed = "20"
			}
			created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg)
			if created.Error != nil {
				return created.Error
			}
			if created.RowsAffected == 0 {
				continue
			}
			// 每组总权重固定为配置百分比，同资质英雄等概率；不把展示战斗表当原服卡池。
			counts := map[string]int64{}
			for _, row := range s.Tables["Hero"] {
				if row["heroClass"] == "0" && weights[row["quality"]] > 0 {
					counts[row["quality"]]++
				}
			}
			// 使用各组人数的积精确保持组间比例，避免整数除法舍入。
			scale := int64(1)
			for _, n := range counts {
				scale *= n
			}
			idx := int32(0)
			for _, row := range s.Tables["Hero"] {
				q := row["quality"]
				if row["heroClass"] != "0" || weights[q] == 0 {
					continue
				}
				idx++
				reward := db.GameplayPoolReward{PoolID: cfg.ID, ID: idx, ResourceID: row["id"], Amount: 1, Weight: weights[q] * scale / counts[q], Guaranteed: q == guaranteed}
				if err := tx.Create(&reward).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// loadGameplayPool 在当前资源事务中读取配置，非法配置会让整次操作失败而不扣费。
func (s *GameService) loadGameplayPool(tx *gorm.DB, key string) (*db.GameplayPool, []db.GameplayPoolReward, error) {
	var cfg db.GameplayPool
	if err := tx.Where("id = ?", key).First(&cfg).Error; err != nil {
		return nil, nil, err
	}
	var rewards []db.GameplayPoolReward
	if err := tx.Where("pool_id = ?", key).Order("id").Find(&rewards).Error; err != nil {
		return nil, nil, err
	}
	if s.P4Ready && (key == "recruit_1" || key == "recruit_2") {
		kind := int32(1)
		if key == "recruit_2" {
			kind = 2
		}
		rotation, _, err := s.loadP4Rotation(tx, kind, LocalPlayNow(tx.Statement.Context))
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, err
		}
		// 限时轮换停用后继续使用常驻池，保留原招募入口和费用。
		if err == nil {
			rewards = rotation
		}
	}
	if !cfg.Enabled || cfg.MinLevel < 1 || cfg.CoinPerDraw < 0 || cfg.FreeSeconds < 1 || cfg.FreeSeconds > 31536000 || cfg.FreePerDay < -1 || cfg.GuaranteeEvery < 0 || cfg.GuaranteeEvery > 10000 || len(rewards) == 0 {
		return nil, nil, errors.New("奖池未开放或运营配置无效")
	}
	var total int64
	for _, v := range rewards {
		if v.Weight <= 0 || v.Weight > 1000000000000 || v.Amount <= 0 || v.Stock < 0 || total > math.MaxInt64-v.Weight {
			return nil, nil, errors.New("奖励数量或概率权重无效")
		}
		total += v.Weight
	}
	return &cfg, rewards, nil
}

// 原客户端没有下发价格字段，保留APK价格避免显示价格与实际扣费不一致。
// 奖励、权重、保底与冷却仍由SQL运营配置读取。
func (s *GameService) validateRecruitClientPrices(cfg *db.GameplayPool, kind int32) error {
	prefix := fmt.Sprintf("WarriorRecruitCostType%d", kind)
	value := func(key string) string { return s.table("GameConfigure", "StaticName", key)["NameValue"] }
	if cfg.SingleCost != value(prefix) || cfg.TenCost != value(prefix+"NumType2") || int64(cfg.CoinPerDraw) != int64(tableNumber(s.table("GameConfigure", "StaticName", fmt.Sprintf("BuyCoinCountOfRecruitType%d", kind)), "NameValue")) {
		return errors.New("当前客户端仅支持APK原招募价格与赠送金币，请恢复SQL价格配置")
	}
	if kind == 2 && (cfg.SingleTicket != value(prefix+"Item") || cfg.TenTicket != value(prefix+"NumType2Item")) {
		return errors.New("当前客户端仅支持APK原招募券配置")
	}
	if kind != 2 && (cfg.SingleTicket != "" || cfg.TenTicket != "") {
		return errors.New("普通招募券配置不匹配当前客户端")
	}
	if kind == 3 && (cfg.MinLevel != 20 || cfg.FreeSeconds != 172800 || cfg.FreePerDay != -1 || cfg.GuaranteeEvery != 10) {
		return errors.New("王者招募门槛、免费冷却或保底间隔与原客户端不匹配")
	}
	return nil
}

func recruitStoredState(g *model.GameState, kind int32) *model.RecruitState {
	if g.Recruit == nil {
		g.Recruit = map[int32]*model.RecruitState{}
	}
	state := g.Recruit[kind]
	if state == nil {
		state = &model.RecruitState{}
		g.Recruit[kind] = state
	}
	return state
}

func recruitState(g *model.GameState, kind int32, now time.Time) *model.RecruitState {
	state := recruitStoredState(g, kind)
	day := now.In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02")
	if state.Day != day {
		state.Day = day
		state.FreeUsed = 0
	}
	return state
}

func (s *GameService) recruitInfo(ctx context.Context, id int64, b *pb.CSWarriorRecruitGet) ([]proto.Message, error) {
	result := &pb.SCWarriorRecruitGet{}
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
		kinds := b.GetCostType()
		if len(kinds) == 0 {
			kinds = []int32{1, 2}
			var count int64
			if err := tx.Model(&db.GameplayPool{}).Where("id=?", "recruit_3").Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				kinds = append(kinds, 3)
			}
		}
		now := LocalPlayNow(ctx)
		for _, kind := range kinds {
			cfg, baseRewards, err := s.loadGameplayPool(tx, fmt.Sprintf("recruit_%d", kind))
			if err != nil {
				return err
			}
			if err := s.validateRecruitClientPrices(cfg, kind); err != nil {
				return err
			}
			st, err := growthRecruitState(tx, id, g, kind, now)
			if err != nil {
				return err
			}
			num := int32(0)
			if kind == 1 {
				num = max(0, cfg.FreePerDay-st.FreeUsed)
			} else if cfg.GuaranteeEvery > 0 {
				num = max(0, cfg.GuaranteeEvery-st.SinceGuarantee-1)
			}
			free := now.UnixMilli() >= st.NextFree && (cfg.FreePerDay < 0 || st.FreeUsed < cfg.FreePerDay)
			on := int32(0)
			if player.GetLevel() >= cfg.MinLevel {
				on = 1
			}
			entry := &pb.WarriorRecruit{CostType: proto.Int32(kind), FreeTime: proto.Int64(st.NextFree), Num: proto.Int32(num), Free: proto.Bool(free), On: proto.Int32(on)}
			if s.P4Ready && (kind == 1 || kind == 2) {
				rewards, change, err := s.loadP4Rotation(tx, kind, LocalPlayNow(tx.Statement.Context))
				if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				if errors.Is(err, gorm.ErrRecordNotFound) {
					rewards, change = baseRewards, 0
				}
				entry.ChangeTime = proto.Int64(change)
				for _, reward := range rewards {
					entry.RewardList = append(entry.RewardList, reward.ResourceID)
				}
			}
			if kind == 3 {
				rules, space, change, err := s.kingConfiguration(tx, now)
				if err != nil {
					return err
				}
				state, err := kingState(tx, id, now)
				if err != nil {
					return err
				}
				_, rewards, err := s.loadGameplayPool(tx, "recruit_3")
				if err != nil {
					return err
				}
				_, red := s.kingRewards(rewards, space)
				if len(red) == 0 {
					return errors.New("当前王者卡池缺少红卡")
				}
				entry.Luck = proto.Int32(state.Luck)
				entry.LeftTimes = proto.Int32(max(0, rules.DailyLimit-state.Draws))
				entry.Space = proto.Int32(space)
				entry.ChangeTime = proto.Int64(change)
				entry.RewardList = red
				entry.Reward1 = proto.String(red[0])
				if len(red) > 1 {
					entry.Reward2 = proto.String(red[1])
				}
				if len(red) > 2 {
					entry.Reward3 = proto.String(red[2])
				}
			}
			result.Recruit = append(result.Recruit, entry)
		}
		return saveProfile(tx, &row, player, g)
	})
	return []proto.Message{result}, err
}

func choosePoolReward(rewards []db.GameplayPoolReward, guaranteed bool) (*db.GameplayPoolReward, error) {
	total := int64(0)
	for _, r := range rewards {
		if !guaranteed || r.Guaranteed {
			total += r.Weight
		}
	}
	if total <= 0 {
		return nil, errors.New("奖池缺少保底奖励")
	}
	n, err := rand.Int(rand.Reader, big.NewInt(total))
	if err != nil {
		return nil, err
	}
	v := n.Int64()
	for i := range rewards {
		r := &rewards[i]
		if guaranteed && !r.Guaranteed {
			continue
		}
		v -= r.Weight
		if v < 0 {
			return r, nil
		}
	}
	return nil, errors.New("奖池抽取失败")
}

func simpleOwned(player *pb.Player, g *model.GameState, key string) int64 {
	switch key {
	case "coin":
		return player.GetCoin()
	case "diamond":
		return player.GetDiamond()
	}
	for _, v := range g.Items {
		if v.GetId() == key {
			return int64(v.GetNum())
		}
	}
	return 0
}
func parsePoolCost(value string) (*pb.BaseResource, error) {
	parts := strings.Split(value, ",")
	if len(parts) != 2 {
		return nil, errors.New("奖池消耗配置无效")
	}
	num, err := strconv.ParseInt(parts[1], 10, 32)
	if err != nil || num <= 0 {
		return nil, errors.New("奖池消耗数量无效")
	}
	if parts[0] != "coin" && parts[0] != "diamond" && !strings.HasPrefix(parts[0], "item_") {
		return nil, errors.New("奖池消耗类型无效")
	}
	return &pb.BaseResource{Id: proto.String(parts[0]), Num: proto.Int32(int32(num))}, nil
}
func deductPoolCost(player *pb.Player, g *model.GameState, cost *pb.BaseResource) error {
	if simpleOwned(player, g, cost.GetId()) < int64(cost.GetNum()) {
		return errors.New("招募或抽奖资源不足")
	}
	switch cost.GetId() {
	case "coin":
		player.Coin = proto.Int64(player.GetCoin() - int64(cost.GetNum()))
	case "diamond":
		player.Diamond = proto.Int64(player.GetDiamond() - int64(cost.GetNum()))
	default:
		for _, v := range g.Items {
			if v.GetId() == cost.GetId() {
				v.Num = proto.Int32(v.GetNum() - cost.GetNum())
				break
			}
		}
	}
	return nil
}

// warriorRecruit 将抽取、扣费、保底、实例和防重回执放入同一事务。
func (s *GameService) warriorRecruit(ctx context.Context, id int64, b *pb.CSWarriorRecruit, stamp int64) ([]proto.Message, error) {
	var result *pb.SCWarriorRecruit
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
		now := LocalPlayNow(ctx)
		st := recruitStoredState(g, b.GetCostType())
		if st.Timestamp > stamp {
			return errors.New("招募请求已过期")
		}
		if st.Timestamp == stamp && st.Response != nil {
			if !proto.Equal(st.Request, b) {
				return errors.New("招募请求时间戳冲突")
			}
			// The original response carries complete warriors; replay must not roll back a grown or sold instance.
			for _, awarded := range st.Response.GetGet().GetWars() {
				current, err := p5Warrior(g, awarded.GetGuid())
				if err != nil || !proto.Equal(current, awarded) {
					return errors.New(constUtil.P5GrowthReceiptChanged)
				}
			}
			result = st.Response
			return nil
		}
		st, err = growthRecruitState(tx, id, g, b.GetCostType(), now)
		if err != nil {
			return err
		}
		cfg, rewards, err := s.loadGameplayPool(tx, fmt.Sprintf("recruit_%d", b.GetCostType()))
		if err != nil {
			return err
		}
		if err := s.validateRecruitClientPrices(cfg, b.GetCostType()); err != nil {
			return err
		}
		if player.GetLevel() < cfg.MinLevel {
			return errors.New("招募等级不足")
		}
		count := int32(1)
		if b.GetNumType() == 2 {
			count = 10
		}
		var king *db.KingRecruitState
		var kingRules *db.KingRecruitRules
		if b.GetCostType() == 3 {
			var space int32
			kingRules, space, _, err = s.kingConfiguration(tx, now)
			if err != nil {
				return err
			}
			king, err = kingState(tx, id, now)
			if err != nil {
				return err
			}
			if king.Draws > kingRules.DailyLimit-count {
				return errors.New("今日王者招募次数不足")
			}
			rewards, _ = s.kingRewards(rewards, space)
		}
		if int64(len(g.Warriors))+int64(count) > int64(player.GetWarriorBagSize()) {
			return errors.New("伙伴背包已满")
		}
		// 所有候选项都先校验，避免错误配置偶发成为有扣费的坏结果。
		hasGuarantee := false
		for _, r := range rewards {
			hero := s.table("Hero", "id", r.ResourceID)
			if hero == nil || hero["heroClass"] != "0" || r.Amount != 1 || r.Stock != 0 {
				return errors.New("招募奖池仅支持真实可培养伙伴且不限库存")
			}
			if b.GetCostType() == 3 && (hero["quality"] != "20" && hero["quality"] != "23" || r.Guaranteed && hero["quality"] != "23") {
				return errors.New("王者卡池与本地红卡保底规则不匹配")
			}
			hasGuarantee = hasGuarantee || r.Guaranteed
		}
		if cfg.GuaranteeEvery > 0 && !hasGuarantee {
			return errors.New("招募保底配置缺失")
		}
		result = &pb.SCWarriorRecruit{Get: &pb.ResultObject{}}
		if b.GetFree() == 1 {
			if count != 1 || now.UnixMilli() < st.NextFree || (cfg.FreePerDay >= 0 && st.FreeUsed >= cfg.FreePerDay) {
				return errors.New("免费招募尚未恢复")
			}
			st.FreeUsed++
			st.NextFree = now.UnixMilli() + cfg.FreeSeconds*1000
		} else {
			price, ticket := cfg.SingleCost, cfg.SingleTicket
			if count == 10 {
				price, ticket = cfg.TenCost, cfg.TenTicket
			}
			cost, err := parsePoolCost(price)
			if err != nil {
				return err
			}
			if ticket != "" {
				t, err := parsePoolCost(ticket)
				if err != nil {
					return err
				}
				if simpleOwned(player, g, t.GetId()) >= int64(t.GetNum()) {
					cost = t
				}
			}
			if err := deductPoolCost(player, g, cost); err != nil {
				return err
			}
			result.Cost = []*pb.BaseResource{cost}
		}
		for i := int32(0); i < count; i++ {
			chosen, err := choosePoolReward(rewards, cfg.GuaranteeEvery > 0 && st.SinceGuarantee+1 >= cfg.GuaranteeEvery)
			if err != nil {
				return err
			}
			token := make([]byte, 16)
			if _, err := rand.Read(token); err != nil {
				return err
			}
			guid := hex.EncodeToString(token)
			for _, existing := range g.Warriors {
				if existing.GetGuid() == guid {
					return errors.New("招募实例冲突")
				}
			}
			w := &pb.Warrior{Id: proto.String(chosen.ResourceID), Guid: proto.String(guid), Level: proto.Int32(1), Exp: proto.Int64(0), Talent: proto.Int32(0), Training: &pb.Training{}, TempTraining: &pb.Training{}, Breached: &pb.Breached{Level: proto.Int32(1)}, Awake: &pb.Awake{}}
			g.Warriors = append(g.Warriors, w)
			result.Get.Wars = append(result.Get.Wars, proto.Clone(w).(*pb.Warrior))
			st.SinceGuarantee++
			if chosen.Guaranteed {
				st.SinceGuarantee = 0
			}
		}
		coins := int64(cfg.CoinPerDraw) * int64(count)
		if coins > math.MaxInt32 || player.GetCoin() > math.MaxInt64-coins {
			return errors.New("招募金币溢出")
		}
		player.Coin = proto.Int64(player.GetCoin() + coins)
		if coins > 0 {
			result.Get.Simps = append(result.Get.Simps, &pb.SimpleItem{Id: proto.String("coin"), Num: proto.Int32(int32(coins))})
		}
		st.Timestamp = stamp
		st.Request = proto.Clone(b).(*pb.CSWarriorRecruit)
		st.Response = proto.Clone(result).(*pb.SCWarriorRecruit)
		if king != nil {
			king.Draws += count
			king.Luck = min(kingRules.LuckLimit, king.Luck+kingRules.LuckPerDraw*count)
			if err := tx.Save(king).Error; err != nil {
				return err
			}
		}
		return saveProfile(tx, &row, player, g)
	})
	return []proto.Message{result}, err
}
