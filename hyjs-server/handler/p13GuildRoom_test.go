package handler

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"testing"
)

func TestP13GuildRoomIngressOriginalSceneAndFinitePoint(t *testing.T) {
	for _, request := range []proto.Message{&pb.CSTransferScene{Target: proto.String("town_union")}, &pb.CSTransferScene{Target: proto.String("town_union"), Point: &pb.PointNew{X: proto.Float32(0), Y: proto.Float32(0), Z: proto.Float32(0)}}, &pb.CSRefreshUnionBuilding{}} {
		if err := ValidateGameRequest(&model.GamePacket{PlayerId: 1, Timestamp: 100, Body: request}); err != nil {
			t.Fatal("original room ingress", err)
		}
	}
	for _, request := range []proto.Message{&pb.CSTransferScene{Target: proto.String("p13-union:foreign")}, &pb.CSTransferScene{Target: proto.String("town_union"), Point: &pb.PointNew{X: proto.Float32(float32(math.NaN()))}}, &pb.CSTransferScene{Target: proto.String("town_union"), Point: &pb.PointNew{Z: proto.Float32(float32(math.Inf(1)))}}} {
		if err := ValidateGameRequest(&model.GamePacket{PlayerId: 1, Timestamp: 100, Body: request}); err == nil {
			t.Fatal("foreign instance/nonfinite point accepted", request)
		}
	}
}
