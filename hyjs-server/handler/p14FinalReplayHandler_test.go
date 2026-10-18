package handler

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
)

func TestP14FinalReplayOriginalClickJointRegistrationAndTypedPlayback(t *testing.T) {
	for _, stage := range []int32{8, 10, 12, 14, 16, 18} {
		body := &pb.CSKFReplayInfo{Type: proto.Int32(3), SubType: proto.Int32(stage), RegId: proto.String("61_2_1")}
		if err := ValidateGameRequest(&model.GamePacket{Timestamp: 1, Body: body}); err != nil {
			t.Fatal("original click", err)
		}
	}
	for _, reg := range []string{"", "61_1", "61_1_1_2", "0_1_1", "61_0_1", "61_2_0", "+61_2_1", "061_2_1", "61_2_-1", "2147483648_2_1"} {
		body := &pb.CSKFReplayInfo{Type: proto.Int32(3), SubType: proto.Int32(8), RegId: proto.String(reg)}
		if ValidateGameRequest(&model.GamePacket{Timestamp: 1, Body: body}) == nil {
			t.Fatal("invalid joint", reg)
		}
	}
	for _, stage := range []int32{0, 7, 9, 11, 19} {
		if ValidateGameRequest(&model.GamePacket{Timestamp: 1, Body: &pb.CSKFReplayInfo{Type: proto.Int32(3), SubType: proto.Int32(stage), RegId: proto.String("61_2_1")}}) == nil {
			t.Fatal("invalid original round", stage)
		}
	}
	valid := &pb.CSKFReplay{Type: proto.Int32(3), WarId: proto.String(strings.Repeat("a", 64))}
	if err := ValidateGameRequest(&model.GamePacket{Timestamp: 1, Body: valid}); err != nil {
		t.Fatal(err)
	}
	for _, war := range []string{"", strings.Repeat("z", 64), strings.Repeat("a", 62), "a\x00"} {
		if ValidateGameRequest(&model.GamePacket{Timestamp: 1, Body: &pb.CSKFReplay{Type: proto.Int32(3), WarId: proto.String(war)}}) == nil {
			t.Fatal("invalid input id")
		}
	}
	if ValidateGameRequest(&model.GamePacket{Timestamp: 0, Body: valid}) == nil {
		t.Fatal("missing HTTP stamp")
	}
}
