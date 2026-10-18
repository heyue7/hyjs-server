package handler

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestBigClientRewardOnlyOriginalType(t *testing.T) {
	for _, kind := range []int32{-1, 0, 1, 2} {
		err := ValidateGameRequest(&model.GamePacket{Body: &pb.CSBigClientReward{Type: proto.Int32(kind)}})
		if (err == nil) != (kind == 1) {
			t.Fatalf("type %d accepted=%v", kind, err == nil)
		}
	}
	if err := ValidateGameRequest(&model.GamePacket{Body: &pb.CSBigClientReward{}}); err == nil {
		t.Fatal("missing original type accepted")
	}
}
