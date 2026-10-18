package service

import (
	"context"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func (s *GameService) p9OwnedEquip(pet *pb.Pet, id string) (*pb.PetEquip, error) {
	if id == "" || len(id) > 64 || s.table("Pet", "id", pet.GetId())["quality"] != "23" || len(pet.GetPetEquipList()) != 4 {
		return nil, errors.New("原红色精灵装备实例缺失")
	}
	for _, equip := range pet.GetPetEquipList() {
		if equip.GetEquipId() == id && equip.GetPetGuid() == pet.GetGuid() {
			if s.table("PetEquip", "id", id)["petId"] != pet.GetId() {
				return nil, p5Error()
			}
			return equip, nil
		}
	}
	return nil, errors.New("装备不属于该精灵实例")
}

func (s *GameService) p9EquipStrengthCap(pet *pb.Pet, equip *pb.PetEquip) (int32, error) {
	var maximum int32
	for _, row := range s.Tables["PetEquipStrength"] {
		if row["equipId"] == equip.GetEquipId() {
			level := int32(tableNumber(row, "level"))
			if level > maximum {
				maximum = level
			}
		}
	}
	if maximum < 1 || equip.GetStrengthLevel() < 0 || equip.GetAdvanceLevel() < 0 || equip.GetAwakeLevel() < 0 {
		return 0, p5Error()
	}
	if maximum > pet.GetLevel() {
		maximum = pet.GetLevel()
	}
	if equip.GetAdvanceLevel() == 0 {
		first := s.p9TablePair("PetEquipAdvance", "equipId", equip.GetEquipId(), "level", "1")
		if first == nil {
			return 0, p5Error()
		}
		limit := int32(tableNumber(first, "needStrengthLevel"))
		if limit < 1 {
			return 0, p5Error()
		}
		if maximum > limit {
			maximum = limit
		}
	} else {
		current := s.p9TablePair("PetEquipAdvance", "equipId", equip.GetEquipId(), "level", strconv.Itoa(int(equip.GetAdvanceLevel())))
		if current == nil {
			return 0, p5Error()
		}
		limit := int32(tableNumber(current, "maxStrengthLevel"))
		if limit < 1 {
			return 0, p5Error()
		}
		if maximum > limit {
			maximum = limit
		}
	}
	return maximum, nil
}

func (s *GameService) p9EquipStrength(ctx context.Context, id int64, b *pb.CSPetEquipStrengthOne, stamp int64) ([]proto.Message, error) {
	out := &pb.SCPetEquipStrengthOne{}
	return s.p9NextMutation(ctx, id, "pet-equip-strength", stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		pet, err := p9OwnedPet(tx, id, b.GetPetGuid())
		if err != nil {
			return err
		}
		equip, err := s.p9OwnedEquip(pet, b.GetEquipId())
		if err != nil {
			return err
		}
		if b.GetNum() != 1 && b.GetNum() != 5 {
			return p5Error()
		}
		// 原五次按钮可停在当前精灵/进阶边界，后续费用、保存与回执
		// 继续共用原强化事务，只记录实际执行的阶数。
		steps := b.GetNum()
		if steps == 5 {
			steps, err = s.p9EquipFiveSteps(pet, equip)
			if err != nil {
				return err
			}
		} else {
			cap, err := s.p9EquipStrengthCap(pet, equip)
			if err != nil {
				return err
			}
			if equip.GetStrengthLevel()+steps > cap {
				return errors.New("强化不能越过原精灵等级或进阶限制")
			}
		}
		costs := []*pb.BaseResource{}
		for step := int32(1); step <= steps; step++ {
			row := s.p9TablePair("PetEquipStrength", "equipId", equip.GetEquipId(), "level", strconv.Itoa(int(equip.GetStrengthLevel()+step)))
			if row == nil {
				return p5Error()
			}
			paid, err := s.growthCost(p, g, row["cost"], 1)
			if err != nil {
				return err
			}
			costs = append(costs, paid...)
		}
		equip.StrengthLevel = proto.Int32(equip.GetStrengthLevel() + steps)
		if err = p9SavePet(tx, id, g, pet); err != nil {
			return err
		}
		if err = p9Invest(tx, id, pet.GetGuid(), costs, nil); err != nil {
			return err
		}
		if steps < b.GetNum() {
			var investment db.P9PetInvestment
			if err = tx.First(&investment, "guid=? AND player_id=?", pet.GetGuid(), id).Error; err != nil {
				return err
			}
			if !strings.Contains(investment.Source, constUtil.P9PetEquipFiveSource) {
				if err = tx.Model(&investment).Update("source", investment.Source+";"+constUtil.P9PetEquipFiveSource).Error; err != nil {
					return err
				}
			}
		}
		out.PetEquip = proto.Clone(equip).(*pb.PetEquip)
		out.Cost = costs
		return s.p9RecordCosts(tx, id, "p9:equip-strength:"+strconv.FormatInt(stamp, 10), costs, LocalPlayNow(ctx))
	})
}

// p9EquipStrengthAll applies the published local round-robin policy to original
// four-slot instances. Each next row is validated before checking affordability.
func (s *GameService) p9EquipStrengthAll(ctx context.Context, id int64, b *pb.CSPetEquipStrengthAll, stamp int64) ([]proto.Message, error) {
	out := &pb.SCPetEquipStrengthAll{}
	return s.p9NextMutation(ctx, id, "pet-equip-strength-all", stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		pet, err := p9OwnedPet(tx, id, b.GetPetGuid())
		if err != nil {
			return err
		}
		caps := map[string]int32{}
		for _, equip := range pet.GetPetEquipList() {
			if _, err = s.p9OwnedEquip(pet, equip.GetEquipId()); err != nil {
				return err
			}
			if _, exists := caps[equip.GetEquipId()]; exists {
				return p5Error()
			}
			cap, err := s.p9EquipStrengthCap(pet, equip)
			if err != nil || cap > 100 {
				return p5Error()
			}
			caps[equip.GetEquipId()] = cap
		}
		if len(caps) != 4 {
			return p5Error()
		}
		costs := []*pb.BaseResource{}
		changed := map[string]bool{}
		for {
			progress := false
			for _, equip := range pet.GetPetEquipList() {
				if equip.GetStrengthLevel() >= caps[equip.GetEquipId()] {
					continue
				}
				row := s.p9TablePair("PetEquipStrength", "equipId", equip.GetEquipId(), "level", strconv.Itoa(int(equip.GetStrengthLevel()+1)))
				if row == nil {
					return p5Error()
				}
				seen := map[string]bool{}
				affordable := true
				for _, part := range strings.Split(row["cost"], ";") {
					fee, err := parsePoolCost(part)
					if err != nil || seen[fee.GetId()] || fee.GetNum() <= 0 || (fee.GetId() != "coin" && fee.GetId() != "diamond" && s.table("Item", "itemId", fee.GetId()) == nil) {
						return p5Error()
					}
					seen[fee.GetId()] = true
					if simpleOwned(p, g, fee.GetId()) < int64(fee.GetNum()) {
						affordable = false
					}
				}
				if !affordable {
					continue
				}
				paid, err := s.growthCost(p, g, row["cost"], 1)
				if err != nil {
					return err
				}
				costs = append(costs, paid...)
				equip.StrengthLevel = proto.Int32(equip.GetStrengthLevel() + 1)
				changed[equip.GetEquipId()] = true
				progress = true
			}
			if !progress {
				break
			}
		}
		if len(changed) == 0 {
			return errors.New(constUtil.P9PetStrengthAllNoProgress)
		}
		// The wire carries one aggregate charge. The native callback consumes it
		// once while updating every changed equipment instance.
		totals := map[string]int64{}
		order := []string{}
		for _, fee := range costs {
			if _, exists := totals[fee.GetId()]; !exists {
				order = append(order, fee.GetId())
			}
			totals[fee.GetId()] += int64(fee.GetNum())
			if totals[fee.GetId()] > math.MaxInt32 {
				return p5Error()
			}
		}
		for _, key := range order {
			out.Cost = append(out.Cost, &pb.BaseResource{Id: proto.String(key), Num: proto.Int32(int32(totals[key]))})
		}
		if err = p9SavePet(tx, id, g, pet); err != nil {
			return err
		}
		if err = p9Invest(tx, id, pet.GetGuid(), out.Cost, nil); err != nil {
			return err
		}
		var investment db.P9PetInvestment
		if err = tx.First(&investment, "guid=?", pet.GetGuid()).Error; err != nil {
			return err
		}
		if !strings.Contains(investment.Source, constUtil.P9PetStrengthAllSource) {
			if err = tx.Model(&investment).Update("source", investment.Source+";"+constUtil.P9PetStrengthAllSource).Error; err != nil {
				return err
			}
		}
		for _, equip := range pet.GetPetEquipList() {
			if changed[equip.GetEquipId()] {
				out.PetEquip = append(out.PetEquip, proto.Clone(equip).(*pb.PetEquip))
			}
		}
		return s.p9RecordCosts(tx, id, "p9:equip-strength-all:"+strconv.FormatInt(stamp, 10), out.Cost, LocalPlayNow(ctx))
	})
}

func (s *GameService) p9EquipAdvance(ctx context.Context, id int64, b *pb.CSPetEquipAdvance, stamp int64) ([]proto.Message, error) {
	out := &pb.SCPetEquipAdvance{}
	return s.p9NextMutation(ctx, id, "pet-equip-advance", stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		pet, err := p9OwnedPet(tx, id, b.GetPetGuid())
		if err != nil {
			return err
		}
		equip, err := s.p9OwnedEquip(pet, b.GetEquipId())
		if err != nil {
			return err
		}
		row := s.p9TablePair("PetEquipAdvance", "equipId", equip.GetEquipId(), "level", strconv.Itoa(int(equip.GetAdvanceLevel()+1)))
		if row == nil || equip.GetStrengthLevel() < int32(tableNumber(row, "needStrengthLevel")) {
			return errors.New("原装备进阶强化等级不足或已达上限")
		}
		costs, err := s.growthCost(p, g, row["cost"], 1)
		if err != nil {
			return err
		}
		equip.AdvanceLevel = proto.Int32(equip.GetAdvanceLevel() + 1)
		if err = p9SavePet(tx, id, g, pet); err != nil {
			return err
		}
		if err = p9Invest(tx, id, pet.GetGuid(), costs, nil); err != nil {
			return err
		}
		out.PetEquip = proto.Clone(equip).(*pb.PetEquip)
		out.Cost = costs
		return s.p9RecordCosts(tx, id, "p9:equip-advance:"+strconv.FormatInt(stamp, 10), costs, LocalPlayNow(ctx))
	})
}

func (s *GameService) p9EquipAwake(ctx context.Context, id int64, b *pb.CSPetEquipAwake, stamp int64) ([]proto.Message, error) {
	out := &pb.SCPetEquipAwake{}
	return s.p9NextMutation(ctx, id, "pet-equip-awake", stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		pet, err := p9OwnedPet(tx, id, b.GetPetGuid())
		if err != nil {
			return err
		}
		equip, err := s.p9OwnedEquip(pet, b.GetEquipId())
		if err != nil {
			return err
		}
		row := s.p9TablePair("PetEquipAwake", "equipId", equip.GetEquipId(), "level", strconv.Itoa(int(equip.GetAwakeLevel()+1)))
		if row == nil {
			return errors.New("原装备觉醒已达上限或配置缺失")
		}
		costs, err := s.growthCost(p, g, row["cost"], 1)
		if err != nil {
			return err
		}
		donorCount := int32(tableNumber(row, "selfCostNum"))
		if donorCount < 0 || donorCount > 100 {
			return p5Error()
		}
		donors := []*pb.Pet{}
		if donorCount > 0 {
			owned, err := P9PetList(tx, id)
			if err != nil {
				return err
			}
			candidates := []*pb.Pet{}
			for _, other := range owned {
				if other.GetId() == pet.GetId() && other.GetGuid() != pet.GetGuid() && !p9Guarded(g, other.GetGuid()) && p9BasicDonor(other) {
					candidates = append(candidates, other)
				}
			}
			sort.Slice(candidates, func(i, j int) bool { return candidates[i].GetGuid() < candidates[j].GetGuid() })
			if len(candidates) < int(donorCount) {
				return errors.New("装备觉醒同名未培养未守护精灵不足")
			}
			for _, donor := range candidates[:donorCount] {
				if err = tx.Delete(&db.P9PetInstance{}, "player_id=? AND guid=?", id, donor.GetGuid()).Error; err != nil {
					return err
				}
				p5SetKV(g, "p9.pet:"+donor.GetGuid(), "")
				donors = append(donors, donor)
				costs = append(costs, &pb.BaseResource{Id: proto.String(donor.GetId()), Guid: proto.String(donor.GetGuid()), Num: proto.Int32(1)})
			}
		}
		equip.AwakeLevel = proto.Int32(equip.GetAwakeLevel() + 1)
		if err = p9SavePet(tx, id, g, pet); err != nil {
			return err
		}
		if err = p9Invest(tx, id, pet.GetGuid(), costs, donors); err != nil {
			return err
		}
		out.PetEquip = proto.Clone(equip).(*pb.PetEquip)
		out.Cost = costs
		return s.p9RecordCosts(tx, id, "p9:equip-awake:"+strconv.FormatInt(stamp, 10), costs, LocalPlayNow(ctx))
	})
}

func (s *GameService) p9EquipInfo(ctx context.Context, id int64, b *pb.CSGetPetEquipInfo) ([]proto.Message, error) {
	out := &pb.SCGetPetEquipInfo{}
	if len(b.GetPetGuid()) > 100 {
		return nil, p5Error()
	}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		seen := map[string]bool{}
		for _, guid := range b.GetPetGuid() {
			if seen[guid] || strings.TrimSpace(guid) != guid {
				return p5Error()
			}
			seen[guid] = true
			pet, err := p9OwnedPet(tx, id, guid)
			if err != nil {
				return err
			}
			for _, equip := range pet.GetPetEquipList() {
				out.PetEquip = append(out.PetEquip, proto.Clone(equip).(*pb.PetEquip))
			}
		}
		return nil
	})
	return []proto.Message{out}, err
}

func (s *GameService) DispatchP9PetEquip(ctx context.Context, id int64, b proto.Message, stamp int64) ([]proto.Message, bool, error) {
	var out []proto.Message
	var err error
	switch request := b.(type) {
	case *pb.CSPetEquipStrengthOne:
		out, err = s.p9EquipStrength(ctx, id, request, stamp)
	case *pb.CSPetEquipAdvance:
		out, err = s.p9EquipAdvance(ctx, id, request, stamp)
	case *pb.CSPetEquipAwake:
		out, err = s.p9EquipAwake(ctx, id, request, stamp)
	case *pb.CSGetPetEquipInfo:
		out, err = s.p9EquipInfo(ctx, id, request)
	case *pb.CSPetEquipStrengthAll:
		out, err = s.p9EquipStrengthAll(ctx, id, request, stamp)
	default:
		return nil, false, nil
	}
	return out, true, err
}
