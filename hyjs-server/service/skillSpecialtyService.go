package service

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// InitSkillSpecialty 首次记录缺失的服务端初始化策略，重启保留 SQL 运维调整。
func (s *GameService) InitSkillSpecialty(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.SkillSpecialtyRule{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.SkillSpecialtyRule{ID: constUtil.SkillSpecialtyRuleID, Enabled: true, InitialTypes: "1,2,3", UnlockPolicy: constUtil.SkillSpecialtyUnlockPolicy, Source: constUtil.SkillSpecialtyRuleSource}).Error
}

func (s *GameService) specialtyRule(ctx context.Context) (*db.SkillSpecialtyRule, error) {
	var rule db.SkillSpecialtyRule
	if err := db.GetIns().WithContext(ctx).First(&rule, "id = ?", constUtil.SkillSpecialtyRuleID).Error; err != nil {
		return nil, err
	}
	if !rule.Enabled {
		return nil, errors.New(constUtil.SkillSpecialtyLockedError)
	}
	if rule.Source == "" || rule.UnlockPolicy != constUtil.SkillSpecialtyUnlockPolicy {
		return nil, errors.New(constUtil.SkillSpecialtyConfigError)
	}
	return &rule, nil
}

func (s *GameService) specialtyGate(player *pb.Player, w *pb.Warrior) error {
	var (
		row   = s.table("Hero", "id", w.GetId())
		level = w.GetLevel()
	)
	if w.GetGuid() == player.GetHeroWarId() {
		level = player.GetLevel()
	}
	gate, err := copySweepNumber(s.table("GameConfigure", "StaticName", "skillSpecialityLevelLimit"), "NameValue")
	if err != nil || gate < 1 {
		return errors.New(constUtil.SkillSpecialtyConfigError)
	}
	if row == nil || (row["quality"] != "20" && row["quality"] != "23") || int64(level) < gate {
		return errors.New(constUtil.SkillSpecialtyGateError)
	}
	return nil
}

func (s *GameService) specialtyTable(name, key string, kind, level int32) (map[string]string, error) {
	var found map[string]string
	for _, row := range s.Tables[name] {
		if row["type"] == strconv.Itoa(int(kind)) && row[key] == strconv.Itoa(int(level)) {
			if found != nil {
				return nil, errors.New(constUtil.SkillSpecialtyConfigError)
			}
			found = row
		}
	}
	if found == nil {
		return nil, errors.New(constUtil.SkillSpecialtyConfigError)
	}
	return found, nil
}

func (s *GameService) specialtyRecords(w *pb.Warrior) (map[int32]*pb.SkillRefine, error) {
	var (
		records       = map[int32]*pb.SkillRefine{}
		maxLevel, err = copySweepNumber(s.table("GameConfigure", "StaticName", "maxSkillSpecialityLevelLimit"), "NameValue")
	)
	if err != nil || maxLevel < 1 {
		return nil, errors.New(constUtil.SkillSpecialtyConfigError)
	}
	maxStrength, err := copySweepNumber(s.table("GameConfigure", "StaticName", "maxSkillRefineStrengthLevel"), "NameValue")
	if err != nil || maxStrength < 1 {
		return nil, errors.New(constUtil.SkillSpecialtyConfigError)
	}
	for _, entry := range w.GetSkillRefine() {
		if entry == nil || entry.GetRoleGuid() != w.GetGuid() || entry.GetType() < 1 || entry.GetType() > 5 || entry.GetLevel() < 1 || int64(entry.GetLevel()) > maxLevel || entry.GetStrengthLevel() < 1 || int64(entry.GetStrengthLevel()) > maxStrength || records[entry.GetType()] != nil {
			return nil, errors.New(constUtil.SkillSpecialtyConfigError)
		}
		row, err := s.specialtyTable("SkillSpecialty", "level", entry.GetType(), entry.GetLevel())
		if err != nil {
			return nil, err
		}
		need, err := copySweepNumber(row, "needStrengthLevel")
		if err != nil || need < 1 || int64(entry.GetStrengthLevel()) < need {
			return nil, errors.New(constUtil.SkillSpecialtyConfigError)
		}
		allowedStrength := need
		if int64(entry.GetLevel()) < maxLevel {
			next, err := s.specialtyTable("SkillSpecialty", "level", entry.GetType(), entry.GetLevel()+1)
			if err != nil {
				return nil, err
			}
			nextNeed, err := copySweepNumber(next, "needStrengthLevel")
			if err != nil || nextNeed < need || nextNeed > maxStrength {
				return nil, errors.New(constUtil.SkillSpecialtyConfigError)
			}
			allowedStrength = nextNeed
		}
		if int64(entry.GetStrengthLevel()) > allowedStrength {
			return nil, errors.New(constUtil.SkillSpecialtyConfigError)
		}
		if _, err := s.specialtyTable("SkillSpecialtyStrength", "strengthLevel", entry.GetType(), entry.GetStrengthLevel()); err != nil {
			return nil, err
		}
		records[entry.GetType()] = entry
	}
	return records, nil
}

func (s *GameService) specialtyInitialize(w *pb.Warrior, rule *db.SkillSpecialtyRule) error {
	if len(w.GetSkillRefine()) != 0 {
		_, err := s.specialtyRecords(w)
		return err
	}
	seen := map[int32]bool{}
	for _, part := range strings.Split(rule.InitialTypes, ",") {
		kind, err := strconv.ParseInt(part, 10, 32)
		if err != nil || kind < 1 || kind > 3 || seen[int32(kind)] {
			return errors.New(constUtil.SkillSpecialtyConfigError)
		}
		seen[int32(kind)] = true
		w.SkillRefine = append(w.SkillRefine, &pb.SkillRefine{RoleGuid: proto.String(w.GetGuid()), Type: proto.Int32(int32(kind)), Level: proto.Int32(1), StrengthLevel: proto.Int32(1)})
	}
	_, err := s.specialtyRecords(w)
	return err
}

// skillSpecialtyPrepare 仅初始化已具原门槛的角色，查询准备不触发真实游戏事件。
func (s *GameService) skillSpecialtyPrepare(ctx context.Context, id int64) error {
	rule, err := s.specialtyRule(ctx)
	if err != nil {
		// SQL 关闭时查询保留既有专精，不影响普通伙伴列表。
		if rule == nil && err.Error() == constUtil.SkillSpecialtyLockedError {
			return nil
		}
		return err
	}
	readCtx := context.WithValue(ctx, constUtil.GameRequestContextKey, struct{}{})
	return updateProfile(readCtx, id, func(p *pb.Player, g *model.GameState) error {
		for _, w := range g.Warriors {
			if w != nil && s.specialtyGate(p, w) == nil {
				if err := s.specialtyInitializeTracked(g, w, rule); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// specialtyUnlock 采用 SQL 明示的双强化策略解释原 canOpen 字符串。
func (s *GameService) specialtyUnlock(w *pb.Warrior) error {
	records, err := s.specialtyRecords(w)
	if err != nil {
		return err
	}
	for kind := int32(1); kind <= 5; kind++ {
		entry := records[kind]
		if entry == nil {
			continue
		}
		for level := int32(1); level <= entry.GetStrengthLevel(); level++ {
			row, err := s.specialtyTable("SkillSpecialtyStrength", "strengthLevel", kind, level)
			if err != nil {
				return err
			}
			if row["canOpen"] == "" {
				continue
			}
			for _, part := range strings.Split(row["canOpen"], ";") {
				var (
					pair      = strings.Split(part, ":")
					condition []string
				)
				if len(pair) != 2 {
					return errors.New(constUtil.SkillSpecialtyConfigError)
				}
				condition = strings.Split(pair[0], ",")
				if len(condition) != 2 {
					return errors.New(constUtil.SkillSpecialtyConfigError)
				}
				other, e1 := strconv.ParseInt(condition[0], 10, 32)
				need, e2 := strconv.ParseInt(condition[1], 10, 32)
				target, e3 := strconv.ParseInt(pair[1], 10, 32)
				if e1 != nil || e2 != nil || e3 != nil || other < 1 || other > 5 || need < 1 || need > 10 || target < 4 || target > 5 {
					return errors.New(constUtil.SkillSpecialtyConfigError)
				}
				if records[int32(other)].GetStrengthLevel() >= int32(need) && records[int32(target)] == nil {
					opened := &pb.SkillRefine{RoleGuid: proto.String(w.GetGuid()), Type: proto.Int32(int32(target)), Level: proto.Int32(1), StrengthLevel: proto.Int32(1)}
					w.SkillRefine = append(w.SkillRefine, opened)
					records[int32(target)] = opened
				}
			}
		}
	}
	return nil
}

func (s *GameService) skillSpecialtyUpgrade(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, error) {
	var (
		guid     string
		kind     int32
		strength bool
		response proto.Message
	)
	switch b := request.(type) {
	case *pb.CSUpRoleSkillRefineLevel:
		guid, kind = b.GetRoleGuid(), b.GetType()
		response = &pb.SCUpRoleSkillRefineLevel{}
	case *pb.CSStrengthRoleSkillRefineLevel:
		guid, kind, strength = b.GetRoleGuid(), b.GetType(), true
		response = &pb.SCStrengthRoleSkillRefineLevel{}
	default:
		return nil, errors.New(constUtil.SkillSpecialtyConfigError)
	}
	rule, err := s.specialtyRule(ctx)
	if err != nil {
		return nil, err
	}
	raw, err := proto.Marshal(request)
	if err != nil {
		return nil, err
	}
	key := "level"
	if strength {
		key = "strength"
	}
	var updated *pb.Player
	err = updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error {
		if stamp <= 0 {
			return errors.New(constUtil.SkillSpecialtyReplayError)
		}
		// 升级和强化共享提交顺序。后续操作成功后拒绝旧完整状态，
		// 防止客户端用旧回包覆盖新专精并再次扣减显示库存。
		for priorKey, receipt := range g.SkillSpecialtyReceipts {
			if priorKey != key && receipt != nil && stamp <= receipt.Timestamp {
				return errors.New(constUtil.SkillSpecialtyReplayError)
			}
		}
		if receipt := g.SkillSpecialtyReceipts[key]; receipt != nil && stamp <= receipt.Timestamp {
			prior := request.ProtoReflect().Type().New().Interface()
			if stamp != receipt.Timestamp || proto.Unmarshal(receipt.Request, prior) != nil || !proto.Equal(prior, request) {
				return errors.New(constUtil.SkillSpecialtyReplayError)
			}
			if err := proto.Unmarshal(receipt.Response, response); err != nil {
				return err
			}
			var cached []*pb.SkillRefine
			switch result := response.(type) {
			case *pb.SCUpRoleSkillRefineLevel:
				cached = result.GetSkillRefine()
			case *pb.SCStrengthRoleSkillRefineLevel:
				cached = result.GetSkillRefine()
			}
			warrior, err := p5Warrior(g, guid)
			if err != nil || !proto.Equal(&pb.Warrior{SkillRefine: warrior.GetSkillRefine()}, &pb.Warrior{SkillRefine: cached}) {
				// 重置等后续动作使旧完整专精状态失效；保留原回执审计，
				// 拒绝让客户端把旧满阶结果覆盖当前角色状态。
				return errors.New(constUtil.SkillSpecialtyReplayError)
			}
			updated = p
			return nil
		}
		var w *pb.Warrior
		for _, owned := range g.Warriors {
			if owned.GetGuid() == guid {
				if w != nil {
					return errors.New(constUtil.SkillSpecialtyConfigError)
				}
				w = owned
			}
		}
		if w == nil || kind < 1 || kind > 5 {
			return errors.New(constUtil.SkillSpecialtyLockedError)
		}
		if err := s.specialtyGate(p, w); err != nil {
			return err
		}
		if err := s.specialtyInitialize(w, rule); err != nil {
			return err
		}
		records, err := s.specialtyRecords(w)
		if err != nil {
			return err
		}
		entry := records[kind]
		if entry == nil {
			return errors.New(constUtil.SkillSpecialtyLockedError)
		}
		maxLevel, _ := copySweepNumber(s.table("GameConfigure", "StaticName", "maxSkillSpecialityLevelLimit"), "NameValue")
		maxStrength, _ := copySweepNumber(s.table("GameConfigure", "StaticName", "maxSkillRefineStrengthLevel"), "NameValue")
		if int64(entry.GetLevel()) >= maxLevel || strength && int64(entry.GetStrengthLevel()) >= maxStrength {
			return errors.New(constUtil.SkillSpecialtyLimitError)
		}
		next, err := s.specialtyTable("SkillSpecialty", "level", kind, entry.GetLevel()+1)
		if err != nil {
			return err
		}
		need, err := copySweepNumber(next, "needStrengthLevel")
		if err != nil || need < 1 {
			return errors.New(constUtil.SkillSpecialtyConfigError)
		}
		if strength != (int64(entry.GetStrengthLevel()) < need) {
			return errors.New(constUtil.SkillSpecialtyModeError)
		}
		cost := next["levelUpCost"]
		if strength {
			row, err := s.specialtyTable("SkillSpecialtyStrength", "strengthLevel", kind, entry.GetStrengthLevel()+1)
			if err != nil {
				return err
			}
			cost = row["strengthCost"]
		}
		// 扣费前拒绝重复库存；相同材料不能以多堆叠造成错误扣除。
		seen := map[string]bool{}
		for _, item := range g.Items {
			if item == nil || item.GetId() == "" || item.GetNum() < 0 || seen[item.GetId()] {
				return errors.New(constUtil.SkillSpecialtyItemError)
			}
			seen[item.GetId()] = true
		}
		costs, err := s.growthCost(p, g, cost, 1)
		if err != nil {
			return err
		}
		if strength {
			entry.StrengthLevel = proto.Int32(entry.GetStrengthLevel() + 1)
		} else {
			entry.Level = proto.Int32(entry.GetLevel() + 1)
		}
		if err := s.specialtyUnlock(w); err != nil {
			return err
		}
		if err := s.AddSkillSpecialtyProperties(w, map[string]float64{}); err != nil {
			return err
		}
		if strength {
			result := response.(*pb.SCStrengthRoleSkillRefineLevel)
			result.SkillRefine, result.Cost = w.SkillRefine, costs
		} else {
			result := response.(*pb.SCUpRoleSkillRefineLevel)
			result.SkillRefine, result.Cost = w.SkillRefine, costs
		}
		encoded, err := proto.Marshal(response)
		if err != nil {
			return err
		}
		if g.SkillSpecialtyReceipts == nil {
			g.SkillSpecialtyReceipts = map[string]*model.SkillSpecialtyReceipt{}
		}
		g.SkillSpecialtyReceipts[key] = &model.SkillSpecialtyReceipt{Timestamp: stamp, Request: raw, Response: encoded}
		updated = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response, &pb.SCPlayer{Player: updated}}, nil
}

// AddSkillSpecialtyProperties 沿 UIHero 累加等级和强化的固定属性；未知键拒绝进入战斗。
func (s *GameService) AddSkillSpecialtyProperties(w *pb.Warrior, values map[string]float64) error {
	if len(w.GetSkillRefine()) > 0 {
		row := s.table("Hero", "id", w.GetId())
		if row == nil || (row["quality"] != "20" && row["quality"] != "23") {
			return errors.New(constUtil.SkillSpecialtyGateError)
		}
	}
	records, err := s.specialtyRecords(w)
	if err != nil {
		return err
	}
	for kind := int32(1); kind <= 5; kind++ {
		entry := records[kind]
		if entry == nil {
			continue
		}
		level, err := s.specialtyTable("SkillSpecialty", "level", kind, entry.GetLevel())
		if err != nil {
			return err
		}
		strength, err := s.specialtyTable("SkillSpecialtyStrength", "strengthLevel", kind, entry.GetStrengthLevel())
		if err != nil {
			return err
		}
		for _, row := range []map[string]string{level, strength} {
			for _, part := range strings.Split(row["propAdd"], ";") {
				pair := strings.Split(part, ",")
				if len(pair) != 2 {
					return errors.New(constUtil.SkillSpecialtyConfigError)
				}
				switch pair[0] {
				case "hp", "att", "pdef", "mdef", "hit", "dodge", "cri", "antiCri":
				default:
					return errors.New(constUtil.SkillSpecialtyConfigError)
				}
				amount, err := strconv.ParseFloat(pair[1], 64)
				if err != nil || math.IsNaN(amount) || math.IsInf(amount, 0) || amount < 0 || math.IsInf(values[pair[0]]+amount, 0) {
					return errors.New(constUtil.SkillSpecialtyConfigError)
				}
				values[pair[0]] += amount
			}
		}
	}
	return nil
}
