package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/google/uuid"
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
)

// treasureRecord 核对实例所有权及原表等级边界。
func (s *GameService) treasureRecord(g *model.GameState, guid string) (*pb.Treasure, error) {
	var found *pb.Treasure
	for _, t := range g.Treasures {
		if t.GetGuid() == guid {
			if found != nil || guid == "" {
				return nil, errors.New("饰品GUID重复")
			}
			found = t
		}
	}
	if found == nil {
		return nil, errors.New("未拥有指定饰品")
	}
	row := s.table("Treasure", "id", found.GetTreasureId())
	if row == nil || found.GetStrengthLevel() < 1 || found.GetRefineLevel() < 0 || found.GetCurrentExp() < 0 || (tableNumber(row, "type") >= 0 && float64(found.GetStrengthLevel()) > tableNumber(row, "maxStrengthen")) || float64(found.GetRefineLevel()) > tableNumber(row, "maxRefine") {
		return nil, errors.New("饰品存档无效")
	}
	return found, nil
}
func treasureMounted(g *model.GameState, guid string) bool {
	for _, slot := range g.Formation {
		for _, thing := range slot.GetThings() {
			if thing.GetGuid() == guid {
				return true
			}
		}
	}
	return false
}

// treasureCombine 按 TableFrag 的不同片种逐项扣除，生成原协议实例。
func (s *GameService) treasureCombine(ctx context.Context, id int64, b *pb.CSCombine, stamp int64) ([]proto.Message, error) {
	var result *pb.SCCombine
	err := updateProfile(ctx, id, func(player *pb.Player, g *model.GameState) error {
		if receipt := g.TreasureCombineReceipt; receipt != nil && stamp != 0 && stamp <= receipt.Timestamp {
			if stamp < receipt.Timestamp {
				return errors.New("合成请求已过期")
			}
			if receipt.TargetID != b.GetTargetId() || receipt.Response == nil {
				return errors.New("合成请求时间重复")
			}
			if err := s.validateP5GrowthReceipt(g, receipt.Response); err != nil {
				return err
			}
			result = proto.Clone(receipt.Response).(*pb.SCCombine)
			return nil
		}
		row := s.table("Treasure", "id", b.GetTargetId())
		if row == nil {
			return errors.New("合成目标尚未恢复")
		}
		var costs []*pb.BaseResource
		var owned []*pb.SimpleItem
		for _, frag := range s.Tables["Frag"] {
			if frag["target"] != b.GetTargetId() {
				continue
			}
			num := int32(tableNumber(frag, "num"))
			if num <= 0 {
				return errors.New("碎片配置无效")
			}
			var item *pb.SimpleItem
			for _, candidate := range g.Items {
				if candidate.GetId() == frag["id"] {
					if item != nil {
						return errors.New("碎片存档重复")
					}
					item = candidate
				}
			}
			if item == nil || item.GetNum() < num {
				return errors.New("饰品碎片不足")
			}
			owned = append(owned, item)
			costs = append(costs, &pb.BaseResource{Id: proto.String(frag["id"]), Num: proto.Int32(num)})
		}
		if len(costs) == 0 || len(costs) != int(tableNumber(row, "needChipNum")) {
			return errors.New("饰品碎片配置缺失")
		}
		if err := treasureBagCapacity(player, g, 1); err != nil {
			return err
		}
		for i, item := range owned {
			item.Num = proto.Int32(item.GetNum() - costs[i].GetNum())
		}
		t := &pb.Treasure{Guid: proto.String(uuid.NewString()), TreasureId: proto.String(b.GetTargetId()), StrengthLevel: proto.Int32(1), RefineLevel: proto.Int32(0), CurrentExp: proto.Int32(0)}
		g.Treasures = append(g.Treasures, t)
		result = &pb.SCCombine{Get: &pb.ResultObject{Treas: []*pb.Treasure{proto.Clone(t).(*pb.Treasure)}}, Cost: costs}
		g.TreasureCombineReceipt = &model.TreasureCombineReceipt{Timestamp: stamp, TargetID: b.GetTargetId(), Response: proto.Clone(result).(*pb.SCCombine)}
		return nil
	})
	return []proto.Message{result}, err
}

// treasureFormationOn 将戒指映射到5，项链映射到6，替换实例仍保留在背包。
func (s *GameService) treasureFormationOn(ctx context.Context, id int64, b *pb.CSFormationOn) ([]proto.Message, error) {
	var result *pb.SCFormationOn
	err := updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error {
		current := &pb.SCFormationOn{}
		if err := s.treasureFormationOnApply(p, g, b, current); err != nil {
			return err
		}
		result = current
		return nil
	})
	return []proto.Message{result}, err
}

// treasureFormationOnApply shares the original type, ownership and slot checks with durable frames.
func (s *GameService) treasureFormationOnApply(p *pb.Player, g *model.GameState, b *pb.CSFormationOn, result *pb.SCFormationOn) error {
	t, err := s.treasureRecord(g, b.GetGuid())
	if err != nil {
		return err
	}
	row := s.table("Treasure", "id", t.GetTreasureId())
	if b.GetId() != t.GetTreasureId() || tableNumber(row, "type") < 0 {
		return errors.New("饰品ID或类型无效")
	}
	if float64(p.GetLevel()) < tableNumber(s.table("GameConfigure", "StaticName", "RobTreasureLevel"), "NameValue") {
		return errors.New("尚未达到饰品穿戴等级")
	}
	if err = s.PrepareFormation(g); err != nil {
		return err
	}
	if err = s.formationPosition(p, g, b.GetFormatPos()); err != nil {
		return err
	}
	var target *pb.FormationItem
	for _, slot := range g.Formation {
		if slot.GetFormatPos() == b.GetFormatPos() {
			target = slot
		}
		for _, thing := range slot.GetThings() {
			if thing.GetGuid() == b.GetGuid() && slot.GetFormatPos() != b.GetFormatPos() {
				return errors.New("饰品已在其他阵位")
			}
		}
	}
	if formationHero(target) == nil {
		return errors.New("请先上阵角色")
	}
	position := int32(6 - tableNumber(row, "type"))
	var things []*pb.FormationThing
	for _, thing := range target.GetThings() {
		if thing.GetPos() != position {
			things = append(things, thing)
		}
	}
	target.Things = append(things, &pb.FormationThing{Pos: proto.Int32(position), Id: proto.String(t.GetTreasureId()), Guid: proto.String(t.GetGuid())})
	power, err := s.formationFightPower(p, g)
	if err != nil {
		return err
	}
	*result = pb.SCFormationOn{FormationItem: proto.Clone(target).(*pb.FormationItem), Fightpower: proto.Int32(power)}
	return nil
}
func (s *GameService) treasureFormationOff(ctx context.Context, id int64, b *pb.CSFormationOff) ([]proto.Message, error) {
	var result *pb.SCFormationOff
	err := updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error {
		current := &pb.SCFormationOff{}
		if err := s.treasureFormationOffApply(p, g, b, current); err != nil {
			return err
		}
		result = current
		return nil
	})
	return []proto.Message{result}, err
}

// treasureFormationOffApply shares the original type, ownership and slot checks with durable frames.
func (s *GameService) treasureFormationOffApply(p *pb.Player, g *model.GameState, b *pb.CSFormationOff, result *pb.SCFormationOff) error {
	t, err := s.treasureRecord(g, b.GetGuid())
	if err != nil {
		return err
	}
	if t.GetTreasureId() != b.GetId() {
		return errors.New("饰品ID不匹配")
	}
	removed := false
	for _, slot := range g.Formation {
		if slot.GetFormatPos() != b.GetFormatPos() {
			continue
		}
		var things []*pb.FormationThing
		for _, thing := range slot.GetThings() {
			if thing.GetGuid() == b.GetGuid() && thing.GetId() == b.GetId() && (thing.GetPos() == 5 || thing.GetPos() == 6) {
				removed = true
			} else {
				things = append(things, thing)
			}
		}
		slot.Things = things
	}
	if !removed {
		return errors.New("指定饰品已卸下")
	}
	power, err := s.formationFightPower(p, g)
	if err != nil {
		return err
	}
	*result = pb.SCFormationOff{Fightpower: proto.Int32(power)}
	return nil
}

// p5TreasureFormationOn binds the original wear frame without repeating a later
// replacement. Both entry points execute the same original treasure rules.
func (s *GameService) p5TreasureFormationOn(ctx context.Context, id int64, request *pb.CSFormationOn, stamp int64) ([]proto.Message, error) {
	out := &pb.SCFormationOn{}
	return s.p5Mutation(ctx, id, constUtil.P5FormationOnOperation, stamp, request, out, func(_ *gorm.DB, p *pb.Player, g *model.GameState) error {
		return s.treasureFormationOnApply(p, g, request, out)
	})
}

func (s *GameService) p5TreasureFormationOff(ctx context.Context, id int64, request *pb.CSFormationOff, stamp int64) ([]proto.Message, error) {
	out := &pb.SCFormationOff{}
	return s.p5Mutation(ctx, id, constUtil.P5TreasureFormationOffOperation, stamp, request, out, func(_ *gorm.DB, p *pb.Player, g *model.GameState) error {
		return s.treasureFormationOffApply(p, g, request, out)
	})
}

// validateP5TreasureFormationOffReceipt protects the original callback's lookup
// of its captured GUID and clearing of the captured necklace/ring position.
func (s *GameService) validateP5TreasureFormationOffReceipt(g *model.GameState, request, response proto.Message) error {
	if _, ok := response.(*pb.SCFormationOff); !ok {
		return nil
	}
	original, ok := request.(*pb.CSFormationOff)
	if !ok || !strings.HasPrefix(original.GetId(), "trea_") {
		return nil
	}
	treasure, err := s.treasureRecord(g, original.GetGuid())
	if err != nil || treasure.GetTreasureId() != original.GetId() {
		return errors.New(constUtil.P5GrowthReceiptChanged)
	}
	position := int32(6 - tableNumber(s.table("Treasure", "id", treasure.GetTreasureId()), "type"))
	var current *pb.FormationItem
	for _, slot := range g.Formation {
		if slot.GetFormatPos() == original.GetFormatPos() {
			if current != nil {
				return errors.New(constUtil.P5GrowthReceiptChanged)
			}
			current = slot
		}
	}
	if current == nil || position < 5 || position > 6 {
		return errors.New(constUtil.P5GrowthReceiptChanged)
	}
	for _, thing := range current.GetThings() {
		if thing.GetPos() == position {
			return errors.New(constUtil.P5GrowthReceiptChanged)
		}
	}
	return nil
}

func (s *GameService) treasureLevelExp(t *pb.Treasure, level int32) (int64, error) {
	quality := s.table("Treasure", "id", t.GetTreasureId())["quality"]
	for _, row := range s.Tables["TreasureStrengthLevelExp"] {
		if row["quality"] == quality && tableNumber(row, "level") == float64(level) {
			n, err := strconv.ParseInt(row["exp"], 10, 32)
			if err != nil || n <= 0 {
				return 0, errors.New("饰品经验配置无效")
			}
			return n, nil
		}
	}
	return 0, errors.New("饰品经验配置缺失")
}

// treasureStrengthen 保留被消耗实例已投入经验，金币系数沿用原配置。
func (s *GameService) treasureStrengthen(ctx context.Context, id int64, b *pb.CSTreasureStrengthen) ([]proto.Message, error) {
	result := &pb.SCTreasureStrengthen{}
	err := updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error {
		return s.treasureStrengthenApply(p, g, b, result)
	})
	return []proto.Message{result}, err
}

// treasureStrengthenApply 保留原材料GUID、跨层经验、金币与封顶规则。
func (s *GameService) treasureStrengthenApply(p *pb.Player, g *model.GameState, b *pb.CSTreasureStrengthen, result *pb.SCTreasureStrengthen) error {
	t, err := s.treasureRecord(g, b.GetTargetId())
	if err != nil {
		return err
	}
	maximum := int32(tableNumber(s.table("Treasure", "id", t.GetTreasureId()), "maxStrengthen"))
	if t.GetStrengthLevel() >= maximum || len(b.GetGuids()) == 0 {
		return errors.New("饰品强化已满级或缺少材料")
	}
	seen := map[string]bool{}
	var total int64
	var costs []*pb.BaseResource
	for _, guid := range b.GetGuids() {
		if guid == t.GetGuid() || seen[guid] || treasureMounted(g, guid) {
			return errors.New("不能消耗自身、重复或已穿戴饰品")
		}
		seen[guid] = true
		material, err := s.treasureRecord(g, guid)
		if err != nil {
			return err
		}
		if material.GetRefineLevel() != 0 {
			return errors.New("请先重生已精炼饰品")
		}
		exp := int64(tableNumber(s.table("Treasure", "id", material.GetTreasureId()), "exp")) + int64(material.GetCurrentExp())
		for lv := int32(1); lv < material.GetStrengthLevel(); lv++ {
			n, err := s.treasureLevelExp(material, lv)
			if err != nil {
				return err
			}
			exp += n
		}
		if exp <= 0 {
			return errors.New("饰品材料经验无效")
		}
		total += exp
		costs = append(costs, &pb.BaseResource{Id: proto.String(material.GetTreasureId()), Guid: proto.String(guid), Num: proto.Int32(1)})
	}
	coin := int64(float64(total) * tableNumber(s.table("GameConfigure", "StaticName", "TreasureStrengthCostExpToCoin"), "NameValue"))
	if coin <= 0 || coin > math.MaxInt32 || total > math.MaxInt32 || p.GetCoin() < coin {
		return errors.New("饰品强化金币不足或经验溢出")
	}
	exp := int64(t.GetCurrentExp()) + total
	lv := t.GetStrengthLevel()
	for lv < maximum {
		need, err := s.treasureLevelExp(t, lv)
		if err != nil {
			return err
		}
		if exp < need {
			break
		}
		exp -= need
		lv++
	}
	// 原UI按maxStrengthen封顶等级，并读取非零currentExp后截显示条。
	// 保存真实跨层余经验，原材料整份经验无需恰好命中最后一阶门槛。
	p.Coin = proto.Int64(p.GetCoin() - coin)
	t.StrengthLevel = proto.Int32(lv)
	t.CurrentExp = proto.Int32(int32(exp))
	var remaining []*pb.Treasure
	for _, item := range g.Treasures {
		if !seen[item.GetGuid()] {
			remaining = append(remaining, item)
		}
	}
	g.Treasures = remaining
	costs = append(costs, &pb.BaseResource{Id: proto.String("coin"), Num: proto.Int32(int32(coin))})
	*result = pb.SCTreasureStrengthen{Treasure: proto.Clone(t).(*pb.Treasure), Cost: costs}
	return nil
}

// p5TreasureStrengthen 在已有玩家事务内保存提交结果，原材料已消耗的重试仍能恢复当前回执。
func (s *GameService) p5TreasureStrengthen(ctx context.Context, id int64, b *pb.CSTreasureStrengthen, stamp int64) ([]proto.Message, error) {
	out := &pb.SCTreasureStrengthen{}
	return s.p5Mutation(ctx, id, constUtil.P5TreasureStrengthenOperation, stamp, b, out, func(_ *gorm.DB, p *pb.Player, g *model.GameState) error {
		return s.treasureStrengthenApply(p, g, b, out)
	})
}

// treasureRefine 从下一等级原表读取成本，完整校验后扣除材料及同名零进阶实例。
func (s *GameService) treasureRefine(ctx context.Context, id int64, b *pb.CSTreasureRefine, stamp int64) ([]proto.Message, error) {
	var result *pb.SCTreasureRefine
	err := updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error {
		if receipt := g.TreasureRefineReceipts[b.GetTargetId()]; receipt != nil && stamp != 0 && stamp <= receipt.Timestamp {
			if stamp < receipt.Timestamp {
				return errors.New("精炼请求已过期")
			}
			if receipt.Response == nil {
				return errors.New("精炼回执无效")
			}
			if err := s.validateP5GrowthReceipt(g, receipt.Response); err != nil {
				return err
			}
			result = proto.Clone(receipt.Response).(*pb.SCTreasureRefine)
			return nil
		}
		t, err := s.treasureRecord(g, b.GetTargetId())
		if err != nil {
			return err
		}
		next := t.GetRefineLevel() + 1
		row := s.table("Treasure", "id", t.GetTreasureId())
		if float64(next) > tableNumber(row, "maxRefine") {
			return errors.New("饰品精炼已满级")
		}
		var costRow map[string]string
		for _, r := range s.Tables["TreasureRefineLevel"] {
			if r["treasureId"] == t.GetTreasureId() && tableNumber(r, "level") == float64(next) {
				costRow = r
			}
		}
		if costRow == nil {
			return errors.New("饰品精炼配置缺失")
		}
		removed := map[string]bool{}
		var costs, stack []*pb.BaseResource
		for _, part := range strings.Split(costRow["objStr"], ";") {
			pair := strings.Split(part, ",")
			if len(pair) != 2 {
				return errors.New("饰品精炼费用无效")
			}
			num, e := strconv.ParseInt(pair[1], 10, 32)
			if e != nil || num <= 0 {
				return errors.New("饰品精炼费用无效")
			}
			if strings.HasPrefix(pair[0], "trea_") {
				for _, material := range g.Treasures {
					if num == 0 {
						break
					}
					if material.GetGuid() != t.GetGuid() && !removed[material.GetGuid()] && material.GetTreasureId() == pair[0] && material.GetStrengthLevel() == 1 && material.GetRefineLevel() == 0 && material.GetCurrentExp() == 0 && !treasureMounted(g, material.GetGuid()) {
						if _, err := s.treasureRecord(g, material.GetGuid()); err != nil {
							return err
						}
						removed[material.GetGuid()] = true
						costs = append(costs, &pb.BaseResource{Id: proto.String(pair[0]), Guid: proto.String(material.GetGuid()), Num: proto.Int32(1)})
						num--
					}
				}
				if num != 0 {
					return errors.New("精炼所需同名零进阶饰品不足")
				}
			} else {
				c := &pb.BaseResource{Id: proto.String(pair[0]), Num: proto.Int32(int32(num))}
				stack = append(stack, c)
				costs = append(costs, c)
			}
		}
		probe := proto.Clone(t).(*pb.Treasure)
		probe.RefineLevel = proto.Int32(next)
		if err = s.treasureProperties(probe, map[string]float64{}, map[string]float64{}); err != nil {
			return err
		}
		if err = s.equipmentConsume(g, p, stack); err != nil {
			return err
		}
		var remaining []*pb.Treasure
		for _, item := range g.Treasures {
			if !removed[item.GetGuid()] {
				remaining = append(remaining, item)
			}
		}
		g.Treasures = remaining
		t.RefineLevel = proto.Int32(next)
		result = &pb.SCTreasureRefine{Treasure: proto.Clone(t).(*pb.Treasure), Cost: costs}
		if g.TreasureRefineReceipts == nil {
			g.TreasureRefineReceipts = map[string]*model.TreasureRefineReceipt{}
		}
		g.TreasureRefineReceipts[b.GetTargetId()] = &model.TreasureRefineReceipt{Timestamp: stamp, Response: proto.Clone(result).(*pb.SCTreasureRefine)}
		return nil
	})
	return []proto.Message{result}, err
}

// treasureProperties 对齐 UITreasure 的固定成长、百分比成长和累积技能属性。
func (s *GameService) treasureProperties(t *pb.Treasure, values, percent map[string]float64) error {
	row := s.table("Treasure", "id", t.GetTreasureId())
	strength := s.table("TreasureStrengthen", "treasureId", t.GetTreasureId())
	refine := s.table("TreasureRefine", "treasureId", t.GetTreasureId())
	if row == nil || strength == nil || refine == nil || tableNumber(row, "type") < 0 {
		return errors.New("饰品属性配置无效")
	}
	for from, to := range map[string]string{"hp": "hp", "att": "att", "cri": "cri", "anticri": "antiCri", "perhit": "hit", "perevade": "dodge"} {
		values[to] += tableNumber(row, from) + tableNumber(strength, from+"Grow")*float64(t.GetStrengthLevel()-1)
		if from != "hp" && from != "att" {
			values[to] += tableNumber(refine, from+"Grow") * float64(t.GetRefineLevel())
		}
	}
	percent["att"] += tableNumber(refine, "perAttGrow") * float64(t.GetRefineLevel()) * .01
	percent["hp"] += tableNumber(refine, "perHpGrow") * float64(t.GetRefineLevel()) * .01
	for _, skill := range s.Tables["TreasureSkill"] {
		if skill["treaId"] != t.GetTreasureId() || tableNumber(skill, "refineLevel") > float64(t.GetRefineLevel()) {
			continue
		}
		if skill["buffId"] != "" || skill["talentId"] != "" {
			return errors.New("饰品触发技能尚未恢复")
		}
		if skill["bonus"] != "" {
			pair := strings.Split(skill["bonus"], ",")
			if len(pair) != 2 {
				return errors.New("饰品技能属性配置无效")
			}
			n, e := strconv.ParseFloat(pair[1], 64)
			if e != nil || n < 0 || math.IsNaN(n) || math.IsInf(n, 0) {
				return errors.New("饰品技能属性配置无效")
			}
			switch pair[0] {
			case "hp", "att", "pdef", "mdef", "cri", "antiCri", "hit", "dodge", "damageAdd", "damageReduce":
				values[pair[0]] += n
			default:
				return fmt.Errorf("饰品技能属性未恢复: %s", pair[0])
			}
		}
	}
	return nil
}
func (s *GameService) treasureFormationProperties(g *model.GameState, slot *pb.FormationItem, values, percent map[string]float64) error {
	seen := map[int32]bool{}
	guids := map[string]bool{}
	for _, candidate := range g.Formation {
		for _, thing := range candidate.GetThings() {
			if thing.GetPos() == 5 || thing.GetPos() == 6 {
				if guids[thing.GetGuid()] {
					return errors.New("饰品重复挂载")
				}
				guids[thing.GetGuid()] = true
			}
		}
	}
	for _, thing := range slot.GetThings() {
		if thing.GetPos() != 5 && thing.GetPos() != 6 {
			continue
		}
		t, err := s.treasureRecord(g, thing.GetGuid())
		if err != nil {
			return err
		}
		row := s.table("Treasure", "id", t.GetTreasureId())
		if seen[thing.GetPos()] || thing.GetId() != t.GetTreasureId() || int32(6-tableNumber(row, "type")) != thing.GetPos() {
			return errors.New("饰品挂载类型无效")
		}
		seen[thing.GetPos()] = true
		if err = s.treasureProperties(t, values, percent); err != nil {
			return err
		}
	}
	return nil
}

// treasureFragmentList 从兼容库存投影原协议各片种，列表请求不创建资源。
func (s *GameService) treasureFragmentList(ctx context.Context, id int64) ([]proto.Message, error) {
	var result *pb.SCFragmentList
	err := updateProfile(ctx, id, func(_ *pb.Player, g *model.GameState) error {
		result = &pb.SCFragmentList{}
		for _, item := range g.Items {
			if item.GetNum() <= 0 {
				continue
			}
			switch {
			case strings.HasPrefix(item.GetId(), "frag_trea_"):
				result.FragTreasureList = append(result.FragTreasureList, item)
			case strings.HasPrefix(item.GetId(), "frag_equip_"):
				result.FragEquipmentList = append(result.FragEquipmentList, item)
			case strings.HasPrefix(item.GetId(), "frag_hero_"):
				result.FragWarriorList = append(result.FragWarriorList, item)
			case strings.HasPrefix(item.GetId(), "frag_pet_"):
				result.FragPetList = append(result.FragPetList, item)
			}
		}
		return nil
	})
	return []proto.Message{result}, err
}

// InitializeTreasureBoxes 首次初始化SQL本地规则，运营修改在重启后保留。
func (s *GameService) InitializeTreasureBoxes(ctx context.Context) error {
	if err := db.GetIns().WithContext(ctx).AutoMigrate(&db.TreasureBoxRule{}, &db.TreasureBoxReward{}); err != nil {
		return err
	}
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		for _, key := range []string{"item_022", "item_024", "item_025", "item_115", "item_116", "item_117"} {
			rule := db.TreasureBoxRule{ItemID: key, Enabled: true, Mode: constUtil.TreasureBoxModeWeight, MaxBatch: constUtil.TreasureBoxMaximumBatch, Source: "APK TableItemUse reward+odds; local normalized single draw, original denominator unknown"}
			created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rule)
			if created.Error != nil {
				return created.Error
			}
			if created.RowsAffected == 0 {
				continue
			}
			for _, row := range s.Tables["ItemUse"] {
				if row["itemId"] != key {
					continue
				}
				pair := strings.Split(row["result"], ",")
				id, err := strconv.ParseInt(row["id"], 10, 64)
				weight, e := strconv.ParseInt(row["odds"], 10, 64)
				if len(pair) != 2 || err != nil || e != nil {
					return errors.New("饰品箱初始奖池无效")
				}
				amount, e := strconv.ParseInt(pair[1], 10, 32)
				if e != nil {
					return e
				}
				reward := db.TreasureBoxReward{ItemID: key, ID: id, ResourceID: pair[0], Amount: int32(amount), Weight: weight, Source: "APK TableItemUse row " + row["id"]}
				if e = tx.Create(&reward).Error; e != nil {
					return e
				}
			}
		}
		return nil
	})
}

// treasureSourceGain 仅接受原表饰品和碎片，并在调用者存档事务内生成全局唯一实例。
func (s *GameService) treasureSourceGain(player *pb.Player, g *model.GameState, result *pb.ResultObject, key string, num int32) error {
	if num <= 0 {
		return errors.New("箱子奖励数量无效")
	}
	if row := s.table("Treasure", "id", key); row != nil {
		if num > constUtil.TreasureBoxMaximumBatch {
			return errors.New("箱子实例数量超限")
		}
		if err := treasureBagCapacity(player, g, num); err != nil {
			return err
		}
		for i := int32(0); i < num; i++ {
			t := &pb.Treasure{Guid: proto.String(uuid.NewString()), TreasureId: proto.String(key), StrengthLevel: proto.Int32(1), RefineLevel: proto.Int32(0), CurrentExp: proto.Int32(0)}
			g.Treasures = append(g.Treasures, t)
			result.Treas = append(result.Treas, proto.Clone(t).(*pb.Treasure))
		}
		return nil
	}
	if s.table("Frag", "id", key) == nil || !strings.HasPrefix(key, "frag_trea_") {
		return errors.New("饰品箱奖励配置尚未恢复")
	}
	var found *pb.SimpleItem
	for _, item := range g.Items {
		if item.GetId() == key {
			if found != nil {
				return errors.New("碎片存档重复")
			}
			found = item
		}
	}
	if found == nil {
		found = &pb.SimpleItem{Id: proto.String(key), Num: proto.Int32(0)}
		g.Items = append(g.Items, found)
	}
	if found.GetNum() < 0 || int64(found.GetNum())+int64(num) > math.MaxInt32 {
		return errors.New("碎片库存溢出")
	}
	found.Num = proto.Int32(found.GetNum() + num)
	for _, item := range result.Simps {
		if item.GetId() == key {
			item.Num = proto.Int32(item.GetNum() + num)
			return nil
		}
	}
	result.Simps = append(result.Simps, &pb.SimpleItem{Id: proto.String(key), Num: proto.Int32(num)})
	return nil
}

// treasureBoxUse 检查所有原奖励后按SQL明确的归一权重单抽，扣箱与发奖原子保存。
func (s *GameService) treasureBoxUse(ctx context.Context, id int64, b *pb.CSItemUse, stamp int64) ([]proto.Message, error) {
	var result *pb.SCItemUse
	// 同一SQL事务锁定存档并读取当前运营规则，保持扣箱、实例和回执一致。
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var profile db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
			return err
		}
		player, g, err := decodeProfileForCalendarUpdate(tx, &profile)
		if err != nil {
			return err
		}
		if receipt := g.TreasureBoxReceipt; receipt != nil && stamp != 0 && stamp <= receipt.Timestamp {
			if stamp < receipt.Timestamp {
				return errors.New("饰品箱请求已过期")
			}
			if !proto.Equal(receipt.Request, b) || receipt.Response == nil {
				return errors.New("饰品箱请求时间重复")
			}
			if err := s.validateP5GrowthReceipt(g, receipt.Response); err != nil {
				return err
			}
			result = proto.Clone(receipt.Response).(*pb.SCItemUse)
			return nil
		}
		var rule db.TreasureBoxRule
		if err := tx.Where("item_id = ?", b.GetCostItem().GetId()).Take(&rule).Error; err != nil {
			return errors.New("饰品箱规则尚未启用")
		}
		if !rule.Enabled || rule.Mode != constUtil.TreasureBoxModeWeight || b.GetCostItem().GetNum() < 1 || b.GetCostItem().GetNum() > rule.MaxBatch || rule.MaxBatch > constUtil.TreasureBoxMaximumBatch || b.GetCostItem().GetGuid() != "" || b.GetGetItemIndex() != 0 {
			return errors.New("饰品箱使用参数或SQL规则无效")
		}
		var rows []db.TreasureBoxReward
		if err := tx.Where("item_id = ?", rule.ItemID).Order("id").Find(&rows).Error; err != nil {
			return err
		}
		var total int64
		for _, row := range rows {
			if row.Weight < 0 || row.Weight > math.MaxInt32 || row.Amount < 1 || row.Amount > constUtil.TreasureBoxMaximumBatch || (s.table("Treasure", "id", row.ResourceID) == nil && (s.table("Frag", "id", row.ResourceID) == nil || !strings.HasPrefix(row.ResourceID, "frag_trea_"))) {
				return errors.New("饰品箱SQL奖励无效")
			}
			total += row.Weight
		}
		if total <= 0 || total > math.MaxInt32 {
			return errors.New("饰品箱SQL权重无效")
		}
		var owned *pb.SimpleItem
		for _, item := range g.Items {
			if item.GetId() == rule.ItemID {
				if owned != nil {
					return errors.New("饰品箱库存重复")
				}
				owned = item
			}
		}
		if owned == nil || owned.GetNum() < b.GetCostItem().GetNum() {
			return errors.New("饰品箱数量不足")
		}
		gain := &pb.ResultObject{}
		for count := int32(0); count < b.GetCostItem().GetNum(); count++ {
			dice, e := rand.Int(rand.Reader, big.NewInt(total))
			if e != nil {
				return e
			}
			selected := dice.Int64()
			var winner *db.TreasureBoxReward
			for index := range rows {
				row := &rows[index]
				if selected < row.Weight {
					winner = row
					break
				}
				selected -= row.Weight
			}
			if winner == nil {
				return errors.New("饰品箱抽取失败")
			}
			if e = s.treasureSourceGain(player, g, gain, winner.ResourceID, winner.Amount); e != nil {
				return e
			}
		}
		owned.Num = proto.Int32(owned.GetNum() - b.GetCostItem().GetNum())
		result = &pb.SCItemUse{Cost: proto.Clone(b.GetCostItem()).(*pb.BaseResource), Get: gain}
		g.TreasureBoxReceipt = &model.TreasureBoxReceipt{Timestamp: stamp, Request: proto.Clone(b).(*pb.CSItemUse), Response: proto.Clone(result).(*pb.SCItemUse)}
		return saveProfile(tx, &profile, player, g)
	})
	return []proto.Message{result}, err
}

// treasureBagCapacity 对齐 UIPlayerData.IsEquipFull 和 GetEquipTreasureTotalCount，
// 全部装备与饰品实例共享 Player.equipBagSize，VIP容量已由该原协议字段承载。
func treasureBagCapacity(player *pb.Player, g *model.GameState, incoming int32) error {
	if player == nil || player.GetEquipBagSize() <= 0 || incoming <= 0 || int64(len(g.Equipment))+int64(len(g.Treasures))+int64(incoming) > int64(player.GetEquipBagSize()) {
		return errors.New("装备与饰品共享背包容量不足")
	}
	return nil
}
