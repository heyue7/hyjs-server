package service

import (
	"context"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

func TestMembershipRedPointsPaidClaimsNextDayAndExpiry(t *testing.T) {
	f, id := p3DemoProviderFixture(t)
	f.id = id
	f.s.P3AutoConfirmDemoPayments = true
	if err := f.s.InitializeLocalPlay(f.ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	check := func(at time.Time, month, vip int32) {
		t.Helper()
		ctx := context.WithValue(f.ctx, constUtil.LocalPlayBoundKey, true)
		ctx = context.WithValue(ctx, constUtil.LocalPlayOwnerKey, id)
		ctx = context.WithValue(ctx, constUtil.LocalPlayOffsetDaysKey, int64(at.Sub(now)/(24*time.Hour)))
		points, err := f.s.WelfareAchievementRedPoints(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]int32{}
		for _, p := range points.RedPointList {
			got[p.GetType()] = p.GetValue()
		}
		if got["monthCardReward"] != month || got["vipBoon"] != vip {
			t.Fatal("membership red points", got, month, vip)
		}
	}
	check(now, 0, 0)
	for i, kind := range []string{"month", "permanent"} {
		var product db.PaymentProduct
		if err := db.GetIns().Where("kind=? AND channels LIKE ?", kind, "%gf_default%").Order("id").First(&product).Error; err != nil {
			t.Fatal(err)
		}
		p3DemoProviderFund(t, f, id, "redpoint-"+kind, product.Cents)
		if _, err := f.s.generatePayment(f.ctx, id, p3DemoProviderRequest(t, product.ID), int64(80001+i), now); err != nil {
			t.Fatal(err)
		}
	}
	check(now, 1, 1)
	var before, after int64
	db.GetIns().Model(&db.MembershipClaim{}).Where("player_id=?", id).Count(&before)
	check(now, 1, 1)
	db.GetIns().Model(&db.MembershipClaim{}).Where("player_id=?", id).Count(&after)
	if before != after {
		t.Fatal("red query wrote claim rows")
	}
	for _, kind := range []string{"month", "permanent"} {
		if _, err := f.s.membership(f.ctx, id, &pb.CSClaimCard{CardType: proto.String(kind)}, now); err != nil {
			t.Fatal(err)
		}
	}
	check(now, 0, 1)
	if _, err := f.s.membership(f.ctx, id, &pb.CSGetVipDailyBoonReward{}, now); err != nil {
		t.Fatal(err)
	}
	check(now, 0, 0)
	f.restart(t)
	check(now, 0, 0)
	check(now.AddDate(0, 0, 1), 1, 1)
	// Permanent ownership remains while the monthly entitlement expires; claim it independently.
	future := now.AddDate(0, 0, 32)
	if _, err := f.s.membership(f.ctx, id, &pb.CSClaimCard{CardType: proto.String("permanent")}, future); err != nil {
		t.Fatal(err)
	}
	check(future, 0, 1)
}
