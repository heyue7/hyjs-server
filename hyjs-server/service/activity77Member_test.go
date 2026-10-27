package service

import (
	"testing"
	"time"

	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestActivity77MemberWeeklyBudgetClaimAndRetry(t *testing.T) {
	f, actor := adminNativeFixture(t, true)
	if err := db.GetIns().AutoMigrate(&db.ClientUpdatePolicy{}); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Save(&db.ClientUpdatePolicy{ID: 1, Maintenance: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyActivity77Member(f.ctx, actor); err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyActivity77Member(f.ctx, actor); err != nil {
		t.Fatal(err)
	}
	form, _, _, err := f.s.adminNativeVIPPolicy(db.GetIns(), false)
	if err != nil {
		t.Fatal(err)
	}
	zone := time.FixedZone("UTC+8", 8*3600)
	monday := time.Date(2026, 10, 26, 12, 0, 0, 0, zone)
	var total int64
	for day := 0; day < 7; day++ {
		fresh, _, _, err := f.s.adminNativeVIPPolicy(db.GetIns(), false)
		if err != nil {
			t.Fatal(err)
		}
		projected, err := f.s.activity77MemberSunday(db.GetIns(), fresh, monday.AddDate(0, 0, day))
		if err != nil {
			t.Fatal(err)
		}
		tier, _ := adminNativeVIPTierFor(projected, 2)
		for _, r := range tier.DailyRewards {
			if r.ResourceID == "coin" {
				total += r.Amount - 3000
			}
		}
	}
	if total != 45000000 {
		t.Fatal("weekly redistribution", total)
	}
	base, _ := adminNativeVIPTierFor(form, 2)
	if len(base.DailyRewards) != 1 || base.DailyRewards[0].Amount != 3000+constUtil.Activity77MemberDayCoin {
		t.Fatal("base tier applied more than once", base)
	}
	id := adminNativeNewPlayer(t, f, "member77-paid", 2, 1000)
	var product db.PaymentProduct
	if err := db.GetIns().Where("kind=?", "month").First(&product).Error; err != nil {
		t.Fatal(err)
	}
	adminNativeConfirmedOrder(t, f, id, product.ID, 3000)
	now := time.Now()
	view, err := f.s.membership(f.ctx, id, &pb.CSGetVipBoonInfo{}, now)
	if err != nil {
		t.Fatal(err)
	}
	expected := view[0].(*pb.SCGetVipBoonInfo).GetDaily()[0].GetReward()[0].GetNum()
	var before db.Profile
	if err := db.GetIns().First(&before, id).Error; err != nil {
		t.Fatal(err)
	}
	paid, _, err := decodeProfile(&before)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.membership(f.ctx, id, &pb.CSGetVipDailyBoonReward{}, now); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.membership(f.ctx, id, &pb.CSGetVipDailyBoonReward{}, now); err == nil {
		t.Fatal("daily retry granted twice")
	}
	var after db.Profile
	if err := db.GetIns().First(&after, id).Error; err != nil {
		t.Fatal(err)
	}
	claimed, _, err := decodeProfile(&after)
	if err != nil || claimed.GetCoin()-paid.GetCoin() != int64(expected) {
		t.Fatal("query and claim differ", err)
	}
	f.restart(t)
	view, err = f.s.membership(f.ctx, id, &pb.CSGetVipBoonInfo{}, now)
	if err != nil || view[0].(*pb.SCGetVipBoonInfo).GetDaily()[0].GetStatus() != 2 {
		t.Fatal("cold claim lost", err)
	}
}
