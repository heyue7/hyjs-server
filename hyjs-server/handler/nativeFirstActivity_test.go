package handler

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

func TestNativeFirstFixedRewardRequest(t *testing.T) {
	native := &pb.CSActivityReward{ActivityId: proto.String(constUtil.P3NativeFirstActivityID), Index: proto.Int32(1)}
	if err := ValidateGameRequest(&model.GamePacket{Timestamp: 1, Body: native}); err != nil {
		t.Fatal("original first fixed reward request rejected", err)
	}
	for _, id := range []string{"firstRecharge_other_local_p3", "rechargeSingle_local_p3", "totalLogin_local_20261011"} {
		bad := &pb.CSActivityReward{ActivityId: proto.String(id), Index: proto.Int32(1)}
		if err := ValidateGameRequest(&model.GamePacket{Timestamp: 1, Body: bad}); err == nil {
			t.Fatal("empty selectable reward accepted", id)
		}
	}
	native.Index = proto.Int32(0)
	if err := ValidateGameRequest(&model.GamePacket{Timestamp: 1, Body: native}); err == nil {
		t.Fatal("invalid first tier accepted")
	}
}
