package service

import (
	"errors"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
)

// RecordP5Save records only the exact costs of the existing finite growth operations.
// It shares saveProfile's transaction; hidden assistance/lock/dispatch protection applies
// to consumed instances as well as direct recycling. Training/one-key own their receipts.
func (s *GameService) RecordP5Save(tx *gorm.DB, row *db.Profile, p *pb.Player, g *model.GameState) error {
	request, ok := tx.Statement.Context.Value(constUtil.GameRequestContextKey).(proto.Message)
	if !ok || row.Game == nil {
		return nil
	}
	name := string(request.ProtoReflect().Descriptor().Name())
	switch name {
	case "CSEquipStrengthen", "CSEquipRefine", "CSEquipStar", "CSEquipForge", "CSWarriorTalent", "CSWarriorStrengthen", "CSWarriorAwake", "CSAwakeFuse", "CSTreasureStrengthen", "CSTreasureRefine", "CSUpRoleSkillRefineLevel", "CSStrengthRoleSkillRefineLevel", "CSFishUsing":
	default:
		return nil
	}
	// Only permanent dishes change the target warrior. Timed bonus food has
	// its own expiry slot and cannot become refundable warrior investment.
	if food, ok := request.(*pb.CSFishUsing); ok {
		dish := s.table("FishSkill", "FishItemID", food.GetCostItem().GetId())
		if dish == nil || !strings.HasPrefix(dish["ValueType"], "permanent") {
			return nil
		}
	}
	guid := ""
	for _, key := range []string{"guid", "heroGuid", "warriorGuid", "roleGuid", "targetId"} {
		field := request.ProtoReflect().Descriptor().Fields().ByName(protoreflect.Name(key))
		if field != nil {
			guid = request.ProtoReflect().Get(field).String()
			break
		}
	}
	if guid == "" {
		return p5Error()
	}
	beforeP, beforeG, err := decodeProfile(row)
	if err != nil {
		return err
	}
	before, err := p5OwnedInstance(beforeG, guid)
	if err != nil {
		return err
	}
	after, err := p5OwnedInstance(g, guid)
	if err != nil {
		return err
	}
	amounts := map[string]int64{}
	if beforeP.GetCoin() > p.GetCoin() {
		amounts["coin"] = beforeP.GetCoin() - p.GetCoin()
	}
	if beforeP.GetDiamond() > p.GetDiamond() {
		amounts["diamond"] = beforeP.GetDiamond() - p.GetDiamond()
	}
	for key, delta := range map[string]int64{"point": int64(beforeP.GetPoint()) - int64(p.GetPoint()), "light": int64(beforeP.GetLight()) - int64(p.GetLight()), "popularity": int64(beforeP.GetPopularity()) - int64(p.GetPopularity())} {
		if delta > 0 {
			amounts[key] = delta
		}
	}
	oldItems := map[string]int64{}
	newItems := map[string]int64{}
	for _, v := range beforeG.Items {
		oldItems[v.GetId()] += int64(v.GetNum())
	}
	for _, v := range g.Items {
		newItems[v.GetId()] += int64(v.GetNum())
	}
	for key, n := range oldItems {
		if n > newItems[key] {
			amounts[key] += n - newItems[key]
		}
	}
	if food, ok := request.(*pb.CSFishUsing); ok {
		// Freeze consumed original dish units, then compute the verified 50%
		// refund from the accumulated total at recycling, never per click.
		key := food.GetCostItem().GetId()
		var old, current int64
		for _, v := range beforeG.Cooking {
			if v.GetId() == key {
				old += int64(v.GetNum())
			}
		}
		for _, v := range g.Cooking {
			if v.GetId() == key {
				current += int64(v.GetNum())
			}
		}
		if old-current != 1 || food.GetCostItem().GetNum() != 1 {
			return p5Error()
		}
		amounts[key] += old - current
	}
	var warriorDonors []*p5Investment
	nowGUID := map[string]bool{}
	for _, w := range g.Warriors {
		nowGUID[w.GetGuid()] = true
	}
	for _, w := range beforeG.Warriors {
		if !nowGUID[w.GetGuid()] {
			if err := P5GuardInstance(beforeP, beforeG, w.GetGuid()); err != nil {
				return err
			}
			if p5KV(beforeG, "p5.invest:"+w.GetGuid()) != "" {
				donor, e := p5InvestmentState(beforeG, w.GetGuid(), w)
				if e != nil {
					return e
				}
				if e = p5CarryWarriorBreachExclusions(beforeG, w.GetGuid(), donor); e != nil {
					return e
				}
				warriorDonors = append(warriorDonors, donor)
			}
			// The original upgrade page permits ordinary level/EXP materials.
			// Carry verified paid costs into the target without refunding now.
			inherited, err := s.p5WarriorMaterialCosts(beforeP, beforeG, g, w, guid, request)
			if err != nil {
				return err
			}
			for _, cost := range inherited {
				amounts[cost.GetId()] += int64(cost.GetNum())
			}
			amounts[w.GetId()]++
		}
	}
	for _, equip := range g.Equipment {
		nowGUID[equip.GetGuid()] = true
	}
	for _, equip := range beforeG.Equipment {
		if !nowGUID[equip.GetGuid()] {
			if err := P5GuardInstance(beforeP, beforeG, equip.GetGuid()); err != nil {
				return err
			}
			if p5EquipmentProgressed(equip) || len(equip.GetGems()) > 0 {
				return p5Error()
			}
			amounts[equip.GetEquipId()]++
		}
	}
	for _, treasure := range g.Treasures {
		nowGUID[treasure.GetGuid()] = true
	}
	for _, treasure := range beforeG.Treasures {
		if !nowGUID[treasure.GetGuid()] {
			if err := P5GuardInstance(beforeP, beforeG, treasure.GetGuid()); err != nil {
				return err
			}
			// The original strength page accepts an unrefined grown donor and
			// inherits all of its EXP. Carry its exact prior costs into the
			// target ledger; no resource is refunded during consumption.
			inherited, err := s.p5TreasureMaterialCosts(beforeP, beforeG, g, treasure, guid, request)
			if err != nil {
				return err
			}
			for _, cost := range inherited {
				amounts[cost.GetId()] += int64(cost.GetNum())
			}
			amounts[treasure.GetTreasureId()]++
		}
	}
	costs := []*pb.BaseResource{}
	for key, n := range amounts {
		if n < 1 || n > 2147483647 {
			return p5Error()
		}
		costs = append(costs, &pb.BaseResource{Id: proto.String(key), Num: proto.Int32(int32(n))})
	}
	regular, err := s.p8SeparateCookingCosts(costs)
	if err != nil {
		return err
	}
	if len(regular) > 0 {
		supported, err := s.openServerRewardSupported(p5ResourceSpec(regular))
		if err != nil {
			return err
		}
		if !supported {
			return errors.New("投入资源尚无可核实返还方式")
		}
	}
	if err := RecordP5Investment(g, guid, before, after, costs, name); err != nil {
		return err
	}
	return p5InheritWarriorRefundMetadata(g, guid, after, warriorDonors)
}

func p5OwnedInstance(g *model.GameState, guid string) (proto.Message, error) {
	var found proto.Message
	assign := func(value proto.Message) error {
		if found != nil {
			return p5Error()
		}
		found = value
		return nil
	}
	for _, w := range g.Warriors {
		if w.GetGuid() == guid {
			if err := assign(w); err != nil {
				return nil, err
			}
		}
	}
	for _, v := range g.Equipment {
		if v.GetGuid() == guid {
			if err := assign(v); err != nil {
				return nil, err
			}
		}
	}
	for _, v := range g.Treasures {
		if v.GetGuid() == guid {
			if err := assign(v); err != nil {
				return nil, err
			}
		}
	}
	if found == nil {
		return nil, p5Error()
	}
	return found, nil
}
