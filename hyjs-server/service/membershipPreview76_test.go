package service

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestMembership76NextTierPreviewAndMaximum(t *testing.T) {
	f, id := p3DemoProviderFixture(t)
	for _, vip := range []int32{0, 10, 12} {
		if err := updateProfile(f.ctx, id, func(p *pb.Player, _ *model.GameState) error {
			p.Vip = proto.Int32(vip)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		messages, err := f.s.membership(f.ctx, id, &pb.CSGetVipBoonInfo{}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		daily := messages[0].(*pb.SCGetVipBoonInfo).GetDaily()
		if messages[0].(*pb.SCGetVipBoonInfo).GetWeek()[0].GetDiscount() != "100" {
			t.Fatal("original integer discount contract changed")
		}
		if daily[0].GetVip() != vip || daily[0].GetReward()[0].GetNum() != (vip+1)*1000 {
			t.Fatal("current membership tier changed", daily)
		}
		if vip < 12 {
			if len(daily) != 2 || daily[1].GetVip() != vip+1 || daily[1].GetReward()[0].GetNum() != (vip+2)*1000 || daily[1].GetStatus() != 0 {
				t.Fatal("next tier missing or claimable", daily)
			}
		} else if len(daily) != 1 {
			t.Fatal("maximum tier fabricated a higher tier", daily)
		}
	}
}
