package handler

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

func TestP7SectionRewardOriginalEmptyTimestamp(t *testing.T) {
	for _, stamp := range []int64{-1, 0, 1} {
		for _, request := range []proto.Message{&pb.CSRushRewardPreview{}, &pb.CSRushGetSectionReward{}} {
			err := ValidateGameRequest(&model.GamePacket{PlayerId: 1, Timestamp: stamp, Body: request})
			_, claim := request.(*pb.CSRushGetSectionReward)
			want := stamp >= 0
			if claim {
				want = stamp > 0
			}
			if (err == nil) != want {
				t.Fatalf("%T stamp%d %v", request, stamp, err)
			}
		}
	}
}
