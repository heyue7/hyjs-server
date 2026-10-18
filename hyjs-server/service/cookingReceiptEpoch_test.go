package service

import (
	"context"
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestCookingReceiptEpoch exercises actual craft/use/reset/sale and reconnect.
// Ingredient balances are finite test prerequisites; natural acquisition belongs
// to the separate source-chain scenarios, not this retired projection test.
func TestCookingReceiptEpoch(t *testing.T) {
	for _, action := range []string{"reset", "sale", "later-growth"} {
		t.Run(action, func(t *testing.T) {
			s, id, path := gemFixture(t, false)
			base := context.Background()
			if err := s.LoadP5Tables("../../analysis/tables"); err != nil {
				t.Fatal(err)
			}
			for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP5, s.InitCooking} {
				if err := init(base); err != nil {
					t.Fatal(err)
				}
			}
			s.P5Ready = true
			var guid string
			if err := updateProfile(base, id, func(p *pb.Player, g *model.GameState) error {
				p.Diamond = proto.Int64(200)
				gain, err := s.GrantFixedReward(p, g, "hero_003,1")
				if err != nil {
					return err
				}
				guid = gain.GetWars()[0].GetGuid()
				for _, key := range []string{"FishItem_007", "FishItem_013", "FishItem_019"} {
					g.Cooking = append(g.Cooking, &pb.SimpleItem{Id: proto.String(key), Num: proto.Int32(15)})
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			makeReq := &pb.CSFishCooking{Fish: &pb.BaseResource{Id: proto.String("FishItem_029"), Num: proto.Int32(3)}}
			if _, err := s.fishCooking(base, id, makeReq, 100); err != nil {
				t.Fatal(err)
			}
			request := &pb.CSFishUsing{HeroGuid: proto.String(guid), CostItem: &pb.BaseResource{Id: proto.String("FishItem_029"), Num: proto.Int32(1)}}
			ctx := context.WithValue(context.WithValue(base, constUtil.GameServiceContextKey, s), constUtil.GameRequestContextKey, request)
			original, err := s.fishUsing(ctx, id, request, 200)
			if err != nil {
				t.Fatal(err)
			}
			pool, _ := db.GetIns().DB()
			pool.Close()
			if err := db.InitGame(base, path); err != nil {
				t.Fatal(err)
			}
			repeated, err := s.fishUsing(ctx, id, request, 200)
			if err != nil || !proto.Equal(original[0], repeated[0]) {
				t.Fatal("unchanged original food receipt must survive reconnect", err)
			}
			switch action {
			case "reset":
				if _, err := s.p5WarriorReset(base, id, &pb.CSWarriorReset{Guid: proto.String(guid)}, 300); err != nil {
					t.Fatal(err)
				}
			case "sale":
				if _, err := s.p5WarriorRecycle(base, id, []string{guid}, false, 300, &pb.CSWarriorSell{Guid: []string{guid}}); err != nil {
					t.Fatal(err)
				}
			case "later-growth":
				if _, err := s.fishUsing(ctx, id, request, 201); err != nil {
					t.Fatal(err)
				}
			}
			var before db.Profile
			if err := db.GetIns().First(&before, id).Error; err != nil {
				t.Fatal(err)
			}
			var receiptBefore db.CookingMutationReceipt
			if err := db.GetIns().First(&receiptBefore, "player_id=? AND opcode=?", id, constUtil.CookingUseOpcode).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := s.fishUsing(ctx, id, request, 200); err == nil {
				t.Fatal("retired original warrior food projection was replayed")
			}
			var after db.Profile
			if err := db.GetIns().First(&after, id).Error; err != nil || *before.Player != *after.Player || *before.Game != *after.Game {
				t.Fatal("rejected old food receipt changed paid state", err)
			}
			var receiptAfter db.CookingMutationReceipt
			if err := db.GetIns().First(&receiptAfter, "player_id=? AND opcode=?", id, constUtil.CookingUseOpcode).Error; err != nil {
				t.Fatal(err)
			}
			first, _ := json.Marshal(receiptBefore)
			second, _ := json.Marshal(receiptAfter)
			if string(first) != string(second) {
				t.Fatal("rejected historical projection destroyed audit")
			}
			if action == "reset" {
				// 伙伴重置已消费整卡并返碎片，剩余料理可用于仍持有的主角。
				p, _ := gemProfile(t, id)
				request.HeroGuid = proto.String(p.GetHeroWarId())
				if _, err := s.fishUsing(ctx, id, request, 400); err != nil {
					t.Fatal("remaining real dish was blocked after reset", err)
				}
			}
		})
	}
}
