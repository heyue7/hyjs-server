package handler

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

func TestOriginalP8SocketZeroTimestampValidation(t *testing.T) {
	requests := []proto.Message{
		&pb.CSFieldResourceStartGather{PosId: proto.String("field_resource_1")},
		&pb.CSFieldGetResource{},
		&pb.CSHpRecover{CostItem: &pb.BaseResource{Id: proto.String("item_129"), Num: proto.Int32(1)}},
		&pb.CSResurrection{},
		&pb.CSTransfer2Safe{},
		&pb.CSWorldBossFight{BossId: proto.String("original-cycle")},
	}
	for _, request := range requests {
		for _, stamp := range []int64{0, 100} {
			if err := ValidateGameRequest(&model.GamePacket{PlayerId: 3, Timestamp: stamp, Body: request}); err != nil {
				t.Fatal("original zero timestamp socket rejected", request.ProtoReflect().Descriptor().Name(), stamp, err)
			}
		}
		if err := ValidateGameRequest(&model.GamePacket{PlayerId: 3, Timestamp: -1, Body: request}); err == nil {
			t.Fatal("negative timestamp accepted", request)
		}
	}
	for _, request := range []proto.Message{
		&pb.CSFieldResourceStartGather{},
		&pb.CSFieldResourceStartGather{PosId: proto.String(" bad ")},
		&pb.CSHpRecover{},
		&pb.CSHpRecover{CostItem: &pb.BaseResource{Id: proto.String("item_129"), Num: proto.Int32(2)}},
		&pb.CSHpRecover{CostItem: &pb.BaseResource{Id: proto.String("item_129"), Num: proto.Int32(1), Guid: proto.String("forged")}},
		&pb.CSWorldBossFight{},
	} {
		if ValidateGameRequest(&model.GamePacket{PlayerId: 3, Body: request}) == nil {
			t.Fatal("invalid original socket body accepted", request)
		}
	}
}
