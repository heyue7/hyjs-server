package handler

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

func TestP13BattleAuditTargetRequest(t *testing.T) {
	for _, stamp := range []int64{0, 100} {
		if err := ValidateGameRequest(&model.GamePacket{PlayerId: 4, Timestamp: stamp, Body: &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(5)}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []struct{ target, stamp int64 }{{0, 100}, {-1, 100}, {4, 100}, {5, -1}} {
		if err := ValidateGameRequest(&model.GamePacket{PlayerId: 4, Timestamp: row.stamp, Body: &pb.CSUFPlayerAskFight{TargetGuid: proto.Int64(row.target)}}); err == nil {
			t.Fatalf("accepted malformed target/stamp: %+v", row)
		}
	}
}
