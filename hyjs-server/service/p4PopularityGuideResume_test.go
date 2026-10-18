package service

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP4PopularityEmptyRewardTabGuideQueryAndCold(t *testing.T) {
	for _, closed := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal-items-only", true: "whole-shop-closed"}[closed], func(t *testing.T) {
			f := p4Setup(t)
			if closed {
				if err := db.GetIns().Model(&db.P4CommerceRule{}).Where("kind=?", "popularity").Update("enabled", false).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
				p.NoviceGuideSchedule = proto.Int32(13)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			before, _ := f.profile(t)
			out, handled, err := f.s.DispatchP4(f.ctx, f.id, &pb.CSShopProductList{ShopType: proto.Int32(4)}, 0)
			if err != nil || !handled || len(out) != 2 {
				t.Fatal("original shop query and guide snapshot", handled, err, len(out))
			}
			if _, ok := out[0].(*pb.SCShopProductList); !ok || out[1].(*pb.SCPlayer).GetPlayer().GetNoviceGuideSchedule() != 14 {
				t.Fatal("guide cursor not delivered with original shop response")
			}
			current, _ := f.profile(t)
			want := proto.Clone(before).(*pb.Player)
			want.NoviceGuideSchedule = proto.Int32(14)
			if !proto.Equal(current, want) || p1Count(t, &db.P4PopularityRankClaim{}, "player_id=?", f.id) != 0 {
				t.Fatal("empty guide changed money, ownership or purchase facts")
			}
			connection, _ := db.GetIns().DB()
			connection.Close()
			f.open(t)
			out, _, err = f.s.DispatchP4(f.ctx, f.id, &pb.CSShopProductList{ShopType: proto.Int32(4)}, 0)
			current, _ = f.profile(t)
			if err != nil || len(out) != 1 || !proto.Equal(current, want) {
				t.Fatal("cold query repeated guide or altered progress", err)
			}
		})
	}
}

func TestP4PopularityRewardGuideKeepsConfiguredAndLaterSteps(t *testing.T) {
	f := p4Setup(t)
	p4PopularityPublish(t, f, time.Now())
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.NoviceGuideSchedule = proto.Int32(13)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.p4Commerce(f.ctx, f.id, &pb.CSShopProductList{ShopType: proto.Int32(4)}, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	p, _ := f.profile(t)
	if p.GetNoviceGuideSchedule() != 13 {
		t.Fatal("configured reward purchase step was skipped")
	}
	for _, cursor := range []int32{0, 12, 14, 15, 20} {
		p.NoviceGuideSchedule = proto.Int32(cursor)
		if p4PopularityEmptyGuide(p, nil) || p.GetNoviceGuideSchedule() != cursor {
			t.Fatal("unrelated guide changed", cursor)
		}
	}
}
