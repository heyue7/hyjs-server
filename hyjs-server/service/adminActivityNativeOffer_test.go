package service

import (
	"context"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

// TestAdminScheduleOriginalNativeLimitedOffer verifies the original standalone offer, not only generic buy.
func TestAdminScheduleOriginalNativeLimitedOffer(t *testing.T) {
	f, actor := adminModesFixture(t)
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	if err := f.s.InitializeAdminGameServers(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeAdminActivitySchedules(f.ctx); err != nil {
		t.Fatal(err)
	}
	form := adminModeForm("limited-shop")
	form.Products = []model.AdminActivityProduct{{ID: "calendar_native_offer", Name: "排期限时礼包", Index: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "item_005", Amount: 2}}, Cost: model.AdminResourceAmount{ResourceID: "diamond", Amount: 10}, DailyLimit: 1, TotalLimit: 1, Stock: -1, MinLevel: 1, DiscountBps: 10000, Weight: 1}}
	def, err := f.s.AdminSaveActivityDefinition(f.ctx, actor, "", model.AdminActivityDefinitionSaveRequest{Name: "限时原入口", Form: form, DefaultDays: 2, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	cal, err := f.s.AdminActivitySchedule(f.ctx, actor, 1)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	start := now.Add(-time.Hour).UnixMilli()
	end := now.Add(time.Hour).UnixMilli()
	_, err = f.s.AdminSaveActivitySchedule(f.ctx, actor, 1, model.AdminActivityScheduleSaveRequest{Version: cal.Version, Items: []model.AdminActivityOccurrenceDTO{{TemplateID: def.ID, TemplateVersion: def.Version, StartAt: start, EndAt: end, ClaimEndAt: end, Enabled: true}}})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := f.profile(t)
	benefitGiftBalance(t, f, 100)
	info := benefitGiftInfo(t, f, now)
	if info.GetTargetId() != "calendar_native_offer" || info.GetDiscountPrice() != 10 {
		t.Fatal("native entry ignored realm calendar", info)
	}
	messages, err := f.s.benefitGiftAt(f.ctx, f.id, &pb.CSBenefitGiftReward{}, 771901, now)
	if err != nil {
		t.Fatal(err)
	}
	first := messages[0]
	after, g := f.profile(t)
	if after.GetDiamond() != 90 || simpleOwned(after, g, "item_005") < 2 {
		t.Fatal("native offer did not grant configured trade")
	}
	_ = before
	f.restart(t)
	again, err := f.s.benefitGiftAt(f.ctx, f.id, &pb.CSBenefitGiftReward{}, 771901, now.Add(3*time.Hour))
	if err != nil || !proto.Equal(first, again[0]) {
		t.Fatal("native cross-window receipt changed", err)
	}
}
