package service

import (
	"context"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func (s *GameService) p5EquipmentDecompose(ctx context.Context, id int64, b *pb.CSEquipDecompose, stamp int64) ([]proto.Message, error) {
	out := &pb.SCEquipDecompose{}
	return s.p5Mutation(ctx, id, "equipment-decompose", stamp, b, out, func(_ *gorm.DB, p *pb.Player, g *model.GameState) error {
		if err := p5ListValid(b.GetGuid()); err != nil {
			return err
		}
		if len(b.GetGuid()) > 4 {
			return p5Error()
		}
		gain := &pb.ResultObject{}
		for _, guid := range b.GetGuid() {
			if err := P5GuardInstance(p, g, guid); err != nil {
				return err
			}
			equip, err := s.equipmentRecord(g, guid)
			if err != nil {
				return err
			}
			if p5EquipmentProgressed(equip) {
				refund, err := s.p5Refund(p, g, guid, equip)
				if err != nil {
					return err
				}
				redMergeGain(gain, refund)
				baseline, err := s.equipmentRecord(g, guid)
				if err != nil || p5EquipmentProgressed(baseline) {
					return p5Error()
				}
			}
			equip, err = s.equipmentRecord(g, guid)
			if err != nil {
				return err
			}
			gems, err := s.p8RecycleEquipmentGems(p, g, equip)
			if err != nil {
				return err
			}
			redMergeGain(gain, gems)
		}
		base, costs, err := s.equipmentDecomposeApply(p, g, b, constUtil.EquipmentRefundBaseOnly)
		if err != nil {
			return err
		}
		redMergeGain(gain, base)
		out.Get = gain
		out.Cost = costs
		return nil
	})
}
