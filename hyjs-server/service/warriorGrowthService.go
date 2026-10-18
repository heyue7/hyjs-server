package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"slices"
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

// InitializeWarriorGrowth 为新库创建透明本地规则，重启保留运营调整。
func (s *GameService) InitializeWarriorGrowth(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.WarriorGrowthRule{}, &db.WarriorGrowthReceipt{}); err != nil {
		return err
	}
	rules := []db.WarriorGrowthRule{
		{ID: "training_1", Enabled: true, Distribution: "[[-20,20],[10,40],[20,40]]", CriticalMultiplier: 1, Source: "本地服训练：上限基点-20/10/20，权重20/40/40；APK费用/上限/10级开放保留"},
		{ID: "training_2", Enabled: true, Distribution: "[[-10,10],[20,40],[30,50]]", CriticalMultiplier: 1, Source: "本地服训练：上限基点-10/20/30，权重10/40/50；APK费用/上限/10级开放保留"},
		{ID: "training_3", Enabled: true, Distribution: "[[20,40],[40,60]]", CriticalMultiplier: 1, Source: "本地服训练：上限基点20/40，权重40/60；APK费用/上限/10级开放保留"},
		{ID: "breach", Enabled: true, Distribution: "[]", CriticalMultiplier: 2, Source: "本地服突破：APK经验权重/分段概率，暴击倍率2；满门槛升阶余量保留；跨日不清空经验；35级开放"},
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rules).Error
}

// growthOperation 将资源、角色和请求回执一起提交；同一时间戳只返回已提交的结果。
func (s *GameService) growthOperation(ctx context.Context, id int64, guid, kind string, stamp int64, request, response proto.Message, action func(*gorm.DB, *pb.Player, *model.GameState, *pb.Warrior) error) ([]proto.Message, error) {
	var updated *pb.Player
	raw, err := proto.Marshal(request)
	if err != nil {
		return nil, err
	}
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
		var receipt db.WarriorGrowthReceipt
		err = tx.Where("player_id = ? AND guid = ? AND kind = ?", id, guid, kind).First(&receipt).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if stamp <= 0 || (err == nil && stamp < receipt.Timestamp) {
			return errors.New(constUtil.WarriorGrowthReplay)
		}
		if err == nil && stamp == receipt.Timestamp {
			if !bytes.Equal(raw, receipt.Request) {
				return errors.New(constUtil.WarriorGrowthReplay)
			}
			if err := proto.Unmarshal(receipt.Response, response); err != nil {
				return err
			}
			// Talent callbacks carry a complete warrior. A later training,
			// awakening, reset or sale must not be overwritten by an old retry.
			if result, ok := response.(*pb.SCWarriorTalent); ok {
				current, e := p5Warrior(g, result.GetGuid().GetGuid())
				if e != nil || !proto.Equal(current, result.GetGuid()) {
					return errors.New(constUtil.P5GrowthReceiptChanged)
				}
			}
			if err := s.validateP5GrowthReceipt(g, response); err != nil {
				return err
			}
			updated = proto.Clone(player).(*pb.Player)
			return nil
		}
		var warrior *pb.Warrior
		for _, w := range g.Warriors {
			if w.GetGuid() == guid {
				warrior = w
				break
			}
		}
		if warrior == nil || s.table("Hero", "id", warrior.GetId()) == nil || s.table("Hero", "id", warrior.GetId())["heroClass"] == "99" {
			return errors.New(constUtil.WarriorGrowthMissing)
		}
		beforeP5 := proto.Clone(warrior).(*pb.Warrior)
		if err = action(tx, player, g, warrior); err != nil {
			return err
		}
		var (
			invested        []*pb.BaseResource
			trackInvestment bool
		)
		switch result := response.(type) {
		case *pb.SCWarriorTraining:
			invested = result.GetCost()
			trackInvestment = true
		case *pb.SCWarriorTrainingReplace:
			invested = result.GetCost()
			trackInvestment = true
		case *pb.SCWarriorBreached:
			invested = result.GetCost()
			trackInvestment = true
		}
		// 训练与突破由本入口记账；觉醒和融合由统一保存钩子从真实
		// 资源差额记账，避免先改Expected后再次追加同一笔投入。
		if trackInvestment {
			if err = RecordP5Investment(g, guid, beforeP5, warrior, invested, string(request.ProtoReflect().Descriptor().Name())); err != nil {
				return err
			}
		}
		// 养成事实使用当前已绑定的个人游玩日；请求回执与金融账本仍保留各自原时钟。
		if _, ok := request.(*pb.CSWarriorTraining); ok && tx.Migrator().HasTable(&db.EventFact{}) {
			if err = s.RecordP1Event(tx, id, fmt.Sprintf("p5:training:%s:%d", guid, stamp), "warrior_training", 1, LocalPlayNow(ctx)); err != nil {
				return err
			}
		}
		receipt = db.WarriorGrowthReceipt{PlayerID: id, Guid: guid, Kind: kind, Timestamp: stamp, Request: raw}
		receipt.Response, err = proto.Marshal(response)
		if err != nil {
			return err
		}
		if err = saveProfile(tx, &row, player, g); err != nil {
			return err
		}
		updated = proto.Clone(player).(*pb.Player)
		return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&receipt).Error
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response, &pb.SCPlayer{Player: updated}}, nil
}

// p5WarriorTalent restores a committed original callback after a lost response.
// Costs, donor consumption and the latest per-warrior talent receipt share the
// existing profile transaction; replay never reactivates or repays a node.
func (s *GameService) p5WarriorTalent(ctx context.Context, id int64, b *pb.CSWarriorTalent, stamp int64) ([]proto.Message, error) {
	result := &pb.SCWarriorTalent{}
	return s.growthOperation(ctx, id, b.GetGuid(), "talent", stamp, b, result,
		func(tx *gorm.DB, player *pb.Player, g *model.GameState, _ *pb.Warrior) error {
			if err := guardP5TalentMaterials(tx, id, player, g, b); err != nil {
				return err
			}
			return s.warriorTalentApply(player, g, b, result)
		})
}

func growthRule(tx *gorm.DB, key string) (*db.WarriorGrowthRule, error) {
	var rule db.WarriorGrowthRule
	if err := tx.Where("id = ?", key).First(&rule).Error; err != nil {
		return nil, err
	}
	if !rule.Enabled {
		return nil, errors.New(constUtil.WarriorGrowthClosed)
	}
	if rule.Source == "" || rule.CriticalMultiplier < 1 || rule.CriticalMultiplier > 100 || rule.ResetSeconds < 0 || rule.ResetSeconds > 31536000 {
		return nil, errors.New(constUtil.WarriorGrowthConfig)
	}
	return &rule, nil
}

func (s *GameService) growthLevel(player *pb.Player, w *pb.Warrior, key string) error {
	level := w.GetLevel()
	if w.GetGuid() == player.GetHeroWarId() {
		level = player.GetLevel()
	}
	limit, err := strconv.ParseInt(s.table("GameConfigure", "StaticName", key)["NameValue"], 10, 32)
	if err != nil || limit <= 0 {
		return errors.New(constUtil.WarriorGrowthConfig)
	}
	if int64(level) < limit {
		return errors.New(constUtil.WarriorGrowthLevel)
	}
	return nil
}

// trainingLimits 对齐 APK 的英雄ID与实际等级二级索引。
func (s *GameService) trainingLimits(player *pb.Player, w *pb.Warrior) ([4]int32, error) {
	var limits [4]int32
	level := w.GetLevel()
	if w.GetGuid() == player.GetHeroWarId() {
		level = player.GetLevel()
	}
	for _, row := range s.Tables["HeroTrainingMaxMin"] {
		if row["warId"] != w.GetId() || row["warLv"] != strconv.Itoa(int(level)) {
			continue
		}
		for i, key := range []string{"hpMax", "attMax", "pdefMax", "mdefMax"} {
			value, err := strconv.ParseInt(row[key], 10, 32)
			if err != nil || value <= 0 {
				return limits, errors.New(constUtil.WarriorGrowthConfig)
			}
			limits[i] = int32(value)
		}
		return limits, nil
	}
	return limits, errors.New(constUtil.WarriorGrowthConfig)
}
func growthValues(t *pb.Training) [4]int32 {
	return [4]int32{t.GetHp(), t.GetAtt(), t.GetPdef(), t.GetMdef()}
}
func growthTraining(v [4]int32, state int32) *pb.Training {
	return &pb.Training{State: proto.Int32(state), Hp: proto.Int32(v[0]), Att: proto.Int32(v[1]), Pdef: proto.Int32(v[2]), Mdef: proto.Int32(v[3])}
}
func trainingPending(t *pb.Training) bool { return t.GetState() != 0 || growthValues(t) != [4]int32{} }

// growthPick 使用加密随机源，完整验证候选权重后抽取。
func growthPick(options [][2]int64) (int64, error) {
	var total int64
	for _, p := range options {
		if p[1] <= 0 || total > math.MaxInt64-p[1] {
			return 0, errors.New(constUtil.WarriorGrowthConfig)
		}
		total += p[1]
	}
	if total <= 0 {
		return 0, errors.New(constUtil.WarriorGrowthConfig)
	}
	dice, err := rand.Int(rand.Reader, big.NewInt(total))
	if err != nil {
		return 0, err
	}
	position := dice.Int64()
	for _, p := range options {
		position -= p[1]
		if position < 0 {
			return p[0], nil
		}
	}
	return 0, errors.New(constUtil.WarriorGrowthConfig)
}

// growthCost 始终按 APK 配置扣费；整个事务失败时恢复全部材料与角色状态。
func (s *GameService) growthCost(player *pb.Player, g *model.GameState, value string, count int32) ([]*pb.BaseResource, error) {
	var costs []*pb.BaseResource
	seen := map[string]bool{}
	for _, part := range strings.Split(value, ";") {
		cost, err := parsePoolCost(part)
		if err != nil {
			return nil, err
		}
		if seen[cost.GetId()] || int64(cost.GetNum())*int64(count) > math.MaxInt32 {
			return nil, errors.New(constUtil.WarriorGrowthConfig)
		}
		seen[cost.GetId()] = true
		if cost.GetId() != "coin" && cost.GetId() != "diamond" && s.table("Item", "itemId", cost.GetId()) == nil {
			return nil, errors.New(constUtil.WarriorGrowthConfig)
		}
		cost.Num = proto.Int32(cost.GetNum() * count)
		if err = deductPoolCost(player, g, cost); err != nil {
			return nil, err
		}
		costs = append(costs, cost)
	}
	return costs, nil
}

// warriorTraining 扣费后持久化待确认增量，不把预览增量加入战斗属性。
func (s *GameService) warriorTraining(ctx context.Context, id int64, b *pb.CSWarriorTraining, stamp int64) ([]proto.Message, error) {
	result := &pb.SCWarriorTraining{}
	return s.growthOperation(ctx, id, b.GetGuid(), "training", stamp, b, result, func(tx *gorm.DB, player *pb.Player, g *model.GameState, w *pb.Warrior) error {
		if b.GetCostType() < 1 || b.GetCostType() > 3 || b.GetNumType() < 1 || b.GetNumType() > 3 {
			return errors.New(constUtil.WarriorGrowthInvalid)
		}
		if err := s.growthLevel(player, w, "HeroTrainLevel"); err != nil {
			return err
		}
		if trainingPending(w.GetTempTraining()) {
			return errors.New(constUtil.WarriorGrowthPending)
		}
		rule, err := growthRule(tx, fmt.Sprintf("training_%d", b.GetCostType()))
		if err != nil {
			return err
		}
		var options [][2]int64
		if err = json.Unmarshal([]byte(rule.Distribution), &options); err != nil {
			return err
		}
		for _, p := range options {
			if p[0] < -10000 || p[0] > 10000 {
				return errors.New(constUtil.WarriorGrowthConfig)
			}
		}
		// 在扣费前验证全部权重，防止无效 SQL 偶发成功。
		if _, err = growthPick(options); err != nil {
			return err
		}
		limits, err := s.trainingLimits(player, w)
		if err != nil {
			return err
		}
		values := growthValues(w.GetTraining())
		original := values
		allMax := true
		for i, v := range values {
			if v < 0 || v > limits[i] {
				return errors.New(constUtil.WarriorGrowthInvalid)
			}
			if v < limits[i] {
				allMax = false
			}
		}
		if allMax {
			return errors.New(constUtil.WarriorGrowthMaximum)
		}
		count := []int32{1, 5, 10}[b.GetNumType()-1]
		costs, err := s.growthCost(player, g, s.table("GameConfigure", "StaticName", fmt.Sprintf("WarriorTraining%d", b.GetCostType()))["NameValue"], count)
		if err != nil {
			return err
		}
		for n := int32(0); n < count; n++ {
			for i, limit := range limits {
				bps, err := growthPick(options)
				if err != nil {
					return err
				}
				change := int64(limit) * bps / 10000
				if bps != 0 && change == 0 {
					if bps > 0 {
						change = 1
					} else {
						change = -1
					}
				}
				values[i] = int32(max(int64(0), min(int64(limit), int64(values[i])+change)))
			}
		}
		for i := range values {
			values[i] -= original[i]
		}
		// 全零预览在 APK 隐藏确认按钮，保存空状态以允许下一次训练。
		state := int32(1)
		if values == [4]int32{} {
			state = 0
		}
		w.TempTraining = growthTraining(values, state)
		result.Guid = proto.Clone(w).(*pb.Warrior)
		result.Cost = costs
		return nil
	})
}

// warriorTrainingReplace 确认时将预览增量累加，取消仅丢弃预览；已付费用不退还。
func (s *GameService) warriorTrainingReplace(ctx context.Context, id int64, b *pb.CSWarriorTrainingReplace, stamp int64) ([]proto.Message, error) {
	result := &pb.SCWarriorTrainingReplace{}
	return s.growthOperation(ctx, id, b.GetGuid(), "replace", stamp, b, result, func(tx *gorm.DB, player *pb.Player, g *model.GameState, w *pb.Warrior) error {
		if b.GetResult() != 0 && b.GetResult() != 1 {
			return errors.New(constUtil.WarriorGrowthInvalid)
		}
		if !trainingPending(w.GetTempTraining()) {
			return errors.New(constUtil.WarriorGrowthNoPending)
		}
		if b.GetResult() == 1 {
			limits, err := s.trainingLimits(player, w)
			if err != nil {
				return err
			}
			values := growthValues(w.GetTraining())
			delta := growthValues(w.GetTempTraining())
			for i := range values {
				v := int64(values[i]) + int64(delta[i])
				if v < 0 || v > int64(limits[i]) {
					return errors.New(constUtil.WarriorGrowthInvalid)
				}
				values[i] = int32(v)
			}
			w.Training = growthTraining(values, 0)
		}
		w.TempTraining = &pb.Training{}
		result.Result = proto.Clone(w).(*pb.Warrior)
		return nil
	})
}

// breachOptions 恢复 APK 每次经验值与权重；临界点选择暴击概率档位。
func breachOptions(row map[string]string, exp int32) ([][2]int64, float64, error) {
	var options [][2]int64
	for _, part := range strings.Split(row["ExpIncrease"], ";") {
		pair := strings.Split(part, ",")
		if len(pair) != 2 {
			return nil, 0, errors.New(constUtil.WarriorGrowthConfig)
		}
		gain, e1 := strconv.ParseInt(pair[0], 10, 32)
		weight, e2 := strconv.ParseInt(pair[1], 10, 32)
		if e1 != nil || e2 != nil || gain <= 0 || weight <= 0 {
			return nil, 0, errors.New(constUtil.WarriorGrowthConfig)
		}
		options = append(options, [2]int64{gain, weight})
	}
	tier := 1
	for i := 1; i <= 3; i++ {
		threshold, err := strconv.ParseFloat(row[fmt.Sprintf("Critical%d", i)], 64)
		if err != nil || math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold < 0 {
			return nil, 0, errors.New(constUtil.WarriorGrowthConfig)
		}
		if float64(exp) >= threshold {
			tier = i + 1
		}
	}
	probability, err := strconv.ParseFloat(row[fmt.Sprintf("Criprobability%d", tier)], 64)
	if err != nil || math.IsNaN(probability) || probability < 0 || probability > 100 {
		return nil, 0, errors.New(constUtil.WarriorGrowthConfig)
	}
	return options, probability, nil
}

// warriorBreached 每次以当前阶费用抽取真实经验，十连在最高阶提前停止并只收已执行费用。
func (s *GameService) warriorBreached(ctx context.Context, id int64, b *pb.CSWarriorBreached, stamp int64) ([]proto.Message, error) {
	result := &pb.SCWarriorBreached{}
	return s.growthOperation(ctx, id, b.GetGuid(), "breach", stamp, b, result, func(tx *gorm.DB, player *pb.Player, g *model.GameState, w *pb.Warrior) error {
		if b.GetType() != 1 && b.GetType() != 2 {
			return errors.New(constUtil.WarriorGrowthInvalid)
		}
		if err := s.growthLevel(player, w, "HeroBreachedLevel"); err != nil {
			return err
		}
		rule, err := growthRule(tx, "breach")
		if err != nil {
			return err
		}
		state := w.GetBreached()
		if state == nil {
			state = &pb.Breached{Level: proto.Int32(1)}
			w.Breached = state
		}
		if state.GetLevel() < 1 || state.GetExp() < 0 {
			return errors.New(constUtil.WarriorGrowthInvalid)
		}
		now := time.Now()
		if state.GetLastAddExpTime() != "" && rule.ResetSeconds > 0 {
			last, err := strconv.ParseInt(state.GetLastAddExpTime(), 10, 64)
			if err != nil {
				return errors.New(constUtil.WarriorGrowthInvalid)
			}
			if now.Unix()-last >= rule.ResetSeconds {
				state.Exp = proto.Int32(0)
				if err := p5TrackBreachRefund(g, w.GetGuid(), nil, false, true); err != nil {
					return err
				}
			}
		}
		count := int32(1)
		if b.GetType() == 2 {
			count = 10
		}
		aggregated := map[string]int32{}
		for n := int32(0); n < count; n++ {
			row := s.table("HeroBreached", "id", strconv.Itoa(int(state.GetLevel())))
			next := s.table("HeroBreached", "id", strconv.Itoa(int(state.GetLevel()+1)))
			if next == nil {
				if n == 0 {
					return errors.New(constUtil.WarriorGrowthMaximum)
				}
				break
			}
			threshold := tableNumber(row, "Exp")
			if threshold <= 0 || threshold > math.MaxInt32 || threshold != math.Trunc(threshold) || state.GetExp() >= int32(threshold) {
				return errors.New(constUtil.WarriorGrowthConfig)
			}
			options, probability, err := breachOptions(row, state.GetExp())
			if err != nil {
				return err
			}
			gain, err := growthPick(options)
			if err != nil {
				return err
			}
			dice, err := rand.Int(rand.Reader, big.NewInt(1000000))
			if err != nil {
				return err
			}
			if float64(dice.Int64()) < probability*10000 {
				gain *= rule.CriticalMultiplier
			}
			if gain > math.MaxInt32-int64(state.GetExp()) {
				return errors.New(constUtil.WarriorGrowthInvalid)
			}
			costs, err := s.growthCost(player, g, row["Cost"], 1)
			if err != nil {
				return err
			}
			for _, cost := range costs {
				if int64(aggregated[cost.GetId()])+int64(cost.GetNum()) > math.MaxInt32 {
					return errors.New(constUtil.WarriorGrowthInvalid)
				}
				aggregated[cost.GetId()] += cost.GetNum()
			}
			previousLevel := state.GetLevel()
			exp := int64(state.GetExp()) + gain
			// 本地规则保留跨阶经验，最后一级停止增长；缺配置导致整个请求回滚。
			for exp >= int64(threshold) {
				exp -= int64(threshold)
				state.Level = proto.Int32(state.GetLevel() + 1)
				row = s.table("HeroBreached", "id", strconv.Itoa(int(state.GetLevel())))
				next = s.table("HeroBreached", "id", strconv.Itoa(int(state.GetLevel()+1)))
				if next == nil {
					exp = 0
					break
				}
				threshold = tableNumber(row, "Exp")
				if threshold <= 0 || threshold > math.MaxInt32 || threshold != math.Trunc(threshold) {
					return errors.New(constUtil.WarriorGrowthConfig)
				}
			}
			if err := p5TrackBreachRefund(g, w.GetGuid(), costs, state.GetLevel() > previousLevel, false); err != nil {
				return err
			}
			state.Exp = proto.Int32(int32(exp))
			state.LastAddExpTime = proto.String(strconv.FormatInt(now.Unix(), 10))
		}
		// costRecord 沿用协议字段保存本次真实消耗，可用于服务器检查记录。
		keys := make([]string, 0, len(aggregated))
		for key := range aggregated {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		records := []string{}
		for _, key := range keys {
			result.Cost = append(result.Cost, &pb.BaseResource{Id: proto.String(key), Num: proto.Int32(aggregated[key])})
			records = append(records, fmt.Sprintf("%s,%d", key, aggregated[key]))
		}
		state.CostRecord = proto.String(strings.Join(records, ";"))
		result.Guid = proto.Clone(w).(*pb.Warrior)
		return nil
	})
}
