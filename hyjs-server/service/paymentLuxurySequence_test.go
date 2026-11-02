package service

import (
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Verify the legacy open query cannot create an offer from historical progress;
// original wallet payments, four equipment rewards and cold recovery remain intact.
func TestP3LuxuryOriginalChapterSequenceAndPaidCold(t *testing.T) {
	f, id := p3DemoProviderFixture(t)
	f.id = id
	f.s.P3AutoConfirmDemoPayments = true
	now := time.Now()
	request := &pb.CSOpenLuxuryEquipment{Chapter: proto.Int32(6), Mission: proto.Int32(1)}
	for _, info := range []*pb.CopySimpleInfo{
		{ChapterId: proto.String("6"), MissionId: proto.String("1"), Star: proto.Int32(5)},
		{ChapterId: proto.String("5"), MissionId: proto.String("30"), Star: proto.Int32(5)},
		{ChapterId: proto.String("6"), MissionId: proto.String("30"), Star: proto.Int32(0)},
	} {
		if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
			g.Copy.PassedCopyInfo = []*pb.CopySimpleInfo{info}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		luxuryTestStart(t, f.ctx, f.s, id, now)
		before, game := f.profile(t)
		out, err := f.s.paymentOffer(f.ctx, id, request, now)
		if err != nil || out[0].(*pb.SCOpenLuxuryEquipment).GetOpenOrNot() != 0 {
			t.Fatal("historical progress created an offer", info, err)
		}
		after, afterGame := f.profile(t)
		var offers int64
		if err := db.GetIns().Model(&db.PaymentLuxuryOffer{}).Where("player_id=?", id).Count(&offers).Error; err != nil || offers != 0 || !proto.Equal(before, after) || !reflect.DeepEqual(game, afterGame) {
			t.Fatal("rejected qualification changed saved state", err)
		}
	}
	if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
		g.Copy.PassedCopyInfo = []*pb.CopySimpleInfo{{ChapterId: proto.String("6"), MissionId: proto.String("30"), Star: proto.Int32(5)}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if out, err := f.s.paymentOffer(f.ctx, id, request, now); err != nil || out[0].(*pb.SCOpenLuxuryEquipment).GetOpenOrNot() != 0 {
		t.Fatal("legacy query created a window from historical completion", err)
	}
	luxuryTestStart(t, f.ctx, f.s, id, now)
	var offer db.PaymentLuxuryOffer
	if err := db.GetIns().First(&offer, "player_id=?", id).Error; err != nil || offer.OfferID != 2 || offer.Cents != 28800 || offer.EndAt-offer.StartAt != 12*3600*1000 {
		t.Fatal("original price/window changed", err)
	}
	if _, err := f.s.paymentOffer(f.ctx, id, request, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var reopened db.PaymentLuxuryOffer
	if err := db.GetIns().First(&reopened, "player_id=?", id).Error; err != nil || !reflect.DeepEqual(offer, reopened) {
		t.Fatal("opening again refreshed frozen window", err)
	}
	if _, err := f.s.paymentOffer(f.ctx, id, &pb.CSLuxuryEquipmentReward{}, now); err == nil {
		t.Fatal("unpaid suit claimed")
	}
	p3DemoProviderFund(t, f, id, "original-luxury288", 28800)
	if _, err := f.s.generatePayment(f.ctx, id, p3DemoProviderRequest(t, "djsy.wuyou.andp0288"), 286001, now.Add(time.Second)); err != nil {
		t.Fatal("actual funded original order", err)
	}
	if _, err := f.s.paymentOffer(f.ctx, id, &pb.CSLuxuryEquipmentReward{}, now.Add(2*time.Second)); err != nil {
		t.Fatal("actual paid offer claim", err)
	}
	_, game := f.profile(t)
	got := map[string]int{}
	for _, equipment := range game.Equipment {
		got[equipment.GetEquipId()]++
	}
	if !reflect.DeepEqual(got, map[string]int{"equip_033": 1, "equip_034": 1, "equip_035": 1, "equip_036": 1}) {
		t.Fatal("original four suit instances", got)
	}
	before, beforeGame := f.profile(t)
	f.restart(t)
	if _, err := f.s.paymentOffer(f.ctx, id, &pb.CSLuxuryEquipmentReward{}, now.Add(3*time.Second)); err == nil {
		t.Fatal("cold duplicate suit reward")
	}
	after, afterGame := f.profile(t)
	if !proto.Equal(before, after) || !reflect.DeepEqual(beforeGame, afterGame) {
		t.Fatal("cold duplicate changed full save")
	}
}
