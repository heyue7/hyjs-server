package handler

import (
	"encoding/binary"
	"math"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The original protobuf-net client omits scalar zero values, including a loss and dead HP.
func TestFieldEndOriginalZeroDefaults31(t *testing.T) {
	end := &pb.CSSidePveFightEnd{FightId: proto.String("native-zero-loss"),
		BattlePlayer: &pb.BattlePlayer{PlayerId: proto.Int64(4), Inst: &pb.BattlePlayerInst{}, Formation: []*pb.BattleHero{{Id: proto.String("hero_001"), Guid: proto.String("owned-hero"), Pose: proto.Int32(1), Inst: &pb.BattleHeroInst{}}}},
		OptRecord:    &pb.BattleOperate{FinalRoundNum: proto.Int32(9), DefenderTotalLeftHp: proto.Float64(10865.9296498705)}}
	raw, err := proto.Marshal(end)
	if err != nil {
		t.Fatal(err)
	}
	frame := make([]byte, 28)
	binary.BigEndian.PutUint32(frame, uint32(24+len(raw)))
	binary.BigEndian.PutUint32(frame[4:], 1)
	binary.BigEndian.PutUint32(frame[8:], 30049)
	binary.BigEndian.PutUint32(frame[12:], 1)
	binary.BigEndian.PutUint64(frame[16:], 4)
	binary.BigEndian.PutUint32(frame[24:], uint32(len(raw)))
	packet, err := model.ParseGamePacket(append(frame, raw...), true)
	if err != nil {
		t.Fatal(err)
	}
	got := packet.Body.(*pb.CSSidePveFightEnd)
	if got.Result != nil || got.OptRecord.AttackerTotalLeftHp != nil || got.BattlePlayer.Formation[0].Inst.CurHp != nil {
		t.Fatal("zero omission fixture did not survive the native wire")
	}
	if err = ValidateGameRequest(packet); err != nil {
		t.Fatal("legitimate native loss closes TCP", err)
	}
	for _, bad := range []float64{-1, math.NaN(), math.Inf(1)} {
		got.BattlePlayer.Formation[0].Inst.CurHp = proto.Float64(bad)
		if ValidateGameRequest(packet) == nil {
			t.Fatal("invalid HP accepted", bad)
		}
	}
	got.BattlePlayer.Formation[0].Inst.CurHp = nil
	got.Result = proto.Int32(2)
	if ValidateGameRequest(packet) == nil {
		t.Fatal("invalid result accepted")
	}
	got.Result = nil
	got.BattlePlayer.Inst = nil
	if ValidateGameRequest(packet) == nil {
		t.Fatal("missing message container accepted")
	}
}
