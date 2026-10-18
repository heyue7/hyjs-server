package service

import (
	"context"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"strconv"
)

func (s *GameService) p5AutoStrengthen(p *pb.Player, g *model.GameState, equip *pb.Equipment) (int64, int32, error) {
	if err := equipmentBasicState(equip); err != nil {
		return 0, 0, err
	}
	row := s.table("Equip", "id", equip.GetEquipId())
	if row == nil {
		return 0, 0, p5Error()
	}
	factor := tableNumber(s.table("GameConfigure", "StaticName", "playerLevel_Equip_Value"), "NameValue")
	maximum := tableNumber(s.table("GameConfigure", "StaticName", "maxEquipLevel"), "NameValue")
	playerMax := tableNumber(s.table("GameConfigure", "StaticName", "maxPlayerLevel"), "NameValue")
	if factor <= 0 || maximum < 1 || maximum > 2000 || p.GetLevel() < 1 || playerMax < 1 {
		return 0, 0, p5Error()
	}
	limit := min(maximum, math.Ceil(float64(p.GetLevel())*factor))
	if float64(p.GetLevel()) >= playerMax {
		limit = maximum
	}
	before := proto.Clone(equip).(*pb.Equipment)
	var spent int64
	var times int32
	for float64(equip.GetLevel()) < limit {
		var costRow map[string]string
		for _, candidate := range s.Tables["EquipExp"] {
			if candidate["quality"] == row["quality"] && candidate["level"] == strconv.FormatInt(int64(equip.GetLevel()), 10) {
				costRow = candidate
				break
			}
		}
		cost, err := strconv.ParseInt(costRow["coin"], 10, 64)
		if err != nil || cost <= 0 || cost > math.MaxInt32-spent {
			return 0, 0, p5Error()
		}
		if p.GetCoin() < cost {
			break
		}
		p.Coin = proto.Int64(p.GetCoin() - cost)
		spent += cost
		times++
		equip.Level = proto.Int32(equip.GetLevel() + 1)
	}
	if times > 0 {
		if err := RecordP5Investment(g, equip.GetGuid(), before, equip, []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(int32(spent))}}); err != nil {
			return 0, 0, err
		}
	}
	return spent, times, nil
}
func (s *GameService) p5StrengthenOne(ctx context.Context, id int64, b *pb.CSEquipStrengthenOneKey, stamp int64) ([]proto.Message, error) {
	out := &pb.SCEquipStrengthenOneKey{}
	return s.p5Mutation(ctx, id, "equipment-one-key", stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		equip, err := s.equipmentRecord(g, b.GetGuid())
		if err != nil {
			return err
		}
		spent, times, err := s.p5AutoStrengthen(p, g, equip)
		if err != nil {
			return err
		}
		if times == 0 {
			return p5Error()
		}
		out.Equip = proto.Clone(equip).(*pb.Equipment)
		out.Cost = []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(int32(spent))}}
		out.CostTimes = proto.Int32(times)
		out.CriTimes = proto.Int32(0)
		return s.RecordP1Event(tx, id, fmt.Sprintf("p5:strength-one:%d", stamp), constUtil.P1EventEquipStrengthen, int64(times), LocalPlayNow(ctx))
	})
}
func (s *GameService) p5StrengthenMore(ctx context.Context, id int64, b *pb.CSEquipStrengthenMoreOneKey, stamp int64) ([]proto.Message, error) {
	out := &pb.SCEquipStrengthenMoreOneKey{}
	return s.p5Mutation(ctx, id, "equipment-more-key", stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		if b.GetFormatPos() < 0 || b.GetFormatPos() > 5 {
			return p5Error()
		}
		seen := map[string]bool{}
		var spent int64
		var total int32
		for _, slot := range g.Formation {
			if b.GetFormatPos() > 0 && slot.GetFormatPos() != b.GetFormatPos() {
				continue
			}
			for _, thing := range slot.GetThings() {
				if thing.GetPos() == 0 || seen[thing.GetGuid()] {
					continue
				}
				equip, err := s.equipmentRecord(g, thing.GetGuid())
				if err != nil {
					continue
				}
				seen[equip.GetGuid()] = true
				cost, times, err := s.p5AutoStrengthen(p, g, equip)
				if err != nil {
					return err
				}
				if cost > math.MaxInt32-spent {
					return p5Error()
				}
				spent += cost
				total += times
				out.Equips = append(out.Equips, &pb.EquipStrengthenInfo{Equip: proto.Clone(equip).(*pb.Equipment), CostTimes: proto.Int32(times), CriTimes: proto.Int32(0)})
			}
		}
		if total == 0 {
			return p5Error()
		}
		out.Cost = []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(int32(spent))}}
		return s.RecordP1Event(tx, id, fmt.Sprintf("p5:strength-more:%d", stamp), constUtil.P1EventEquipStrengthen, int64(total), LocalPlayNow(ctx))
	})
}
