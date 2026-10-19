package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// tacticalFormationView 仅把实际上阵英雄的位置发给原布阵页面。
// 页面会自行将空模型填入剩余坐标，存档仍保留完整阵位映射。
func tacticalFormationView(g *model.GameState, formation *pb.TaticalFormation) *pb.TaticalFormation {
	view := proto.Clone(formation).(*pb.TaticalFormation)
	active := map[int32]bool{}
	for _, slot := range g.Formation {
		if formationHero(slot) != nil {
			active[slot.GetFormatPos()] = true
		}
	}
	view.PosInfoList = nil
	for _, position := range formation.PosInfoList {
		if active[position.GetFormatPos()] {
			view.PosInfoList = append(view.PosInfoList, proto.Clone(position).(*pb.FormationPosInfo))
		}
	}
	return view
}
