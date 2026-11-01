package handler

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestRechargeDirectoryUsesExistingActivityProtocol(t *testing.T) {
	if len(constUtil.RechargeTotalDirectoryID) > 64 {
		t.Fatal("directory ID exceeds the original protocol limit")
	}
	for _, request := range []proto.Message{
		&pb.CSActivityInfo{ActivityId: proto.String(constUtil.RechargeTotalDirectoryID)},
		&pb.CSActivityReward{ActivityId: proto.String(constUtil.RechargeTotalDirectoryID), Index: proto.Int32(1), GetResources: []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(1)}}},
	} {
		if err := ValidateGameRequest(&model.GamePacket{Timestamp: 1, Body: request}); err != nil {
			t.Fatal("original activity protocol rejected directory", err)
		}
	}
	if err := ValidateGameRequest(&model.GamePacket{Timestamp: 1, Body: &pb.CSActivityInfo{ActivityId: proto.String(strings.Repeat("a", 65))}}); err == nil {
		t.Fatal("existing activity ID limit changed")
	}
	if err := ValidateGameRequest(&model.GamePacket{Timestamp: 1, Body: &pb.CSActivityReward{ActivityId: proto.String(constUtil.RechargeTotalDirectoryID), Index: proto.Int32(1)}}); err == nil {
		t.Fatal("directory accepted a reward claim without resources")
	}
}
