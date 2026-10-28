package service

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestActivity77MemberCurvePreservesRewardsAndClaims(t *testing.T) {
	f, actor := adminNativeFixture(t, true)
	if err := db.GetIns().AutoMigrate(&db.ClientUpdatePolicy{}); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Save(&db.ClientUpdatePolicy{ID: 1, Maintenance: true}).Error; err != nil {
		t.Fatal(err)
	}
	old := adminNativeFind(t, f, actor, "vip-benefits")
	for i := range old.Form.VipTiers {
		tier := &old.Form.VipTiers[i]
		tier.DailyRewards = []model.AdminResourceAmount{{ResourceID: "item_005", Amount: int64(i + 1)}, {ResourceID: "coin", Amount: int64(6428571 + (i+1)*1000)}}
		tier.WeekCost.Amount = int64(100 + i)
		tier.WeekRewards[0].Amount = int64(10 + i)
	}
	// Existing tiers without a coin entry keep their other rewards and gain one.
	old.Form.VipTiers[1].DailyRewards = old.Form.VipTiers[1].DailyRewards[:1]
	saved, err := f.s.AdminSaveNativeActivity(f.ctx, actor, old.ID, model.AdminNativeActivitySaveRequest{Version: old.Version, Enabled: true, Alias: "保留备注", Form: old.Form})
	if err != nil {
		t.Fatal(err)
	}
	if err := adminAudit(db.GetIns(), actor, "activity77.member-daily", fmt.Sprint(f.s.OriginalServerID()), "旧每日方案"); err != nil {
		t.Fatal(err)
	}
	id := adminNativeNewPlayer(t, f, "member77-paid", 2, 1000)
	var product db.PaymentProduct
	if err := db.GetIns().Where("kind=?", "month").First(&product).Error; err != nil {
		t.Fatal(err)
	}
	adminNativeConfirmedOrder(t, f, id, product.ID, 3000)
	now := time.Now()
	initialView, err := f.s.membership(f.ctx, id, &pb.CSGetVipBoonInfo{}, now)
	if err != nil {
		t.Fatal(err)
	}
	week := initialView[0].(*pb.SCGetVipBoonInfo).GetWeek()[0]
	if _, err = f.s.membership(f.ctx, id, &pb.CSGetVipWeekBoonReward{Index: proto.Int32(1), CostResources: week.GetCostResource(), GetResources: week.GetGetResource()}, now); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.membership(f.ctx, id, &pb.CSGetVipDailyBoonReward{}, now); err != nil {
		t.Fatal(err)
	}
	var beforeClaim, beforeWeek db.MembershipClaim
	if err = db.GetIns().Where("player_id=? AND kind=? AND period=?", id, "vip:daily", p3Day(now)).First(&beforeClaim).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Where("player_id=? AND kind=? AND period=?", id, "vip:week", p3Week(now)).First(&beforeWeek).Error; err != nil {
		t.Fatal(err)
	}

	if err = f.s.ApplyActivity77Member(f.ctx, actor); err != nil {
		t.Fatal(err)
	}
	if err = f.s.ApplyActivity77Member(f.ctx, actor); err != nil {
		t.Fatal(err)
	}
	updated := adminNativeFind(t, f, actor, "vip-benefits")
	wantCoins := []int64{100000, 200000, 300000, 400000, 550000, 700000, 900000, 1100000, 1300000, 1600000, 2000000, 2500000, 3000000}
	if !updated.Enabled || updated.Alias != saved.Alias || len(updated.Form.VipTiers) != len(wantCoins) {
		t.Fatal("policy metadata changed", updated)
	}
	for i, tier := range updated.Form.VipTiers {
		if tier.Vip != int64(i) || len(tier.DailyRewards) != 2 || tier.DailyRewards[0] != (model.AdminResourceAmount{ResourceID: "item_005", Amount: int64(i + 1)}) || tier.DailyRewards[1] != (model.AdminResourceAmount{ResourceID: "coin", Amount: wantCoins[i]}) {
			t.Fatalf("VIP%d daily rewards: %+v", i, tier.DailyRewards)
		}
		if tier.WeekCost != saved.Form.VipTiers[i].WeekCost || !reflect.DeepEqual(tier.WeekRewards, saved.Form.VipTiers[i].WeekRewards) {
			t.Fatalf("VIP%d weekly rewards changed", i)
		}
	}
	var afterClaim db.MembershipClaim
	if err = db.GetIns().Where("player_id=? AND kind=? AND period=?", id, "vip:daily", p3Day(now)).First(&afterClaim).Error; err != nil || !reflect.DeepEqual(beforeClaim, afterClaim) {
		t.Fatal("existing daily receipt changed", err)
	}
	var afterWeek db.MembershipClaim
	if err = db.GetIns().Where("player_id=? AND kind=? AND period=?", id, "vip:week", p3Week(now)).First(&afterWeek).Error; err != nil || !reflect.DeepEqual(beforeWeek, afterWeek) {
		t.Fatal("existing weekly receipt changed", err)
	}
	if _, err = f.s.membership(f.ctx, id, &pb.CSGetVipDailyBoonReward{}, now); err == nil {
		t.Fatal("already claimed day granted twice")
	}
	if _, err = f.s.membership(f.ctx, id, &pb.CSGetVipWeekBoonReward{Index: proto.Int32(1), CostResources: week.GetCostResource(), GetResources: week.GetGetResource()}, now); err == nil {
		t.Fatal("already claimed week granted twice")
	}
	// Query every weekday with the old audit present; Sunday uses the same curve.
	for day := 1; day <= 7; day++ {
		view, e := f.s.membership(f.ctx, id, &pb.CSGetVipBoonInfo{}, now.AddDate(0, 0, day))
		if e != nil || view[0].(*pb.SCGetVipBoonInfo).GetDaily()[0].GetReward()[1].GetNum() != 300000 {
			t.Fatalf("day %d coin differs: %v", day, e)
		}
	}
	next := now.AddDate(0, 0, 1)
	var before db.Profile
	if err = db.GetIns().First(&before, id).Error; err != nil {
		t.Fatal(err)
	}
	paid, _, err := decodeProfile(&before)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.membership(f.ctx, id, &pb.CSGetVipDailyBoonReward{}, next); err != nil {
		t.Fatal(err)
	}
	var after db.Profile
	if err = db.GetIns().First(&after, id).Error; err != nil {
		t.Fatal(err)
	}
	claimed, _, err := decodeProfile(&after)
	if err != nil || claimed.GetCoin()-paid.GetCoin() != 300000 {
		t.Fatal("claim differs from preview", err)
	}
	f.restart(t)
	view, err := f.s.membership(f.ctx, id, &pb.CSGetVipBoonInfo{}, next)
	if err != nil || view[0].(*pb.SCGetVipBoonInfo).GetDaily()[0].GetStatus() != 2 || view[0].(*pb.SCGetVipBoonInfo).GetDaily()[0].GetReward()[1].GetNum() != 300000 {
		t.Fatal("cold policy or claim lost", err)
	}
	var auditCount int64
	if err = db.GetIns().Model(&db.AdminAudit{}).Where("action=? AND target=?", constUtil.Activity77MemberAudit, fmt.Sprint(f.s.OriginalServerID())).Count(&auditCount).Error; err != nil || auditCount != 1 {
		t.Fatal("curve audit not idempotent", auditCount, err)
	}
	updated.Form.VipTiers[2].DailyRewards[1].Amount = 321000
	if _, err = f.s.AdminSaveNativeActivity(f.ctx, actor, updated.ID, model.AdminNativeActivitySaveRequest{Version: updated.Version, Enabled: updated.Enabled, Alias: updated.Alias, Form: updated.Form}); err != nil {
		t.Fatal(err)
	}
	if err = f.s.ApplyActivity77Member(f.ctx, actor); err != nil {
		t.Fatal(err)
	}
	if latest := adminNativeFind(t, f, actor, "vip-benefits"); latest.Form.VipTiers[2].DailyRewards[1].Amount != 321000 {
		t.Fatal("retry overwrote later admin change")
	}
}
