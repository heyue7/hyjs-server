package handler

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestOriginalSceneTransferAcceptsTCPWithoutHTTPTimestamp(t *testing.T) {
	for _, stamp := range []int64{0, 100} {
		for _, line := range []int32{1, 32} {
			if err := ValidateGameRequest(&model.GamePacket{PlayerId: 3, Timestamp: stamp, Body: &pb.CSTransferLine{Line: proto.Int32(line)}}); err != nil {
				t.Fatal("valid original socket or HTTP transfer rejected", stamp, line, err)
			}
		}
	}
	for _, invalid := range []struct {
		stamp int64
		line  int32
	}{{-1, 1}, {0, 0}, {0, 33}} {
		if ValidateGameRequest(&model.GamePacket{PlayerId: 3, Timestamp: invalid.stamp, Body: &pb.CSTransferLine{Line: proto.Int32(invalid.line)}}) == nil {
			t.Fatal("invalid original transfer accepted", invalid)
		}
	}
}
