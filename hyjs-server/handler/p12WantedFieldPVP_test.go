package handler

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Original TCP has no timestamp. The same body validation must preserve it
// while rejecting forged actor payloads and unknown business enums before SQL.
func TestP12WantedFieldPVPOriginalRequestBoundary(t *testing.T) {
	valid := []proto.Message{
		&pb.CSGetRebels{},
		&pb.CSGetRebelRewardInfo{},
		&pb.CSShareRebel{RebelId: proto.String("owned-instance")},
		&pb.CSRebelFightStart{RebelId: proto.String("owned-instance"), Type: proto.Int32(1)},
		&pb.CSRebelFightStart{RebelId: proto.String("owned-instance"), Type: proto.Int32(2)},
		&pb.CSRebelFightEnd{FightId: proto.String("owned-fight"), Result: proto.Int32(0), OptRecord: &pb.BattleOperate{}},
		&pb.CSRebelFightEnd{FightId: proto.String("owned-fight"), Result: proto.Int32(1), OptRecord: &pb.BattleOperate{}},
		&pb.CSGetRebelReward{RebelRewardId: proto.Int32(1)},
		&pb.CSGetRebelReward{RebelRewardId: proto.Int32(2)},
		&pb.CSSidePvpFightResult{PlayerId: proto.Int64(8)},
		&pb.CSSidePvpFightResult{PlayerId: proto.Int64(8), ServerId: proto.Int32(60006), FightType: proto.Int32(1)},
		&pb.CSPlayerRedeem{Type: proto.Int32(0)},
		&pb.CSPlayerRedeem{Type: proto.Int32(1)},
	}
	for _, body := range valid {
		for _, stamp := range []int64{0, 1792148400000} {
			if err := ValidateGameRequest(&model.GamePacket{PlayerId: 7, Timestamp: stamp, Body: body}); err != nil {
				t.Fatal(body.ProtoReflect().Descriptor().Name(), "original transport refused", stamp, err)
			}
		}
		if ValidateGameRequest(&model.GamePacket{PlayerId: 7, Timestamp: -1, Body: body}) == nil {
			t.Fatal(body.ProtoReflect().Descriptor().Name(), "negative timestamp accepted")
		}
	}
	for _, body := range []proto.Message{
		&pb.CSShareRebel{},
		&pb.CSShareRebel{RebelId: proto.String(" padded ")},
		&pb.CSRebelFightStart{RebelId: proto.String("owned-instance"), Type: proto.Int32(3)},
		&pb.CSRebelFightStart{Type: proto.Int32(1)},
		&pb.CSRebelFightEnd{FightId: proto.String("owned-fight"), Result: proto.Int32(1)},
		&pb.CSRebelFightEnd{FightId: proto.String("owned-fight"), Result: proto.Int32(2), OptRecord: &pb.BattleOperate{}},
		&pb.CSRebelFightEnd{FightId: proto.String("owned-fight"), OptRecord: &pb.BattleOperate{}, BattlePlayer: &pb.BattlePlayer{}},
		&pb.CSGetRebelReward{RebelRewardId: proto.Int32(3)},
		&pb.CSSidePvpFightResult{PlayerId: proto.Int64(7)},
		&pb.CSSidePvpFightResult{PlayerId: proto.Int64(8), FightType: proto.Int32(2)},
		&pb.CSSidePvpFightResult{PlayerId: proto.Int64(8), ServerId: proto.Int32(-1)},
		&pb.CSPlayerRedeem{Type: proto.Int32(2)},
	} {
		if ValidateGameRequest(&model.GamePacket{PlayerId: 7, Body: body}) == nil {
			t.Fatal(body.ProtoReflect().Descriptor().Name(), "invalid original request accepted")
		}
	}
}
