package handler

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

func TestP11PlunderSweepOriginalHTTPCountAndTimestamp(t *testing.T) {
	for _, stamp := range []int64{-1, 0, 1} {
		request := &pb.CSSweepTreasureFight{FragId: proto.String("frag_trea_001_1"), SweepNumber: proto.Int32(5), AutoAddEnergy: proto.Int32(0)}
		err := ValidateGameRequest(&model.GamePacket{PlayerId: 1, Timestamp: stamp, Body: request})
		if (stamp <= 0) != (err != nil) {
			t.Fatal("original56009 mutation requires positive HTTP stamp", stamp, err)
		}
	}
	for _, count := range []int32{0, 1, 2, 5, 6} {
		request := &pb.CSSweepTreasureFight{FragId: proto.String("frag_trea_001_1"), SweepNumber: proto.Int32(count), AutoAddEnergy: proto.Int32(0)}
		err := ValidateGameRequest(&model.GamePacket{PlayerId: 1, Timestamp: 1, Body: request})
		if (count == 1 || count == 5) != (err == nil) {
			t.Fatal("originalUI sends automatic1 or five5", count, err)
		}
	}
}
