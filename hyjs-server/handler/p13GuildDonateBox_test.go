package handler

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP13GuildDonateBoxOriginalIngressIDAndStamp(t *testing.T) {
	valid := strings.Repeat("0123456789abcdef", 4)
	if err := ValidateGameRequest(&model.GamePacket{PlayerId: 1, Timestamp: 100, Body: &pb.CSUnionOpenDonateBox{BoxId: proto.String(valid)}}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("A", 64), strings.Repeat("g", 64), " " + valid[1:], valid[:63] + "\n"} {
		if err := ValidateGameRequest(&model.GamePacket{PlayerId: 1, Timestamp: 100, Body: &pb.CSUnionOpenDonateBox{BoxId: proto.String(id)}}); err == nil {
			t.Fatalf("invalid box ID accepted %q", id)
		}
	}
	for _, stamp := range []int64{0, -1} {
		if err := ValidateGameRequest(&model.GamePacket{PlayerId: 1, Timestamp: stamp, Body: &pb.CSUnionOpenDonateBox{BoxId: proto.String(valid)}}); err == nil {
			t.Fatal("nonpositive HTTP stamp accepted")
		}
	}
}
