package service

import (
	"context"
	"fmt"
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
		if len(b.GetGuid()) > constUtil.RecycleDecomposeMaxBatch {
			return fmt.Errorf("装备分解每批最多选择%d件：%w", constUtil.RecycleDecomposeMaxBatch, p5Error())
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
					// Keep untracked investment protected and identify the selected item that blocked the batch.
					return fmt.Errorf("装备%s（%s）培养返还校验失败，请保留装备并核对投入记录：%w", equip.GetEquipId(), guid, err)
				}
				redMergeGain(gain, refund)
				baseline, err := s.equipmentRecord(g, guid)
				if err != nil || p5EquipmentProgressed(baseline) {
					return fmt.Errorf("装备%s（%s）返还后仍有培养进度，已保留整批装备：%w", equip.GetEquipId(), guid, p5Error())
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
