package handler

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP14FinalDrawOriginalHTTPGroupsAndReplayBoundaries(t *testing.T) {
	for _, group := range []string{"", "A", "B", "C", "D", " A "} {
		for _, flag := range []int32{0, 1} {
			body := &pb.CSKFMatchFrightList{GroupId: proto.String(group), ReplayFlag: proto.Int32(flag)}
			if err := ValidateGameRequest(&model.GamePacket{Timestamp: 1, Body: body}); err != nil {
				t.Fatal(group, flag, err)
			}
			if group == " A " && body.GetGroupId() != "A" {
				t.Fatal("known original group was not trimmed")
			}
		}
	}
	if err := ValidateGameRequest(&model.GamePacket{Timestamp: 1, Body: &pb.CSKFMatchFrightList{}}); err != nil {
		t.Fatal("omitted original FinalFour group/flag", err)
	}
	for _, group := range []string{"a", "E", "1", "A\x00", "A/B", string([]byte{0xff})} {
		if ValidateGameRequest(&model.GamePacket{Timestamp: 1, Body: &pb.CSKFMatchFrightList{GroupId: proto.String(group)}}) == nil {
			t.Fatal("invalid group accepted", group)
		}
	}
	for _, test := range []struct {
		stamp int64
		flag  int32
	}{{0, 0}, {-1, 0}, {1, -1}, {1, 2}} {
		if ValidateGameRequest(&model.GamePacket{Timestamp: test.stamp, Body: &pb.CSKFMatchFrightList{GroupId: proto.String("A"), ReplayFlag: proto.Int32(test.flag)}}) == nil {
			t.Fatal("invalid original HTTP/enum accepted", test)
		}
	}
}
