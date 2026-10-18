package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
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

// InitCooking 创建料理持久化结构并保留运营调整的本地永久增幅。
func (s *GameService) InitCooking(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if err := tx.AutoMigrate(&db.CookingCraftState{}, &db.CookingMutationReceipt{}, &db.CookingRule{}, &db.CookingEffect{}); err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.CookingRule{ID: 1, Enabled: true, Gains: constUtil.CookingDefaultGains, Source: constUtil.CookingLocalRuleSource}).Error
	})
}

// cookingMutation 以玩家行锁保护库存、永久属性、时效、经验和回执。
func (s *GameService) cookingMutation(ctx context.Context, id int64, opcode int32, stamp int64, request, response proto.Message, apply func(*gorm.DB, *pb.Player, *model.GameState, *db.CookingCraftState) error) error {
	if stamp <= 0 {
		return errors.New("料理请求时间戳无效")
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
		if err = s.PrepareP5GuildPrayerProperties(ctx, tx, player, game, p13UFNow(ctx)); err != nil {
			return err
		}
		var receipt db.CookingMutationReceipt
		err = tx.Where("player_id = ? AND opcode = ?", id, opcode).Take(&receipt).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil && stamp <= receipt.Timestamp {
			if stamp < receipt.Timestamp {
				return errors.New("料理请求已过期")
			}
			prior := request.ProtoReflect().Type().New().Interface()
			if proto.Unmarshal(receipt.Request, prior) != nil || !proto.Equal(prior, request) {
				return errors.New("料理请求与回执不匹配")
			}
			if err = proto.Unmarshal(receipt.Response, response); err != nil {
				return err
			}
			// Preserve the stored audit while rejecting a retired whole-warrior
			// projection after reset, sale or later real growth.
			if used, ok := response.(*pb.SCFishUsing); ok && used.GetWarrior() != nil {
				current, lookup := p5Warrior(game, used.GetWarrior().GetGuid())
				if lookup != nil || !proto.Equal(current, used.GetWarrior()) {
					return errors.New(constUtil.P5GrowthReceiptChanged)
				}
			}
			return nil
		}
		var state db.CookingCraftState
		if err = tx.Where("player_id = ?", id).Take(&state).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			state = db.CookingCraftState{PlayerID: id, Level: 1}
		} else if err != nil {
			return err
		}
		if state.Level < 1 || state.Level > 6 || state.Exp < 0 {
			return errors.New("料理等级存档无效")
		}
		// Use the same Profile-first, ordered period locks as other economic
		// mutations before save hooks can acquire shared activity rows.
		if err = lockProfilePeriods(tx); err != nil {
			return err
		}
		if err = apply(tx, player, game, &state); err != nil {
			return err
		}
		if err = saveProfile(tx, &row, player, game); err != nil {
			return err
		}
		if err = tx.Save(&state).Error; err != nil {
			return err
		}
		rawRequest, err := proto.Marshal(request)
		if err != nil {
			return err
		}
		rawResponse, err := proto.Marshal(response)
		if err != nil {
			return err
		}
		return tx.Save(&db.CookingMutationReceipt{PlayerID: id, Opcode: opcode, Timestamp: stamp, Request: rawRequest, Response: rawResponse}).Error
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
}

// CookingCraftLevel 对齐 SCCookingLevel 的料理等级字段。
func (s *GameService) CookingCraftLevel(ctx context.Context, id int64) (int32, int32, error) {
	var state db.CookingCraftState
	err := db.GetIns().WithContext(ctx).Where("player_id = ?", id).Take(&state).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 1, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	if state.Level < 1 || state.Level > 6 || state.Exp < 0 {
		return 0, 0, errors.New("料理等级存档无效")
	}
	return state.Level, state.Exp, nil
}

func (s *GameService) cookingOpen(player *pb.Player) error {
	level, err := strconv.ParseInt(s.table("GameConfigure", "StaticName", "OutdoorLevel")["NameValue"], 10, 32)
	if err != nil || level <= 0 || int64(player.GetLevel()) < level {
		return errors.New("生活技能尚未开放")
	}
	return nil
}

// cookingInventoryDelta 校验唯一堆叠和溢出后统一改变食材、成品及金币。
func (s *GameService) cookingInventoryDelta(p *pb.Player, g *model.GameState, deltas map[string]int64) error {
	stacks := map[string]*pb.SimpleItem{}
	for _, item := range g.Cooking {
		if item == nil || item.GetId() == "" || item.GetNum() < 0 || stacks[item.GetId()] != nil {
			return errors.New("料理库存存档无效")
		}
		stacks[item.GetId()] = item
	}
	for key, delta := range deltas {
		if key == "coin" {
			if delta > 0 || p.GetCoin() < 0 || p.GetCoin()+delta < 0 {
				return errors.New("料理金币不足")
			}
			continue
		}
		if s.table("FishSkill", "FishItemID", key) == nil {
			return errors.New("料理库存配置缺失")
		}
		balance := int64(stacks[key].GetNum()) + delta
		if balance < 0 || balance > math.MaxInt32 {
			return errors.New("料理材料不足或库存溢出")
		}
	}
	for key, delta := range deltas {
		if key == "coin" {
			p.Coin = proto.Int64(p.GetCoin() + delta)
			continue
		}
		if stacks[key] == nil {
			stacks[key] = &pb.SimpleItem{Id: proto.String(key)}
			g.Cooking = append(g.Cooking, stacks[key])
		}
		stacks[key].Num = proto.Int32(int32(int64(stacks[key].GetNum()) + delta))
	}
	var remaining []*pb.SimpleItem
	for _, item := range g.Cooking {
		if item.GetNum() > 0 {
			remaining = append(remaining, item)
		}
	}
	g.Cooking = remaining
	return nil
}

func (s *GameService) cookingExperience(state *db.CookingCraftState, amount int64) error {
	if amount <= 0 || amount > math.MaxInt32 || int64(state.Exp)+amount > math.MaxInt32 {
		return errors.New("料理经验配置无效")
	}
	state.Exp += int32(amount)
	for state.Level < 6 {
		limit, err := strconv.ParseInt(s.table("CookingMakeLevelup", "level", strconv.Itoa(int(state.Level)))["exp"], 10, 32)
		if err != nil || limit <= 0 {
			return errors.New("料理升级配置缺失")
		}
		if int64(state.Exp) < limit {
			break
		}
		state.Exp -= int32(limit)
		state.Level++
	}
	if state.Level == 6 {
		state.Exp = 0
	}
	return nil
}

// fishCooking 从完整原配方扣费，客户端只选择成品数量。
func (s *GameService) fishCooking(ctx context.Context, id int64, r *pb.CSFishCooking, stamp int64) ([]proto.Message, error) {
	result := &pb.SCFishCooking{}
	err := s.cookingMutation(ctx, id, constUtil.CookingMakeOpcode, stamp, r, result, func(tx *gorm.DB, p *pb.Player, g *model.GameState, state *db.CookingCraftState) error {
		fish := r.GetFish()
		row := s.table("FishSkill", "FishItemID", fish.GetId())
		if fish.GetNum() < 1 || fish.GetNum() > constUtil.CookingMaxBatch || fish.GetGuid() != "" || row["Type"] != "dish" || row["IsDish"] != "1" || row["Material"] == "" {
			return errors.New("料理制作配方或数量无效")
		}
		if err := s.cookingOpen(p); err != nil {
			return err
		}
		level, err := strconv.ParseInt(row["LevelLimit"], 10, 32)
		if err != nil || level < 1 || level > int64(state.Level) {
			return errors.New("料理等级不足")
		}
		if row["SkillNeeded"] != "" {
			learned, err := P8HasLifeSkill(tx, p.GetPlayerId(), row["SkillNeeded"])
			if err != nil {
				return err
			}
			if !learned {
				return errors.New("该料理需要实际学习生活技能书")
			}
		}
		deltas := map[string]int64{}
		for _, entry := range strings.Split(row["Material"], ";") {
			pair := strings.Split(entry, ",")
			if len(pair) != 2 || pair[0] == "" || deltas[pair[0]] != 0 {
				return errors.New("料理材料配置无效")
			}
			n, err := strconv.ParseInt(pair[1], 10, 32)
			if err != nil || n <= 0 || n*int64(fish.GetNum()) > math.MaxInt32 {
				return errors.New("料理材料数量配置无效")
			}
			deltas[pair[0]] = -n * int64(fish.GetNum())
		}
		result.Cost = gemCosts(deltas)
		deltas[fish.GetId()] += int64(fish.GetNum())
		if err = s.cookingInventoryDelta(p, g, deltas); err != nil {
			return err
		}
		exp, err := strconv.ParseInt(row["MakingExp"], 10, 32)
		if err != nil {
			return errors.New("料理制作经验配置无效")
		}
		add := exp * int64(fish.GetNum())
		if err = s.cookingExperience(state, add); err != nil {
			return err
		}
		result.Res = gemRewards(map[string]int64{fish.GetId(): int64(fish.GetNum())})
		result.Level, result.Exp, result.Add = proto.Int32(state.Level), proto.Int32(state.Exp), proto.Int32(int32(add))
		return nil
	})
	return []proto.Message{result}, err
}

func cookingProperties(prop *pb.BaseProp) map[string]float64 {
	return map[string]float64{"hp": prop.GetHp(), "att": prop.GetAtt(), "pdef": prop.GetPdef(), "mdef": prop.GetMdef(), "cri": prop.GetCri(), "antiCri": prop.GetAntiCri()}
}

// AddCookingProperties 将 Warrior.cooking 的永久基础属性同时用于面板和战斗。
func (s *GameService) AddCookingProperties(w *pb.Warrior, values map[string]float64) error {
	for key, value := range cookingProperties(w.GetCooking().GetProp()) {
		if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return errors.New("料理永久属性存档无效")
		}
		values[key] += value
	}
	return nil
}

// fishUsing 消耗一份料理；永久属性累加到角色，奖励料理替换全局时效槽。
func (s *GameService) fishUsing(ctx context.Context, id int64, r *pb.CSFishUsing, stamp int64) ([]proto.Message, error) {
	result := &pb.SCFishUsing{}
	err := s.cookingMutation(ctx, id, constUtil.CookingUseOpcode, stamp, r, result, func(tx *gorm.DB, p *pb.Player, g *model.GameState, _ *db.CookingCraftState) error {
		item := r.GetCostItem()
		row := s.table("FishSkill", "FishItemID", item.GetId())
		if item.GetNum() != 1 || item.GetGuid() != "" || row["Type"] != "dish" {
			return errors.New("料理使用道具无效")
		}
		if err := s.cookingOpen(p); err != nil {
			return err
		}
		var warrior *pb.Warrior
		for _, w := range g.Warriors {
			if w.GetGuid() == r.GetHeroGuid() {
				if warrior != nil {
					return errors.New("料理角色存档重复")
				}
				warrior = w
			}
		}
		if warrior == nil || r.GetHeroGuid() == "" || s.table("Hero", "id", warrior.GetId()) == nil || s.table("Hero", "id", warrior.GetId())["heroClass"] == "99" {
			return errors.New("料理角色不存在")
		}
		kind := row["ValueType"]
		switch kind {
		case "permanent1", "permanent2", "permanent3", "permanent4":
			var rule db.CookingRule
			if err := tx.Take(&rule, 1).Error; err != nil {
				return err
			}
			if !rule.Enabled || strings.TrimSpace(rule.Source) == "" {
				return errors.New("本地永久料理规则未启用")
			}
			gains, limits, err := s.cookingPermanentPolicy(&rule, row)
			if err != nil {
				return err
			}
			values := cookingProperties(warrior.GetCooking().GetProp())
			if err := s.AddCookingProperties(warrior, map[string]float64{}); err != nil {
				return err
			}
			seen := map[string]bool{}
			for _, key := range strings.Split(row["Value"], ";") {
				gain, ok := gains[key]
				if _, valid := values[key]; !valid || !ok || gain <= 0 || gain > 1000000 || math.IsNaN(gain) || math.IsInf(gain, 0) || seen[key] {
					return errors.New("永久料理增幅配置无效")
				}
				seen[key] = true
				if limit, capped := limits[key]; capped {
					if values[key] >= limit {
						return errors.New("该料理涉及的属性已达上限，请使用更高等级的料理")
					}
					gain = math.Min(gain, limit-values[key])
				}
				values[key] += gain
				if math.IsInf(values[key], 0) {
					return errors.New("永久料理属性溢出")
				}
			}
			prop, err := model.GameMessage("BaseProp", values)
			if err != nil {
				return err
			}
			warrior.Cooking = &pb.Cooking{Prop: prop.(*pb.BaseProp)}
			result.Warrior = proto.Clone(warrior).(*pb.Warrior)
		case "extraCoin", "extraExp", "extraCoinExp":
			pair := strings.Split(row["Value"], ";")
			if len(pair) != 2 {
				return errors.New("奖励料理配置无效")
			}
			minutes, err := strconv.ParseInt(pair[0], 10, 32)
			if err != nil || minutes < 1 || minutes > 525600 {
				return errors.New("奖励料理持续时间无效")
			}
			percent, err := strconv.ParseFloat(pair[1], 64)
			if err != nil || percent <= 0 || percent > 10000 || math.IsNaN(percent) || math.IsInf(percent, 0) {
				return errors.New("奖励料理百分比无效")
			}
			// 客户端弹窗确认后发送同一使用消息，使用服务器时间替换旧时效。
			if err = tx.Save(&db.CookingEffect{PlayerID: id, ItemID: item.GetId(), Kind: kind, EndAt: time.Now().UnixMilli() + minutes*60000, Value: percent / 100}).Error; err != nil {
				return err
			}
		default:
			return errors.New("该料理用途尚未恢复，回血料理需在野外使用")
		}
		if err := s.cookingInventoryDelta(p, g, map[string]int64{item.GetId(): -1}); err != nil {
			return err
		}
		if err := s.P8RefreshCraftPower(p, g); err != nil {
			return err
		}
		result.Cost = []*pb.BaseResource{{Id: proto.String(item.GetId()), Num: proto.Int32(1)}}
		return nil
	})
	return []proto.Message{result}, err
}

// cookingPermanentPolicy 以原宝石固定值推导单份增幅，保留显式运营数值与可配置累计上限。
func (s *GameService) cookingPermanentPolicy(rule *db.CookingRule, row map[string]string) (map[string]float64, map[string]float64, error) {
	policy := map[string]float64{}
	if json.Unmarshal([]byte(rule.Gains), &policy) != nil {
		return nil, nil, errors.New("本地永久料理规则无效")
	}
	level := row["Quality"]
	if row["ValueType"] == "permanent3" {
		// 原黄金套餐资质为7，用户指定按10级料理对照宝石曲线。
		level = "10"
	}
	gains := map[string]float64{}
	limits := map[string]float64{}
	scale, scaled := policy["gemScale"]
	if scaled && (scale <= 0 || scale > 1 || math.IsNaN(scale) || math.IsInf(scale, 0)) {
		return nil, nil, errors.New("料理宝石倍率配置无效")
	}
	for _, key := range strings.Split(row["Value"], ";") {
		if scaled {
			// 每个普通颜色只提供一种属性；按等级及属性精确查找，避免误用七彩的复合值。
			found := false
			for _, gem := range s.Tables["MineSkill"] {
				if gem["Type"] != "magicGem" || gem["Level"] != level || gem["Color"] == "7" {
					continue
				}
				pair := strings.Split(gem["Value"], ",")
				if len(pair) != 2 || pair[0] != key {
					continue
				}
				value, err := strconv.ParseFloat(pair[1], 64)
				if err != nil || found || value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
					return nil, nil, errors.New("料理对应宝石属性无效或重复")
				}
				gains[key], found = value*scale, true
			}
			if !found {
				return nil, nil, errors.New("料理对应等级的宝石属性缺失")
			}
		} else {
			gains[key] = policy[key]
		}
		// 上限以角色累计属性比较，多个属性共用当前存量，不能通过混吃绕开。
		limitLevel := level
		if row["ValueType"] == "permanent4" {
			limitLevel = "2"
		}
		if limit, ok := policy["limit"+limitLevel+"."+key]; ok {
			if limit <= 0 || math.IsNaN(limit) || math.IsInf(limit, 0) {
				return nil, nil, errors.New("料理累计上限配置无效")
			}
			limits[key] = limit
		} else if policy["requireLimits"] == 1 {
			return nil, nil, errors.New("该料理永久属性上限尚未配置")
		}
	}
	return gains, limits, nil
}

func cookingEffect(tx *gorm.DB, id, now int64) (*db.CookingEffect, error) {
	var effect db.CookingEffect
	err := tx.Where("player_id = ?", id).Take(&effect).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if effect.Kind != "extraCoin" && effect.Kind != "extraExp" && effect.Kind != "extraCoinExp" || effect.Value <= 0 || effect.Value > 100 || math.IsNaN(effect.Value) || math.IsInf(effect.Value, 0) || effect.EndAt < 0 {
		return nil, errors.New("奖励料理时效存档无效")
	}
	if effect.EndAt <= now {
		return nil, nil
	}
	return &effect, nil
}

func (s *GameService) cookingEffectList(ctx context.Context, id int64) ([]proto.Message, error) {
	effect, err := cookingEffect(db.GetIns().WithContext(ctx), id, time.Now().UnixMilli())
	if err != nil {
		return nil, err
	}
	result := &pb.SCCookingEffectList{}
	if effect != nil {
		// 原 UI 以 effectId 匹配 extra*；未恢复用途的 effectType 保留协议缺省值。
		result.EffectList = []*pb.ContinuedEffect{{EffectId: proto.String(effect.Kind), EffectTime: proto.Int64(effect.EndAt), EffectValue: proto.Float64(effect.Value)}}
	}
	return []proto.Message{result}, nil
}

// ApplyCookingRewardBonus 在调用方玩家行锁事务内结算真实有效时效；扫荡每场额外扣原表分钟数。
// 普通战斗传 sweepCount=0，失败或重复回执分支不调用，全部结果与战斗存档原子提交。
func (s *GameService) ApplyCookingRewardBonus(tx *gorm.DB, id, now, coin, exp int64, sweepCount int32) (int64, int64, error) {
	if coin < 0 || exp < 0 || coin > math.MaxInt32 || exp > math.MaxInt32 || sweepCount < 0 {
		return 0, 0, errors.New("奖励料理基础奖励无效")
	}
	effect, err := cookingEffect(tx, id, now)
	if err != nil {
		return 0, 0, err
	}
	if effect == nil {
		return coin, exp, nil
	}
	if effect.Kind == "extraCoin" || effect.Kind == "extraCoinExp" {
		coin = int64(math.Floor(float64(coin) * (1 + effect.Value)))
	}
	if effect.Kind == "extraExp" || effect.Kind == "extraCoinExp" {
		exp = int64(math.Floor(float64(exp) * (1 + effect.Value)))
	}
	if coin > math.MaxInt32 || exp > math.MaxInt32 {
		return 0, 0, errors.New("奖励料理结算溢出")
	}
	if sweepCount > 0 {
		minutes, err := strconv.ParseInt(s.table("GameConfigure", "StaticName", "LifeSkillFightTenTimeCost")["NameValue"], 10, 32)
		if err != nil || minutes < 1 || minutes > 525600 {
			return 0, 0, errors.New("扫荡料理时效配置无效")
		}
		effect.EndAt = max(now, effect.EndAt-minutes*60000*int64(sweepCount))
		if err = tx.Save(effect).Error; err != nil {
			return 0, 0, err
		}
	}
	return coin, exp, nil
}
