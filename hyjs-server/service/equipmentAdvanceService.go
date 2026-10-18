package service

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// equipmentRefine 按原道具经验和资质分档精炼，时间戳回执保证重试只扣一次材料。
func (s *GameService) equipmentRefine(ctx context.Context, id int64, request *pb.CSEquipRefine, stamp int64) ([]proto.Message, error) {
	var result *pb.SCEquipRefine
	err := updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
		if stamp <= 0 {
			return errors.New("精炼请求缺少有效时间戳")
		}
		if receipt := g.EquipmentRefineReceipts[request.GetGuid()]; receipt != nil {
			if stamp == receipt.Timestamp {
				if receipt.Request == nil || receipt.Response == nil || !proto.Equal(receipt.Request, request) {
					return errors.New("精炼请求与已有回执不匹配")
				}
				if err := s.validateP5GrowthReceipt(g, receipt.Response); err != nil {
					return err
				}
				result = proto.Clone(receipt.Response).(*pb.SCEquipRefine)
				return nil
			}
			if stamp < receipt.Timestamp {
				return errors.New("精炼请求已过期")
			}
		}
		equip, err := s.equipmentRecord(g, request.GetGuid())
		if err != nil {
			return err
		}
		if err = equipmentBasicState(equip); err != nil {
			return err
		}
		if err = s.equipmentAdvancedProperties(equip, map[string]float64{}); err != nil {
			return err
		}
		levels, err := s.equipmentRefineLevels(equip)
		if err != nil {
			return err
		}
		if int(equip.GetRefineLv()) >= len(levels) {
			return errors.New("装备已达到精炼上限")
		}
		if int64(equip.GetRefineExp()) >= levels[equip.GetRefineLv()] {
			return errors.New("装备精炼经验存档无效")
		}
		allowed := strings.Split(s.table("GameConfigure", "StaticName", "EquipRefineCostItemEnum")["NameValue"], ",")
		var experience int64
		seen := map[string]bool{}
		for _, cost := range request.GetCost() {
			valid := false
			for _, key := range allowed {
				valid = valid || key == cost.GetId()
			}
			row := s.table("Item", "itemId", cost.GetId())
			exp, parseErr := strconv.ParseInt(row["exp"], 10, 32)
			if !valid || seen[cost.GetId()] || cost.GetGuid() != "" || cost.GetNum() <= 0 || parseErr != nil || exp <= 0 || row["category"] != "7" {
				return errors.New("精炼材料类型或数量无效")
			}
			seen[cost.GetId()] = true
			experience += exp * int64(cost.GetNum())
		}
		if experience <= 0 || experience > math.MaxInt32-int64(equip.GetRefineExp()) {
			return errors.New("精炼经验数量无效或溢出")
		}
		if err = s.equipmentConsume(g, nil, request.GetCost()); err != nil {
			return err
		}
		// 客户端预测逐级扣除需求，跨级后的余量保留在 refineExp。
		experience += int64(equip.GetRefineExp())
		level := equip.GetRefineLv()
		for int(level) < len(levels) && experience >= levels[level] {
			experience -= levels[level]
			level++
		}
		equip.RefineLv = proto.Int32(level)
		equip.RefineExp = proto.Int32(int32(experience))
		if err = s.equipmentAdvancedProperties(equip, map[string]float64{}); err != nil {
			return err
		}
		result = &pb.SCEquipRefine{Equip: proto.Clone(equip).(*pb.Equipment), Cost: proto.Clone(request).(*pb.CSEquipRefine).Cost}
		if g.EquipmentRefineReceipts == nil {
			g.EquipmentRefineReceipts = map[string]*model.EquipmentRefineReceipt{}
		}
		g.EquipmentRefineReceipts[request.GetGuid()] = &model.EquipmentRefineReceipt{Timestamp: stamp, Request: proto.Clone(request).(*pb.CSEquipRefine), Response: proto.Clone(result).(*pb.SCEquipRefine)}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{result}, nil
}

// equipmentStar 每次提升一星，费用取目标星级，低资质装备和未开放玩家不可升星。
func (s *GameService) equipmentStar(ctx context.Context, id int64, request *pb.CSEquipStar, stamp int64) ([]proto.Message, error) {
	var result *pb.SCEquipStar
	err := updateProfile(ctx, id, func(player *pb.Player, g *model.GameState) error {
		if stamp <= 0 {
			return errors.New("升星请求缺少有效时间戳")
		}
		if receipt := g.EquipmentStarReceipts[request.GetGuid()]; receipt != nil {
			if stamp == receipt.Timestamp {
				if receipt.Response == nil {
					return errors.New("装备升星回执无效")
				}
				if err := s.validateP5GrowthReceipt(g, receipt.Response); err != nil {
					return err
				}
				result = proto.Clone(receipt.Response).(*pb.SCEquipStar)
				return nil
			}
			if stamp < receipt.Timestamp {
				return errors.New("升星请求已过期")
			}
		}
		equip, err := s.equipmentRecord(g, request.GetGuid())
		if err != nil {
			return err
		}
		if err = equipmentBasicState(equip); err != nil {
			return err
		}
		if err = s.equipmentAdvancedProperties(equip, map[string]float64{}); err != nil {
			return err
		}
		gate := tableNumber(s.table("GameConfigure", "StaticName", "equipStarOpenLevel"), "NameValue")
		row := s.table("Equip", "id", equip.GetEquipId())
		if gate <= 0 || float64(player.GetLevel()) < gate || tableNumber(row, "quality") < 20 {
			return errors.New("玩家等级或装备资质尚未达到升星要求")
		}
		star := equip.GetStar() + 1
		target := s.equipmentStarRow(row["type"], star)
		if target == nil {
			return errors.New("装备已达到升星上限")
		}
		var costs []*pb.BaseResource
		for _, part := range strings.Split(target["cost"], ";") {
			pair := strings.Split(part, ",")
			if len(pair) != 2 {
				return errors.New("装备升星费用配置无效")
			}
			num, err := strconv.ParseInt(pair[1], 10, 32)
			if err != nil || num <= 0 {
				return errors.New("装备升星费用数量无效")
			}
			costs = append(costs, &pb.BaseResource{Id: proto.String(pair[0]), Num: proto.Int32(int32(num))})
		}
		if err = s.equipmentConsume(g, player, costs); err != nil {
			return err
		}
		equip.Star = proto.Int32(star)
		if err = s.equipmentAdvancedProperties(equip, map[string]float64{}); err != nil {
			return err
		}
		result = &pb.SCEquipStar{Equip: proto.Clone(equip).(*pb.Equipment), Cost: costs}
		if g.EquipmentStarReceipts == nil {
			g.EquipmentStarReceipts = map[string]*model.EquipmentStarReceipt{}
		}
		g.EquipmentStarReceipts[request.GetGuid()] = &model.EquipmentStarReceipt{Timestamp: stamp, Response: proto.Clone(result).(*pb.SCEquipStar)}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{result}, nil
}

// equipmentConsume 只接受原表堆叠材料和金币，完整检查所有权后原子扣费。
func (s *GameService) equipmentConsume(g *model.GameState, player *pb.Player, costs []*pb.BaseResource) error {
	seen := map[string]bool{}
	for _, cost := range costs {
		key := cost.GetId()
		if seen[key] || cost.GetNum() <= 0 || cost.GetGuid() != "" {
			return errors.New("装备材料请求重复或无效")
		}
		seen[key] = true
		if key == "coin" {
			if player == nil || player.GetCoin() < int64(cost.GetNum()) {
				return errors.New("装备进阶金币不足")
			}
			player.Coin = proto.Int64(player.GetCoin() - int64(cost.GetNum()))
			continue
		}
		if !strings.HasPrefix(key, "item_") || s.table("Item", "itemId", key) == nil {
			return errors.New("装备进阶材料配置无效")
		}
		var owned *pb.SimpleItem
		for _, item := range g.Items {
			if item.GetId() == key {
				if owned != nil {
					return errors.New("装备材料存档重复")
				}
				owned = item
			}
		}
		if owned == nil || owned.GetNum() < cost.GetNum() {
			return errors.New("未拥有足够装备进阶材料")
		}
		owned.Num = proto.Int32(owned.GetNum() - cost.GetNum())
	}
	var remaining []*pb.SimpleItem
	for _, item := range g.Items {
		if item.GetNum() != 0 {
			remaining = append(remaining, item)
		}
	}
	g.Items = remaining
	return nil
}

func (s *GameService) equipmentRefineLevels(equip *pb.Equipment) ([]int64, error) {
	quality := s.table("Equip", "id", equip.GetEquipId())["quality"]
	var count int
	for _, row := range s.Tables["EquipRefineExp"] {
		if row["quality"] == quality {
			count++
		}
	}
	maximum := tableNumber(s.table("GameConfigure", "StaticName", "EquipMaxRefineValue"), "NameValue")
	if count == 0 || float64(count) != maximum {
		return nil, errors.New("装备精炼上限配置无效")
	}
	levels := make([]int64, count)
	for _, row := range s.Tables["EquipRefineExp"] {
		if row["quality"] != quality {
			continue
		}
		level, err := strconv.Atoi(row["level"])
		experience, expErr := strconv.ParseInt(row["refineExp"], 10, 32)
		if err != nil || expErr != nil || level < 1 || level > count || experience <= 0 || levels[level-1] != 0 {
			return nil, errors.New("装备精炼经验配置无效")
		}
		levels[level-1] = experience
	}
	for _, experience := range levels {
		if experience == 0 {
			return nil, errors.New("装备精炼经验配置缺档")
		}
	}
	return levels, nil
}

func (s *GameService) equipmentStarRow(position string, star int32) map[string]string {
	for _, row := range s.Tables["EquipStar"] {
		if row["pos"] == position && row["starLevel"] == strconv.FormatInt(int64(star), 10) {
			return row
		}
	}
	return nil
}

// equipmentAdvancedProperties 与 UIEquipment 相同地累加精炼成长、传奇静态加成和当前星级加成。
func (s *GameService) equipmentAdvancedProperties(equip *pb.Equipment, values map[string]float64) error {
	row := s.table("Equip", "id", equip.GetEquipId())
	if err := s.equipmentForgeProperties(equip, values); err != nil {
		return err
	}
	if equip.GetRefineLv() > 0 || equip.GetRefineExp() > 0 {
		levels, err := s.equipmentRefineLevels(equip)
		if err != nil || int(equip.GetRefineLv()) > len(levels) || (int(equip.GetRefineLv()) < len(levels) && int64(equip.GetRefineExp()) >= levels[equip.GetRefineLv()]) {
			return errors.New("装备精炼存档无效")
		}
		var refine map[string]string
		for _, candidate := range s.Tables["EquipRefine"] {
			if candidate["quality"] == row["quality"] && candidate["type"] == row["type"] {
				refine = candidate
			}
		}
		if refine == nil {
			return errors.New("装备精炼属性配置缺失")
		}
		for key, field := range map[string]string{"hp": "hpGrow", "att": "attGrow", "pdef": "pdefGrow", "mdef": "mdefGrow", "cri": "criGrow", "dodge": "perevadeGrow", "hit": "perhitGrow", "antiCri": "anticriGrow"} {
			values[key] += tableNumber(refine, field) * float64(equip.GetRefineLv())
		}
		for _, skill := range s.Tables["EquipSkill"] {
			if skill["equipId"] == equip.GetEquipId() && tableNumber(skill, "refineLevel") <= float64(equip.GetRefineLv()) {
				if skill["talentID"] != "" {
					return errors.New("装备传奇动态天赋尚未恢复")
				}
				if skill["bonus"] != "" {
					if err := equipmentPropertyString(values, skill["bonus"], false); err != nil {
						return err
					}
				}
			}
		}
	}
	if equip.GetStar() > 0 {
		star := s.equipmentStarRow(row["type"], equip.GetStar())
		if star == nil || tableNumber(row, "quality") < 20 {
			return errors.New("装备升星属性或资质无效")
		}
		return equipmentPropertyString(values, star["propAdd"], false)
	}
	return nil
}

// equipmentFormationPercent 四槽同套装按最低星级选取唯一档，加入百分比乘算前的属性集合。
func (s *GameService) equipmentFormationPercent(g *model.GameState, slot *pb.FormationItem, percent map[string]float64) error {
	positions := map[int32]bool{}
	outfit := ""
	minimum := int32(math.MaxInt32)
	for _, thing := range slot.GetThings() {
		if thing.GetPos() < 1 || thing.GetPos() > 4 {
			continue
		}
		equip, err := s.equipmentRecord(g, thing.GetGuid())
		if err != nil {
			return err
		}
		row := s.table("Equip", "id", equip.GetEquipId())
		if positions[thing.GetPos()] || (outfit != "" && outfit != row["outfitId"]) {
			return nil
		}
		positions[thing.GetPos()] = true
		outfit = row["outfitId"]
		minimum = min(minimum, equip.GetStar())
	}
	if len(positions) != 4 || minimum <= 0 || outfit == "" {
		return nil
	}
	for _, row := range s.Tables["EquipStarSuitProp"] {
		if row["suitId"] == outfit && tableNumber(row, "starLevel") == float64(minimum) {
			return equipmentPropertyString(percent, row["suitAdd"], true)
		}
	}
	return nil
}

func equipmentPropertyString(values map[string]float64, content string, percentage bool) error {
	for _, part := range strings.Split(content, ";") {
		pair := strings.Split(part, ",")
		if len(pair) != 2 {
			return errors.New("装备进阶属性格式无效")
		}
		key := pair[0]
		if percentage {
			key = map[string]string{"perhp": "hp", "peratt": "att", "perpdef": "pdef", "permdef": "mdef"}[key]
		} else if key != "hp" && key != "att" && key != "pdef" && key != "mdef" {
			return errors.New("装备进阶属性类型尚未恢复")
		}
		value, err := strconv.ParseFloat(pair[1], 64)
		if key == "" || err != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return errors.New("装备进阶属性数值无效")
		}
		if percentage {
			value *= .01
		}
		values[key] += value
	}
	return nil
}
