package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// P8GemTransferSnapshot freezes only the target native equipment before an
// authorized gem movement; ownership stays in the existing equipment record.
func (s *GameService) P8GemTransferSnapshot(g *model.GameState, request proto.Message) (*pb.Equipment, error) {
	var guid string
	switch b := request.(type) {
	case *pb.CSGemMount:
		guid = b.GetGuid()
	case *pb.CSGemMountOrChange:
		guid = b.GetGuid()
	case *pb.CSGemUnmount:
		guid = b.GetGuid()
	default:
		return nil, nil
	}
	equip, err := s.equipmentRecord(g, guid)
	if err != nil {
		return nil, err
	}
	return proto.Clone(equip).(*pb.Equipment), nil
}

// P8RecordGemTransfer advances an existing exact investment fingerprint after
// a legitimate gem movement. Gems remain physical slot inventory, never costs
// refunded twice. Missing legacy investment is never reconstructed.
func (s *GameService) P8RecordGemTransfer(g *model.GameState, before *pb.Equipment) error {
	if before == nil || p5KV(g, "p5.invest:"+before.GetGuid()) == "" {
		return nil
	}
	after, err := s.equipmentRecord(g, before.GetGuid())
	if err != nil {
		return err
	}
	return RecordP5Investment(g, before.GetGuid(), before, after, nil)
}

// p8RecycleEquipmentGems follows the original reset/decompose instruction to
// return every currently mounted gem. It validates native four-hole colours
// and inventory limits before clearing slots, so failed batches lose nothing.
func (s *GameService) p8RecycleEquipmentGems(p *pb.Player, g *model.GameState, equip *pb.Equipment) (*pb.ResultObject, error) {
	if err := s.equipmentGemProperties(equip, map[string]float64{}); err != nil {
		return nil, err
	}
	gems := map[string]int64{}
	for _, key := range equip.GetGems() {
		if key != "" {
			gems[key]++
		}
	}
	if len(gems) > 0 {
		if err := s.gemInventoryDelta(p, g, gems); err != nil {
			return nil, err
		}
	}
	equip.Gems = nil
	return gemRewards(gems), nil
}
