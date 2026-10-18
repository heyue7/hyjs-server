package handler

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

func TestP11GrabRandomOriginalTimestampGuardAndZeroReads(t *testing.T) {
	for _, stamp := range []int64{-1, 0, 1} {
		err := ValidateGameRequest(&model.GamePacket{PlayerId: 1, Timestamp: stamp, Body: &pb.CSGetRandReward{}})
		if (stamp <= 0) != (err != nil) {
			t.Fatal("original82053 positive mutation stamp", stamp, err)
		}
	}
	for _, request := range []proto.Message{&pb.CSGetGrabData{}, &pb.CSRedPoint{}} {
		if err := ValidateGameRequest(&model.GamePacket{PlayerId: 1, Timestamp: 0, Body: request}); err != nil {
			t.Fatal("original read rejects zero timestamp", request, err)
		}
	}
}
