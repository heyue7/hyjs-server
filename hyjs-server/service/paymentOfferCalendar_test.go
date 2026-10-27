package service

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// 原订单实际支付事实保留；日历资格不得提前借用未来时刻的订单。
func TestLocalPlayPaymentOfferAsOfConfirmedTime(t *testing.T) {
	f, id, owner := localClockFixture(t)
	localClockEnable(t)
	ctx := localClockBind(t, f, owner)
	f.s.P3AutoConfirmDemoPayments = true
	p3DemoProviderFund(t, f, id, "offer-as-of", 28800)
	local := time.Now().In(time.FixedZone("UTC+8", 8*3600))
	paidAt := local.Add(-2 * time.Second)
	// This test opts into a configured daily reward; production keeps missing rewards closed.
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.ActivityRules{}, &db.ActivityReward{}); err != nil {
		t.Fatal(err)
	}
	for _, row := range []any{
		&db.ActivityRules{ID: constUtil.P3LuxurySignActivityID, Enabled: true, StartAt: paidAt.Add(-2 * time.Hour).UnixMilli(), EndAt: paidAt.Add(2 * time.Hour).UnixMilli(), RewardEndAt: paidAt.Add(2 * time.Hour).UnixMilli(), Source: "isolated as-of payment test"},
		&db.ActivityReward{ActivityID: constUtil.P3LuxurySignActivityID, RewardType: constUtil.P3LuxurySignRewardType, Reward: "coin,1"},
	} {
		if err := tx.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	// 隔离历史通关资格沿现有神装测试；不声称自然打通原6-1。
	if err := updateProfile(ctx, id, func(p *pb.Player, g *model.GameState) error {
		g.Copy.PassedCopyInfo = append(g.Copy.PassedCopyInfo, &pb.CopySimpleInfo{ChapterId: proto.String("6"), MissionId: proto.String("30"), Star: proto.Int32(5)})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.paymentOffer(ctx, id, &pb.CSOpenLuxuryEquipment{Chapter: proto.Int32(6), Mission: proto.Int32(1)}, paidAt.Add(-time.Hour)); err != nil {
		t.Fatal("original qualified offer", err)
	}
	out, err := f.s.generatePayment(ctx, id, p3DemoProviderRequest(t, "djsy.wuyou.andp0288"), 19001, paidAt)
	if err != nil {
		t.Fatal("original funded payment", err)
	}
	orderID := out[0].(*pb.SCGenerateOrder).GetOrderId()
	var order db.PaymentOrder
	if err = db.GetIns().First(&order, "id=?", orderID).Error; err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		at   time.Time
		want int32
	}{{paidAt.Add(-time.Millisecond), 0}, {paidAt, 1}, {paidAt.Add(time.Hour), 1}} {
		status, e := f.s.luxurySignStatus(db.GetIns().WithContext(ctx), id, tc.at)
		if e != nil || status != tc.want {
			t.Fatal("original paid-day qualification at", tc.at, status, tc.want, e)
		}
		info, e := f.s.paymentOffer(ctx, id, &pb.CSLuxuryEquipmentInfo{}, tc.at)
		if e != nil || info[0].(*pb.SCLuxuryEquipmentInfo).GetRewardState() != tc.want {
			t.Fatal("original chapter offer borrowed a future payment", tc.at, e)
		}
	}
	var offer db.PaymentLuxuryOffer
	if err = db.GetIns().Where("player_id=?", id).First(&offer).Error; err != nil {
		t.Fatal(err)
	}
	for _, at := range []time.Time{time.UnixMilli(offer.StartAt - 1), time.UnixMilli(offer.EndAt)} {
		info, e := f.s.paymentOffer(ctx, id, &pb.CSLuxuryEquipmentInfo{}, at)
		if e != nil || info[0].(*pb.SCLuxuryEquipmentInfo).GetId() != 0 || info[0].(*pb.SCLuxuryEquipmentInfo).GetEndTime() != at.UnixMilli() {
			t.Fatal("offer advertised outside its frozen window", e)
		}
		if _, e = f.s.paymentOffer(ctx, id, &pb.CSLuxuryEquipmentReward{}, at); e == nil {
			t.Fatal("outside-window suit claim accepted")
		}
		opened, e := f.s.paymentOffer(ctx, id, &pb.CSOpenLuxuryEquipment{Chapter: proto.Int32(6), Mission: proto.Int32(1)}, at)
		if e != nil || opened[0].(*pb.SCOpenLuxuryEquipment).GetOpenOrNot() != 0 {
			t.Fatal("outside-window offer reopened", e)
		}
	}
	if _, err = f.s.paymentOffer(ctx, id, &pb.CSLuxuryEquipmentReward{}, paidAt.Add(-time.Millisecond)); err == nil {
		t.Fatal("future payment granted an early suit")
	}
	if _, err = f.s.paymentOffer(ctx, id, &pb.CSLuxuryEquipmentReward{}, paidAt); err != nil {
		t.Fatal("actual confirmed-time suit claim", err)
	}
	info, err := f.s.paymentOffer(ctx, id, &pb.CSLuxuryEquipmentInfo{}, paidAt)
	if err != nil || info[0].(*pb.SCLuxuryEquipmentInfo).GetRewardState() != 2 {
		t.Fatal("claimed offer status lost", err)
	}
	opened, err := f.s.paymentOffer(ctx, id, &pb.CSOpenLuxuryEquipment{Chapter: proto.Int32(6), Mission: proto.Int32(1)}, paidAt)
	if err != nil || opened[0].(*pb.SCOpenLuxuryEquipment).GetOpenOrNot() != 0 {
		t.Fatal("claimed offer reopened", err)
	}
	var after db.PaymentOrder
	if err = db.GetIns().First(&after, "id=?", orderID).Error; err != nil || after.ConfirmedAt != order.ConfirmedAt || after.Cents != order.Cents {
		t.Fatal("as-of query changed financial fact", err)
	}
}
