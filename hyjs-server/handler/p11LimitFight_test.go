package handler

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

func TestP11LimitFightOriginalHTTPPositiveTimestamp(t *testing.T) {
	for _, stamp := range []int64{-1, 0, 1} {
		err := ValidateGameRequest(&model.GamePacket{PlayerId: 1, Timestamp: stamp, Body: &pb.CSLimitPvpFightResult{PlayerId: proto.Int64(2), ServerId: proto.Int32(1)}})
		if (stamp <= 0) != (err != nil) {
			t.Fatal("original82057 requires positive HTTP stamp", stamp, err)
		}
	}
}
