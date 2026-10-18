package service

import (
	"errors"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// validateP5GrowthReceipt retains historical request/response audit but sends a
// complete instance snapshot only while it still matches the owned live instance.
// Reset, sale, identity or later growth changes retire the old client projection.
func (s *GameService) validateP5GrowthReceipt(g *model.GameState, response proto.Message) error {
	var (
		snapshots []proto.Message
		award     *pb.ResultObject
	)
	switch result := response.(type) {
	case *pb.SCSidePveFightEnd:
		award = result.GetGetn()
	case *pb.SCDailyCopyFightEnd:
		award = result.GetGet()
	case *pb.SCCombine:
		award = result.GetGet()
	case *pb.SCCopyFightEnd:
		award = result.GetGain()
	case *pb.SCCopySweep:
		award = result.GetGain()
	case *pb.SCItemUse:
		// Original AddResultObj recreates instance dictionaries by GUID. Only
		// replay full awarded instances while the current body still matches.
		award = result.GetGet()
	case *pb.SCWarriorSell:
		award = result.GetGet()
	case *pb.SCWarriorDecompose:
		award = result.GetGet()
	case *pb.SCEquipmentSell:
		award = result.GetGet()
	case *pb.SCTreasureSell:
		award = result.GetGet()
	case *pb.SCCopyBox:
		// CopyBox carries descriptors rather than old growth bodies. Growth
		// does not stale that descriptor; removed or changed ownership does.
		for _, resource := range result.GetCurrentResource() {
			if resource.GetGuid() == "" {
				continue
			}
			current, err := p5OwnedInstance(g, resource.GetGuid())
			if err != nil || resource.GetNum() != 1 {
				return errors.New(constUtil.P5GrowthReceiptChanged)
			}
			var id string
			switch entry := current.(type) {
			case *pb.Warrior:
				id = entry.GetId()
			case *pb.Equipment:
				id = entry.GetEquipId()
			case *pb.Treasure:
				id = entry.GetTreasureId()
			default:
				return errors.New(constUtil.P5GrowthReceiptChanged)
			}
			if id != resource.GetId() {
				return errors.New(constUtil.P5GrowthReceiptChanged)
			}
		}
	case *pb.SCLightenWorldSeed:
		award = result.GetGet()
		if result.GetNewWarrior() != nil {
			snapshots = append(snapshots, result.GetNewWarrior())
		}
	case *pb.SCWarriorStrengthen:
		snapshots = append(snapshots, result.GetGuid())
	case *pb.SCWarriorTraining:
		snapshots = append(snapshots, result.GetGuid())
	case *pb.SCWarriorTrainingReplace:
		snapshots = append(snapshots, result.GetResult())
	case *pb.SCWarriorBreached:
		snapshots = append(snapshots, result.GetGuid())
	case *pb.SCWarriorAwake:
		snapshots = append(snapshots, result.GetNewWarrior())
	case *pb.SCAwakeFuse:
		snapshots = append(snapshots, result.GetNewWarrior())
	case *pb.SCGemMount:
		snapshots = append(snapshots, result.GetEquip())
	case *pb.SCGemMountOrChange:
		snapshots = append(snapshots, result.GetEquip())
	case *pb.SCGemUnmount:
		snapshots = append(snapshots, result.GetEquip())
	case *pb.SCEquipStrengthen:
		snapshots = append(snapshots, result.GetEquip())
	case *pb.SCEquipForge:
		snapshots = append(snapshots, result.GetEquip())
	case *pb.SCEquipRefine:
		snapshots = append(snapshots, result.GetEquip())
	case *pb.SCEquipStar:
		snapshots = append(snapshots, result.GetEquip())
	case *pb.SCTreasureStrengthen:
		snapshots = append(snapshots, result.GetTreasure())
	case *pb.SCTreasureRefine:
		snapshots = append(snapshots, result.GetTreasure())
	case *pb.SCEquipStrengthenOneKey:
		snapshots = append(snapshots, result.GetEquip())
	case *pb.SCEquipStrengthenMoreOneKey:
		for _, entry := range result.GetEquips() {
			snapshots = append(snapshots, entry.GetEquip())
		}
	case *pb.SCWarriorReset:
		for _, entry := range result.GetWarriors() {
			snapshots = append(snapshots, entry)
		}
		for _, entry := range result.GetGet().GetWars() {
			snapshots = append(snapshots, entry)
		}
	case *pb.SCEquipReBuild:
		for _, entry := range result.GetGet().GetEquips() {
			snapshots = append(snapshots, entry)
		}
	case *pb.SCtreasureRebuild:
		for _, entry := range result.GetGet().GetTreas() {
			snapshots = append(snapshots, entry)
		}
	}
	// Recycling can refund fresh material instances. Those bodies are just as
	// mutable/consumable as box awards; simple currency refunds remain replayable.
	for _, entry := range award.GetWars() {
		snapshots = append(snapshots, entry)
	}
	for _, entry := range award.GetEquips() {
		snapshots = append(snapshots, entry)
	}
	for _, entry := range award.GetTreas() {
		snapshots = append(snapshots, entry)
	}
	for _, snapshot := range snapshots {
		var current proto.Message
		var err error
		switch entry := snapshot.(type) {
		case *pb.Warrior:
			current, err = p5Warrior(g, entry.GetGuid())
		case *pb.Equipment:
			current, err = s.equipmentRecord(g, entry.GetGuid())
		case *pb.Treasure:
			current, err = s.treasureRecord(g, entry.GetGuid())
		default:
			return errors.New(constUtil.P5GrowthReceiptChanged)
		}
		if err != nil || !proto.Equal(current, snapshot) {
			return errors.New(constUtil.P5GrowthReceiptChanged)
		}
	}
	return nil
}
