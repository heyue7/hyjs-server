package handler

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP11PlunderOriginalQueryAndMutationInputs(t *testing.T) {
	for _, stamp := range []int64{-1, 0, 1} {
		request := &pb.CSFightTreasureStartResult{FightPlayerId: proto.Int64(2), TargetFlagId: proto.String("frag_trea_001_1")}
		err := ValidateGameRequest(&model.GamePacket{PlayerId: 1, Timestamp: stamp, Body: request})
		if (stamp <= 0) != (err != nil) {
			t.Fatal("original56011 requires a positive HTTP timestamp", stamp, err)
		}
	}
	if err := ValidateGameRequest(&model.GamePacket{PlayerId: 1, Timestamp: 0, Body: &pb.CSTreasureBattleList{FragTreasId: proto.String("frag_trea_001_1")}}); err != nil {
		t.Fatal("read-only original21055 allows zero stamp", err)
	}
	for _, target := range []string{"", "item_001", "frag_hero_001"} {
		for _, body := range []proto.Message{&pb.CSTreasureBattleList{FragTreasId: proto.String(target)}, &pb.CSFightTreasureStartResult{FightPlayerId: proto.Int64(2), TargetFlagId: proto.String(target)}} {
			if err := ValidateGameRequest(&model.GamePacket{PlayerId: 1, Timestamp: 1, Body: body}); err == nil {
				t.Fatal("invalid original fragment request", body)
			}
		}
	}
}
