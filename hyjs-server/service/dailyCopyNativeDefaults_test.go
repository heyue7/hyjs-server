package service

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestDailyCopyNativeDefaultZeroFieldsSettleAndReplay(t *testing.T) {
	for _, win := range []bool{true, false} {
		t.Run(map[bool]string{true: "zero defender omitted", false: "zero attacker and loss omitted"}[win], func(t *testing.T) {
			s, ctx, _ := newDailyCopyTest(t)
			id := dailyCopyTestPlayer(t, ctx, "daily-native-default", 75)
			now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
			messages, err := s.dailyCopyAt(ctx, id, &pb.CSDailyCopyFightStart{ChapterId: proto.String("5")}, 100, now)
			if err != nil {
				t.Fatal(err)
			}
			start := messages[0].(*pb.SCDailyCopyFightStart)
			request := &pb.CSDailyCopyFightEnd{ChapterId: proto.String("5"), FightId: start.FightId, OptRecord: &pb.BattleOperate{FinalRoundNum: proto.Int32(1)}}
			if win {
				request.IsWin = proto.Int32(1)
				request.OptRecord.AttackerTotalLeftHp = proto.Float64(1)
			} else {
				request.OptRecord.DefenderTotalLeftHp = proto.Float64(1)
			}
			// 通过真实protobuf编解码保留C#非nullable零值不在wire中的形状。
			wire, err := proto.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			decoded := &pb.CSDailyCopyFightEnd{}
			if err := proto.Unmarshal(wire, decoded); err != nil {
				t.Fatal(err)
			}
			settled, err := s.dailyCopyAt(ctx, id, decoded, 101, now)
			if err != nil {
				t.Fatal("native default values rejected", err)
			}
			result := settled[0].(*pb.SCDailyCopyFightEnd)
			if (result.GetIsWin() == 1) != win || (len(result.GetGet().GetTreas()) == 4) != win {
				t.Fatal("wrong native victory/loss reward", result)
			}
			replay, err := s.dailyCopyAt(ctx, id, decoded, 102, now)
			if err != nil || !proto.Equal(replay[0], result) {
				t.Fatal("native replay differs", err)
			}
			var row db.Profile
			if err := db.GetIns().First(&row, id).Error; err != nil {
				t.Fatal(err)
			}
			player, _, err := decodeProfile(&row)
			if err != nil || player.GetCoin() != 0 || player.GetStamina() != 190 {
				t.Fatal("settlement/replay changed wallet or recharged entry", player, err)
			}
		})
	}
}
