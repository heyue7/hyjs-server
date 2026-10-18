package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// InitializeWarriorAwake 保留运营SQL修改，初次启动插入透明本地规则。
func (s *GameService) InitializeWarriorAwake(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.WarriorAwakeRule{}); err != nil {
		return err
	}
	rules := []db.WarriorAwakeRule{
		{ID: "awake_1", Enabled: true, Source: "本地规则：无额外金币；APK物品与2张同卡；GUID排序选取非主角非阵容IsZeroHero闲置卡"},
		{ID: "awake_2", Enabled: true, Source: "本地规则：无额外金币；APK物品与5张同卡；GUID排序选取非主角非阵容IsZeroHero闲置卡"},
		{ID: "fuse", Enabled: true, Source: "本地规则：无额外金币；APK类别19道具经验；到最高级余量丢弃，消耗全部提交材料"},
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rules).Error
}

func awakeRule(tx *gorm.DB, key string) (*db.WarriorAwakeRule, error) {
	var rule db.WarriorAwakeRule
	if err := tx.Where("id = ?", key).First(&rule).Error; err != nil {
		return nil, err
	}
	if !rule.Enabled {
		return nil, errors.New(constUtil.WarriorAwakeClosed)
	}
	if rule.Source == "" || rule.CoinCost < 0 || rule.CoinCost > math.MaxInt32 {
		return nil, errors.New(constUtil.WarriorAwakeConfig)
	}
	return &rule, nil
}

func (s *GameService) awakeEligible(player *pb.Player, w *pb.Warrior) error {
	if s.table("Hero", "id", w.GetId())["IsOpenAwake"] != "1" {
		return errors.New(constUtil.WarriorAwakeClosed)
	}
	if err := s.growthLevel(player, w, "heroAwakeLevel"); err != nil {
		return err
	}
	a := w.GetAwake()
	if a.GetStage() < 0 || a.GetStage() > 2 || a.GetFuseLevel() < 0 || a.GetFuseExp() < 0 || (a.GetStage() < 2 && (a.GetFuseLevel() != 0 || a.GetFuseExp() != 0)) {
		return errors.New(constUtil.WarriorAwakeInvalid)
	}
	return nil
}

func (s *GameService) awakeTable(name, id string, stage int32) map[string]string {
	for _, row := range s.Tables[name] {
		if row["heroId"] == id && row["stage"] == strconv.Itoa(int(stage)) {
			return row
		}
	}
	return nil
}

// warriorAwake 从存档选择真实闲置同卡；材料、阶段、金币与回执同事务提交。
func (s *GameService) warriorAwake(ctx context.Context, id int64, b *pb.CSWarriorAwake, stamp int64) ([]proto.Message, error) {
	result := &pb.SCWarriorAwake{}
	return s.growthOperation(ctx, id, b.GetGuid(), "awake", stamp, b, result, func(tx *gorm.DB, player *pb.Player, g *model.GameState, w *pb.Warrior) error {
		if err := s.awakeEligible(player, w); err != nil {
			return err
		}
		stage := w.GetAwake().GetStage() + 1
		if stage > 2 {
			return errors.New(constUtil.WarriorGrowthMaximum)
		}
		rule, err := awakeRule(tx, fmt.Sprintf("awake_%d", stage))
		if err != nil {
			return err
		}
		cost := s.awakeTable("HeroAwakenCost", w.GetId(), stage)
		add := s.awakeTable("HeroAwakenAdd", w.GetId(), stage)
		if cost == nil || add == nil || cost["quality"] != s.table("Hero", "id", w.GetId())["quality"] {
			return errors.New(constUtil.WarriorAwakeConfig)
		}
		count, err := strconv.Atoi(cost["selfCostNum"])
		if err != nil || count < 1 || count > 100 {
			return errors.New(constUtil.WarriorAwakeConfig)
		}
		held, err := p5WarriorMaterialHolds(tx, id)
		if err != nil {
			return err
		}
		used := map[string]bool{w.GetGuid(): true, player.GetHeroWarId(): true}
		for _, slot := range g.Formation {
			for _, thing := range slot.GetThings() {
				used[thing.GetGuid()] = true
			}
		}
		donors := []*pb.Warrior{}
		for _, candidate := range g.Warriors {
			if !used[candidate.GetGuid()] && candidate.GetId() == w.GetId() && p5WarriorMaterialAvailable(player, g, candidate, held) {
				donors = append(donors, candidate)
			}
		}
		sort.Slice(donors, func(i, j int) bool { return donors[i].GetGuid() < donors[j].GetGuid() })
		if len(donors) < count {
			return errors.New(constUtil.WarriorAwakeMaterial)
		}
		costs, err := s.growthCost(player, g, cost["itemCost"], 1)
		if err != nil {
			return err
		}
		if rule.CoinCost > 0 {
			gold := &pb.BaseResource{Id: proto.String("coin"), Num: proto.Int32(int32(rule.CoinCost))}
			if err = deductPoolCost(player, g, gold); err != nil {
				return err
			}
			costs = append(costs, gold)
		}
		consumed := map[string]bool{}
		for _, donor := range donors[:count] {
			if consumed[donor.GetGuid()] {
				return errors.New(constUtil.WarriorAwakeInvalid)
			}
			consumed[donor.GetGuid()] = true
			costs = append(costs, &pb.BaseResource{Id: proto.String(donor.GetId()), Guid: proto.String(donor.GetGuid()), Num: proto.Int32(1)})
		}
		remaining := make([]*pb.Warrior, 0, len(g.Warriors)-count)
		for _, candidate := range g.Warriors {
			if !consumed[candidate.GetGuid()] {
				remaining = append(remaining, candidate)
			}
		}
		g.Warriors = remaining
		w.Awake = &pb.Awake{Stage: proto.Int32(stage), FuseLevel: proto.Int32(0), FuseExp: proto.Int32(0)}
		if stage == 2 {
			skill := s.table("Hero", "id", w.GetId())["AwakeSkillId"]
			if skill == "" || s.table("Skill", "SkillId", skill) == nil {
				return errors.New(constUtil.WarriorAwakeConfig)
			}
			w.AwakenSkill = proto.String(skill + ",2")
		}
		// 在提交前校验新阶段的战斗配置，缺属性或天赋时回滚材料消耗。
		var talents []string
		if err = s.AddWarriorAwakeProperties(w, map[string]float64{}, map[string]float64{}, &talents); err != nil {
			return err
		}
		result.NewWarrior = proto.Clone(w).(*pb.Warrior)
		result.Cost = costs
		return nil
	})
}

func (s *GameService) fuseTable(w *pb.Warrior, level int32) map[string]string {
	quality := s.table("Hero", "id", w.GetId())["quality"]
	for _, row := range s.Tables["HeroAwakenFuseLevel"] {
		if row["quality"] == quality && row["fuseLevel"] == strconv.Itoa(int(level)) {
			return row
		}
	}
	return nil
}

// warriorAwakeFuse 只接受APK类别19物品，服务端计算经验与跨级余量。
func (s *GameService) warriorAwakeFuse(ctx context.Context, id int64, b *pb.CSAwakeFuse, stamp int64) ([]proto.Message, error) {
	result := &pb.SCAwakeFuse{}
	return s.growthOperation(ctx, id, b.GetGuid(), "fuse", stamp, b, result, func(tx *gorm.DB, player *pb.Player, g *model.GameState, w *pb.Warrior) error {
		if err := s.awakeEligible(player, w); err != nil {
			return err
		}
		if w.GetAwake().GetStage() != 2 || len(b.GetCost()) < 1 || len(b.GetCost()) > 100 {
			return errors.New(constUtil.WarriorAwakeInvalid)
		}
		rule, err := awakeRule(tx, "fuse")
		if err != nil {
			return err
		}
		level := w.GetAwake().GetFuseLevel()
		exp := int64(w.GetAwake().GetFuseExp())
		row := s.fuseTable(w, level)
		need, err := strconv.ParseInt(row["needExp"], 10, 32)
		if err != nil || need < 0 {
			return errors.New(constUtil.WarriorAwakeConfig)
		}
		if need == 0 {
			return errors.New(constUtil.WarriorGrowthMaximum)
		}
		if exp >= need {
			return errors.New(constUtil.WarriorAwakeInvalid)
		}
		seen := map[string]bool{}
		costs := []*pb.BaseResource{}
		for _, cost := range b.GetCost() {
			item := s.table("Item", "itemId", cost.GetId())
			gain, parseErr := strconv.ParseInt(item["exp"], 10, 32)
			if seen[cost.GetId()] || cost.GetGuid() != "" || cost.GetNum() < 1 || item["category"] != "19" || parseErr != nil || gain < 1 {
				return errors.New(constUtil.WarriorAwakeInvalid)
			}
			seen[cost.GetId()] = true
			exp += gain * int64(cost.GetNum())
			if exp > math.MaxInt32 {
				return errors.New(constUtil.WarriorAwakeInvalid)
			}
			if err = deductPoolCost(player, g, cost); err != nil {
				return err
			}
			costs = append(costs, proto.Clone(cost).(*pb.BaseResource))
		}
		if rule.CoinCost > 0 {
			gold := &pb.BaseResource{Id: proto.String("coin"), Num: proto.Int32(int32(rule.CoinCost))}
			if err = deductPoolCost(player, g, gold); err != nil {
				return err
			}
			costs = append(costs, gold)
		}
		for exp >= need {
			exp -= need
			level++
			row = s.fuseTable(w, level)
			need, err = strconv.ParseInt(row["needExp"], 10, 32)
			if err != nil || need < 0 || level > 100 {
				return errors.New(constUtil.WarriorAwakeConfig)
			}
			if need == 0 {
				exp = 0
				break
			}
		}
		w.Awake.FuseLevel = proto.Int32(level)
		w.Awake.FuseExp = proto.Int32(int32(exp))
		var talents []string
		if err = s.AddWarriorAwakeProperties(w, map[string]float64{}, map[string]float64{}, &talents); err != nil {
			return err
		}
		result.NewWarrior = proto.Clone(w).(*pb.Warrior)
		result.Cost = costs
		return nil
	})
}

// AddWarriorAwakeProperties 对齐UIHeroAwakeData的两阶段累加与逐级融合成长。
func (s *GameService) AddWarriorAwakeProperties(w *pb.Warrior, values, percent map[string]float64, talents *[]string) error {
	a := w.GetAwake()
	if a.GetStage() < 0 || a.GetStage() > 2 || a.GetFuseLevel() < 0 || a.GetFuseLevel() > 100 || (a.GetStage() < 2 && a.GetFuseLevel() != 0) {
		return errors.New(constUtil.WarriorAwakeInvalid)
	}
	if a.GetStage() == 0 {
		return nil
	}
	for stage := int32(1); stage <= a.GetStage(); stage++ {
		row := s.awakeTable("HeroAwakenAdd", w.GetId(), stage)
		if row == nil {
			return errors.New(constUtil.WarriorAwakeConfig)
		}
		for _, key := range []string{"hp", "att", "pdef", "mdef"} {
			value, err := strconv.ParseFloat(row[key], 64)
			if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
				return errors.New(constUtil.WarriorAwakeConfig)
			}
			values[key] += value
		}
		// 第二阶段talentId使用技能ID，觉醒技能单独写入BattleHero.awakenSkill。
		if talentID := row["talentId"]; stage == 1 && talentID != "" {
			talent := s.table("Talent", "Id", talentID)
			if talent == nil {
				return errors.New(constUtil.WarriorAwakeConfig)
			}
			*talents = append(*talents, talentID)
			for i := 0; i < 3; i++ {
				key := fmt.Sprintf("Buff%d", i)
				if talent[key] != "" && tableNumber(talent, key+"TriggerTimer") == 0 && tableNumber(talent, key+"Select") == 0 {
					buff := s.table("Buff", "Id", talent[key])
					if buff == nil {
						return errors.New(constUtil.WarriorAwakeConfig)
					}
					copyTalentProperty(values, percent, buff)
				}
			}
		}
	}
	for level := int32(1); level <= a.GetFuseLevel(); level++ {
		var found map[string]string
		for _, row := range s.Tables["HeroAwakenFuseProp"] {
			if row["heroId"] == w.GetId() && int64(level) >= int64(tableNumber(row, "startLevel")) && int64(level) <= int64(tableNumber(row, "endLevel")) {
				found = row
				break
			}
		}
		if found == nil {
			return errors.New(constUtil.WarriorAwakeConfig)
		}
		for _, key := range []string{"hp", "att", "pdef", "mdef"} {
			values[key] += tableNumber(found, key+"Grow")
		}
	}
	return nil
}
