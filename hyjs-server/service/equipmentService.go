package service

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"math"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// equipmentRecord 以实例 GUID 查找所有权，配置 ID 不能代替实例校验。
func (s *GameService) equipmentRecord(g *model.GameState, guid string) (*pb.Equipment, error) {
	var found *pb.Equipment
	for _, equip := range g.Equipment {
		if equip.GetGuid() == guid {
			if found != nil || guid == "" {
				return nil, errors.New("装备存档包含重复 GUID")
			}
			found = equip
		}
	}
	if found == nil || s.table("Equip", "id", found.GetEquipId()) == nil || found.GetLevel() < 1 {
		return nil, errors.New("未拥有指定装备或装备存档无效")
	}
	return found, nil
}

// equipmentList 返回完整原协议记录，旧档的精炼、升星等字段继续保留。
func (s *GameService) equipmentList(ctx context.Context, id int64) ([]proto.Message, error) {
	var result *pb.SCEquipmentTreasureList
	err := updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
		result = &pb.SCEquipmentTreasureList{EquipmentList: g.Equipment, TreasureList: g.Treasures}
		return nil
	})
	return []proto.Message{result}, err
}

// syncEquipmentOwners 对齐 UIHeroFormation.InitData 的 lastHeroId，并拒绝重复挂载。
func (s *GameService) syncEquipmentOwners(g *model.GameState) error {
	used := map[string]bool{}
	for _, slot := range g.Formation {
		positions := map[int32]bool{}
		for _, thing := range slot.GetThings() {
			if thing.GetPos() < 1 || thing.GetPos() > 4 {
				continue
			}
			equip, err := s.equipmentRecord(g, thing.GetGuid())
			if err != nil {
				return err
			}
			if equip.GetEquipId() != thing.GetId() || int32(tableNumber(s.table("Equip", "id", thing.GetId()), "type")) != thing.GetPos() || used[thing.GetGuid()] || positions[thing.GetPos()] {
				return errors.New("装备类型与挂载位置不匹配或存在重复挂载")
			}
			used[thing.GetGuid()] = true
			positions[thing.GetPos()] = true
			heroID := ""
			if hero := formationHero(slot); hero != nil {
				heroID = hero.GetId()
			}
			equip.LastHeroId = proto.String(heroID)
		}
	}
	return nil
}

func equipmentBasicState(equip *pb.Equipment) error {
	if equip.GetRefineLv() < 0 || equip.GetRefineLv() > 80 || equip.GetRefineExp() < 0 || equip.GetStar() < 0 || equip.GetStar() > 5 || equip.GetStarExp() != 0 || equip.GetForgeLevel() < 0 || equip.GetForgeLevel() > 15 {
		return errors.New("装备进阶存档无效")
	}
	if len(equip.GetGems()) > 4 {
		return errors.New("装备宝石孔位存档无效")
	}
	return nil
}

// equipmentFormationOn 在英雄阵位上替换同类型装备，原装备继续保留在背包。
func (s *GameService) equipmentFormationOn(ctx context.Context, id int64, request *pb.CSFormationOn) ([]proto.Message, error) {
	var result *pb.SCFormationOn
	err := updateProfile(ctx, id, func(player *pb.Player, g *model.GameState) error {
		current := &pb.SCFormationOn{}
		if err := s.equipmentFormationOnApply(player, g, request, current); err != nil {
			return err
		}
		result = current
		return nil
	})
	return []proto.Message{result}, err
}

// equipmentFormationOnApply shares the unchanged original checks with the frame receipt transaction.
func (s *GameService) equipmentFormationOnApply(player *pb.Player, g *model.GameState, request *pb.CSFormationOn, result *pb.SCFormationOn) error {
	equip, err := s.equipmentRecord(g, request.GetGuid())
	if err != nil {
		return err
	}
	if equip.GetEquipId() != request.GetId() {
		return errors.New("装备 GUID 与配置 ID 不匹配")
	}
	if err = equipmentBasicState(equip); err != nil {
		return err
	}
	if err = s.equipmentAdvancedProperties(equip, map[string]float64{}); err != nil {
		return err
	}
	if err = s.equipmentGemProperties(equip, map[string]float64{}); err != nil {
		return err
	}
	if err = s.PrepareFormation(g); err != nil {
		return err
	}
	if err = s.formationPosition(player, g, request.GetFormatPos()); err != nil {
		return err
	}
	var target *pb.FormationItem
	for _, slot := range g.Formation {
		if slot.GetFormatPos() == request.GetFormatPos() {
			target = slot
		}
		for _, thing := range slot.GetThings() {
			if thing.GetGuid() == request.GetGuid() && slot.GetFormatPos() != request.GetFormatPos() {
				return errors.New("该装备已挂载在其他阵位")
			}
		}
	}
	if formationHero(target) == nil {
		return errors.New("请先在目标阵位上阵角色")
	}
	position := int32(tableNumber(s.table("Equip", "id", equip.GetEquipId()), "type"))
	if position < 1 || position > 4 {
		return errors.New("装备类型配置无效")
	}
	var remaining []*pb.FormationThing
	for _, thing := range target.GetThings() {
		if thing.GetPos() != position {
			remaining = append(remaining, thing)
		}
	}
	target.Things = append(remaining, &pb.FormationThing{Pos: proto.Int32(position), Id: proto.String(equip.GetEquipId()), Guid: proto.String(equip.GetGuid())})
	if err = s.syncEquipmentOwners(g); err != nil {
		return err
	}
	power, err := s.formationFightPower(player, g)
	if err != nil {
		return err
	}
	*result = pb.SCFormationOn{FormationItem: proto.Clone(target).(*pb.FormationItem), Fightpower: proto.Int32(power)}
	return nil
}

// equipmentFormationOff 只卸下请求指定实例，过期请求不能移除后来换入的装备。
func (s *GameService) equipmentFormationOff(ctx context.Context, id int64, request *pb.CSFormationOff) ([]proto.Message, error) {
	var result *pb.SCFormationOff
	err := updateProfile(ctx, id, func(player *pb.Player, g *model.GameState) error {
		current := &pb.SCFormationOff{}
		if err := s.equipmentFormationOffApply(player, g, request, current); err != nil {
			return err
		}
		result = current
		return nil
	})
	return []proto.Message{result}, err
}

// equipmentFormationOffApply shares the unchanged original checks with the frame receipt transaction.
func (s *GameService) equipmentFormationOffApply(player *pb.Player, g *model.GameState, request *pb.CSFormationOff, result *pb.SCFormationOff) error {
	equip, err := s.equipmentRecord(g, request.GetGuid())
	if err != nil {
		return err
	}
	if equip.GetEquipId() != request.GetId() {
		return errors.New("装备 GUID 与配置 ID 不匹配")
	}
	if err = s.PrepareFormation(g); err != nil {
		return err
	}
	var removed bool
	for _, slot := range g.Formation {
		if slot.GetFormatPos() != request.GetFormatPos() {
			continue
		}
		var remaining []*pb.FormationThing
		for _, thing := range slot.GetThings() {
			if thing.GetGuid() == request.GetGuid() && thing.GetId() == request.GetId() && thing.GetPos() >= 1 && thing.GetPos() <= 4 {
				removed = true
			} else {
				remaining = append(remaining, thing)
			}
		}
		slot.Things = remaining
	}
	if !removed {
		return errors.New("指定装备已不在该阵位")
	}
	if err = s.syncEquipmentOwners(g); err != nil {
		return err
	}
	power, err := s.formationFightPower(player, g)
	if err != nil {
		return err
	}
	*result = pb.SCFormationOff{Fightpower: proto.Int32(power)}
	return nil
}

// p5EquipmentFormationOn binds the original frame before applying the same
// equipment rules. Earlier successful frames cannot re-equip a replaced GUID.
func (s *GameService) p5EquipmentFormationOn(ctx context.Context, id int64, request *pb.CSFormationOn, stamp int64) ([]proto.Message, error) {
	out := &pb.SCFormationOn{}
	return s.p5Mutation(ctx, id, constUtil.P5FormationOnOperation, stamp, request, out, func(_ *gorm.DB, p *pb.Player, g *model.GameState) error {
		return s.equipmentFormationOnApply(p, g, request, out)
	})
}

func (s *GameService) p5EquipmentFormationOff(ctx context.Context, id int64, request *pb.CSFormationOff, stamp int64) ([]proto.Message, error) {
	out := &pb.SCFormationOff{}
	return s.p5Mutation(ctx, id, constUtil.P5EquipmentFormationOffOperation, stamp, request, out, func(_ *gorm.DB, p *pb.Player, g *model.GameState) error {
		return s.equipmentFormationOffApply(p, g, request, out)
	})
}

// validateP5EquipmentFormationOffReceipt retains a valid empty-slot retry while
// preventing the old captured callback from clearing a newly worn item.
func (s *GameService) validateP5EquipmentFormationOffReceipt(g *model.GameState, request, response proto.Message) error {
	if _, ok := response.(*pb.SCFormationOff); !ok {
		return nil
	}
	original, ok := request.(*pb.CSFormationOff)
	if !ok || !strings.HasPrefix(original.GetId(), "equip_") {
		return nil
	}
	equip, err := s.equipmentRecord(g, original.GetGuid())
	if err != nil || equip.GetEquipId() != original.GetId() {
		return errors.New(constUtil.P5GrowthReceiptChanged)
	}
	position := int32(tableNumber(s.table("Equip", "id", equip.GetEquipId()), "type"))
	var current *pb.FormationItem
	for _, slot := range g.Formation {
		if slot.GetFormatPos() == original.GetFormatPos() {
			if current != nil {
				return errors.New(constUtil.P5GrowthReceiptChanged)
			}
			current = slot
		}
	}
	if current == nil || position < 1 || position > 4 {
		return errors.New(constUtil.P5GrowthReceiptChanged)
	}
	for _, thing := range current.GetThings() {
		if thing.GetPos() == position {
			return errors.New(constUtil.P5GrowthReceiptChanged)
		}
	}
	return nil
}

// equipmentStrengthen 校验原客户端逐级报价，再将最多五次确定性强化限制在角色上限内。
func (s *GameService) equipmentStrengthen(ctx context.Context, id int64, request *pb.CSEquipStrengthen) ([]proto.Message, error) {
	result := &pb.SCEquipStrengthen{}
	err := updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error {
		return s.equipmentStrengthenMutation(p, g, request, result)
	})
	return []proto.Message{result}, err
}

// equipmentStrengthenStamped binds ordinary quoted strengthening to its original
// frame timestamp, including the two real red stages with the same quoted fee.
func (s *GameService) equipmentStrengthenStamped(ctx context.Context, id int64, request *pb.CSEquipStrengthen, stamp int64) ([]proto.Message, error) {
	result := &pb.SCEquipStrengthen{}
	err := s.equipmentMutation(ctx, id, constUtil.EquipmentOrdinaryStrengthenOpcode, stamp, request, result, func(_ *gorm.DB, p *pb.Player, g *model.GameState) error {
		return s.equipmentStrengthenMutation(p, g, request, result)
	})
	return []proto.Message{result}, err
}

// equipmentStrengthenMutation shares the exact original quote/level calculation
// between domain calls and the authenticated frame's durable receipt transaction.
func (s *GameService) equipmentStrengthenMutation(player *pb.Player, g *model.GameState, request *pb.CSEquipStrengthen, result *pb.SCEquipStrengthen) error {
	if request.GetTimes() < 1 || request.GetTimes() > 5 {
		return errors.New("装备强化次数必须为1至5次")
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
	row := s.table("Equip", "id", equip.GetEquipId())
	limit := tableNumber(s.table("GameConfigure", "StaticName", "playerLevel_Equip_Value"), "NameValue")
	maximum := tableNumber(s.table("GameConfigure", "StaticName", "maxEquipLevel"), "NameValue")
	playerMaximum := tableNumber(s.table("GameConfigure", "StaticName", "maxPlayerLevel"), "NameValue")
	// OnStrengthOnce 比较当前等级与 2.2 倍上限，再加一级；满级玩家可继续升至装备表上限。
	levelLimit := math.Ceil(float64(player.GetLevel()) * limit)
	if float64(player.GetLevel()) >= playerMaximum {
		levelLimit = maximum
	}
	if math.IsNaN(levelLimit) || math.IsInf(levelLimit, 0) || limit <= 0 || maximum <= 0 || playerMaximum <= 0 || player.GetLevel() < 1 || int64(equip.GetLevel())+1 > int64(levelLimit) || float64(equip.GetLevel()+1) > maximum {
		return errors.New("装备已达到等级上限或强化费用配置无效")
	}
	// OnStrengthFive 使用 Max 计算预期终点，可能报出超过角色上限的五级费用。
	// 保留原请求报价校验，只执行可提升的级数；原回调使用响应 cost 扣除实际费用。
	var (
		quotedCost int64
		cost       int64
		actual     int32
	)
	for step := int32(0); step < request.GetTimes(); step++ {
		level := int64(equip.GetLevel()) + int64(step)
		if float64(level+1) > maximum {
			return errors.New("装备强化报价超出装备等级上限")
		}
		var costRow map[string]string
		for _, candidate := range s.Tables["EquipExp"] {
			if candidate["quality"] == row["quality"] && candidate["level"] == strconv.FormatInt(level, 10) {
				costRow = candidate
			}
		}
		stepCost, parseErr := strconv.ParseInt(costRow["coin"], 10, 32)
		if parseErr != nil || stepCost <= 0 || stepCost > math.MaxInt32-quotedCost {
			return errors.New("装备强化费用配置无效")
		}
		quotedCost += stepCost
		if float64(level+1) <= levelLimit {
			cost += stepCost
			actual++
		}
	}
	// 旧费用或伪造报价必须在任何扣费和等级修改前拒绝。
	if len(request.GetCost()) != 1 || request.Cost[0].GetId() != "coin" || request.Cost[0].GetGuid() != "" || int64(request.Cost[0].GetNum()) != quotedCost {
		return errors.New("装备强化费用与当前等级不匹配")
	}
	if player.GetCoin() < cost {
		return errors.New("金币不足")
	}
	player.Coin = proto.Int64(player.GetCoin() - cost)
	equip.Level = proto.Int32(equip.GetLevel() + actual)
	*result = pb.SCEquipStrengthen{Equip: proto.Clone(equip).(*pb.Equipment), Cost: []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(int32(cost))}}, CostTimes: proto.Int32(actual), CriTimes: proto.Int32(0)}
	return nil
}

// equipmentFormationProperties 累加装备、普通套装和最高强化大师档，供百分比乘算前使用。
func (s *GameService) equipmentFormationProperties(g *model.GameState, slot *pb.FormationItem) (*pb.BaseProp, error) {
	values := map[string]float64{}
	outfits := map[string]int{}
	positions := map[int32]bool{}
	minimum := int32(math.MaxInt32)
	refineMinimum := int32(math.MaxInt32)
	for _, thing := range slot.GetThings() {
		if thing.GetPos() < 1 || thing.GetPos() > 4 {
			continue
		}
		equip, err := s.equipmentRecord(g, thing.GetGuid())
		if err != nil {
			return nil, err
		}
		if err = equipmentBasicState(equip); err != nil {
			return nil, err
		}
		row := s.table("Equip", "id", equip.GetEquipId())
		if thing.GetId() != equip.GetEquipId() || int32(tableNumber(row, "type")) != thing.GetPos() || positions[thing.GetPos()] {
			return nil, errors.New("装备挂载位置与类型不匹配")
		}
		positions[thing.GetPos()] = true
		outfits[row["outfitId"]]++
		minimum = min(minimum, equip.GetLevel())
		refineMinimum = min(refineMinimum, equip.GetRefineLv())
		for _, key := range []string{"hp", "att", "pdef", "mdef"} {
			values[key] += tableNumber(row, key+"Base") + tableNumber(row, key+"Grow")*float64(equip.GetLevel()-1)
		}
		if err = s.equipmentAdvancedProperties(equip, values); err != nil {
			return nil, err
		}
	}
	for _, row := range s.Tables["EquipOutfit"] {
		if row["outfitId"] != "" && outfits[row["outfitId"]] >= int(tableNumber(row, "num")) {
			equipmentAddProperty(values, row)
		}
	}
	// 四槽全部装备齐全才达到强化大师，按 IL 只取满足条件的最高档。
	if len(positions) == 4 {
		var master map[string]string
		for _, row := range s.Tables["StrengthenMaster"] {
			if row["type"] == "1" && tableNumber(row, "strengthenLevel") <= float64(minimum) && (master == nil || tableNumber(row, "level") > tableNumber(master, "level")) {
				master = row
			}
		}
		equipmentAddProperty(values, master)
		var refineMaster map[string]string
		for _, row := range s.Tables["StrengthenMaster"] {
			if row["type"] == "2" && tableNumber(row, "strengthenLevel") <= float64(refineMinimum) && (refineMaster == nil || tableNumber(row, "level") > tableNumber(refineMaster, "level")) {
				refineMaster = row
			}
		}
		equipmentAddProperty(values, refineMaster)
	}
	fields := map[string]any{}
	for key, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return nil, errors.New("装备属性配置无效")
		}
		fields[key] = value
	}
	message, err := model.GameMessage("BaseProp", fields)
	if err != nil {
		return nil, err
	}
	return message.(*pb.BaseProp), nil
}

// equipmentFormationGemProperties 单独累积孔位属性，按原 UI 在百分比乘算后追加。
func (s *GameService) equipmentFormationGemProperties(g *model.GameState, slot *pb.FormationItem) (map[string]float64, error) {
	values := map[string]float64{}
	for _, thing := range slot.GetThings() {
		if thing.GetPos() < 1 || thing.GetPos() > 4 {
			continue
		}
		equip, err := s.equipmentRecord(g, thing.GetGuid())
		if err != nil {
			return nil, err
		}
		if err = s.equipmentGemProperties(equip, values); err != nil {
			return nil, err
		}
	}
	return values, nil
}

func equipmentAddProperty(values map[string]float64, row map[string]string) {
	for _, key := range []string{"hp", "att", "pdef", "mdef", "cri"} {
		values[key] += tableNumber(row, key)
	}
	for from, to := range map[string]string{"perhit": "hit", "anticri": "antiCri", "perevade": "dodge", "damadd": "damageAdd", "damsub": "damageReduce"} {
		values[to] += tableNumber(row, from)
	}
}
