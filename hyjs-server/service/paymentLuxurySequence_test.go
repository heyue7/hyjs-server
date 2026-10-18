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

// TestP3LuxuryOriginalChapterSequenceAndPaidCold 从原6-1真实ID30核原礼包合同。
// 历史通关是有限前提；原钱包订单、四件原奖励与冷恢复均走业务。
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
		before, game := f.profile(t)
		if _, err := f.s.paymentOffer(f.ctx, id, request, now); err == nil {
			t.Fatal("wrong chapter/global ID or zero stars opened original offer", info)
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
	if _, err := f.s.paymentOffer(f.ctx, id, request, now); err != nil {
		t.Fatal("actual original sixth chapter first mission30", err)
	}
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
