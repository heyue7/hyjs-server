package handler

import (
	"math"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestNativeDailyCopyAllowsZeroOmissionAndRetainsBounds(t *testing.T) {
	base := &pb.CSDailyCopyFightEnd{ChapterId: proto.String("5"), FightId: proto.String("frozen-fight"), IsWin: proto.Int32(1), OptRecord: &pb.BattleOperate{FinalRoundNum: proto.Int32(3), AttackerTotalLeftHp: proto.Float64(1)}}
	if err := ValidateGameRequest(&model.GamePacket{Body: base, Timestamp: 1}); err != nil {
		t.Fatal("omitted dead defender HP rejected", err)
	}
	loss := proto.Clone(base).(*pb.CSDailyCopyFightEnd)
	loss.IsWin = nil
	loss.OptRecord.AttackerTotalLeftHp = nil
	loss.OptRecord.DefenderTotalLeftHp = proto.Float64(1)
	if err := ValidateGameRequest(&model.GamePacket{Body: loss, Timestamp: 1}); err != nil {
		t.Fatal("omitted loss and dead attacker rejected", err)
	}
	for _, hp := range []float64{-1, math.NaN(), math.Inf(1)} {
		invalid := proto.Clone(base).(*pb.CSDailyCopyFightEnd)
		invalid.OptRecord.DefenderTotalLeftHp = proto.Float64(hp)
		if err := ValidateGameRequest(&model.GamePacket{Body: invalid, Timestamp: 1}); err == nil {
			t.Fatal("invalid remaining HP accepted", hp)
		}
	}
	base.OptRecord.FinalRoundNum = nil
	if err := ValidateGameRequest(&model.GamePacket{Body: base, Timestamp: 1}); err == nil {
		t.Fatal("missing positive final round accepted")
	}
}
