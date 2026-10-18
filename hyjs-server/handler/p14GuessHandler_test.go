package handler

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
)

func p14GuessOriginalBody() *pb.CSKFGuess {
	return &pb.CSKFGuess{GuessType: proto.Int32(1), BattleType: proto.Int32(0), OwnerPlayer: &pb.OwnerPlayer{ServerId: proto.Int32(1), PlayerId: proto.Int64(1)}, GuessPlayer: &pb.GuessPlayer{ServerId: proto.Int32(2), PlayerId: proto.Int64(1)}}
}

func TestP14GuessOriginalAbsentFightItemAndSamePIDAcrossRealms(t *testing.T) {
	if err := ValidateGameRequest(&model.GamePacket{Timestamp: 1, Body: p14GuessOriginalBody()}); err != nil {
		t.Fatal("original champion frame", err)
	}
	if err := ValidateGameRequest(&model.GamePacket{Timestamp: 1, Body: &pb.CSKFGuessList{}}); err != nil {
		t.Fatal(err)
	}
	for _, stage := range []int32{0, 7, 9, 11, 13, 15, 17} {
		b := p14GuessOriginalBody()
		b.GuessType = proto.Int32(2)
		b.BattleType = proto.Int32(stage)
		if err := ValidateGameRequest(&model.GamePacket{Timestamp: 1, Body: b}); err != nil {
			t.Fatal("original serialized ordinary state", stage, err)
		}
	}
	for _, change := range []func(*pb.CSKFGuess){func(b *pb.CSKFGuess) { b.OwnerPlayer = nil }, func(b *pb.CSKFGuess) { b.GuessPlayer.PlayerId = proto.Int64(0) }, func(b *pb.CSKFGuess) { b.GuessPlayer.ServerId = proto.Int32(33) }, func(b *pb.CSKFGuess) { b.BattleType = proto.Int32(8) }, func(b *pb.CSKFGuess) { b.GuessType = proto.Int32(3) }} {
		b := p14GuessOriginalBody()
		change(b)
		if ValidateGameRequest(&model.GamePacket{Timestamp: 1, Body: b}) == nil {
			t.Fatal("invalid original guess input")
		}
	}
	if ValidateGameRequest(&model.GamePacket{Timestamp: 0, Body: p14GuessOriginalBody()}) == nil {
		t.Fatal("missing HTTP timestamp")
	}
}
