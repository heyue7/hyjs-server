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

// Original RecycleResetPanel sends CStreasureRebuild and uses EquipmentRebuildCost.
func (s *GameService) p5TreasureReset(ctx context.Context, id int64, b *pb.CStreasureRebuild, stamp int64) ([]proto.Message, error) {
	out := &pb.SCtreasureRebuild{}
	return s.p5Mutation(ctx, id, "treasure-reset", stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		if err := P5GuardInstance(p, g, b.GetTargetId()); err != nil {
			return err
		}
		treasure, err := s.treasureRecord(g, b.GetTargetId())
		if err != nil {
			return err
		}
		costs, err := p3Resources(s.table("GameConfigure", "StaticName", "EquipmentRebuildCost")["NameValue"])
		if err != nil || len(costs) != 1 || costs[0].GetId() != "diamond" {
			return p5Error()
		}
		for _, cost := range costs {
			if err = deductPoolCost(p, g, cost); err != nil {
				return err
			}
		}
		gain, err := s.p5Refund(p, g, treasure.GetGuid(), treasure)
		if err != nil {
			return err
		}
		gain.Treas = append(gain.Treas, proto.Clone(treasure).(*pb.Treasure))
		out.Get = gain
		out.Cost = costs
		return s.RecordP1Event(tx, id, fmt.Sprintf("p5:treasure-reset:%d", stamp), constUtil.P1EventDiamondSpent, int64(costs[0].GetNum()), LocalPlayNow(ctx))
	})
}
