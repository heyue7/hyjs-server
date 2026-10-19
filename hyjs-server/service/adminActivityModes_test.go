package service

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func adminModesFixture(t *testing.T) (*p1Fixture, *model.AdminPrincipal) {
	t.Helper()
	f := p4Setup(t)
	for _, initialize := range []func() error{func() error { return AdminInitialize(f.ctx, "") }, func() error { return InitializeAdminOperations(f.ctx) }, func() error { return InitializeAdminActivityModes(f.ctx) }} {
		if err := initialize(); err != nil {
			t.Fatal(err)
		}
	}
	user := db.AdminUser{Username: "mode-operator-" + p6Digest([]byte(t.Name()))[:24], PasswordHash: "unused-fixture", Role: constUtil.AdminRoleOperator, Enabled: true}
	if err := db.GetIns().Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	return f, &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
}
func adminModeForm(mode string) model.AdminActivityForm {
	now := time.Now().UnixMilli()
	return model.AdminActivityForm{Mode: mode, TemplateID: mode + "-original", Name: "运营测试", Source: "finite typed consumer fixture", StartAt: now - 1000, EndAt: now + 86400000, ClaimEndAt: now + 86400000, Products: []model.AdminActivityProduct{}, Milestones: []model.AdminActivityMilestone{}, Prizes: []model.AdminActivityPrize{}}
}

// TestAdminTypedActivityOriginalCommerceVIPAndCold writes typed configuration,
// then uses original activity list/buy messages, real VIP denial, frozen receipts and cold once-only grant.
func TestAdminTypedActivityOriginalCommerceVIPAndCold(t *testing.T) {
	f, operator := adminModesFixture(t)
	form := adminModeForm("discount-shop")
	form.Products = []model.AdminActivityProduct{{ID: "typed_vip_product", Name: "指定礼包", Index: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "item_005", Amount: 2}}, Cost: model.AdminResourceAmount{ResourceID: "diamond", Amount: 10}, DailyLimit: 2, TotalLimit: 2, Stock: -1, MinLevel: 10, MinVip: 2, DiscountBps: 10000, Weight: 1}}
	created, err := f.s.AdminSaveTypedActivity(f.ctx, operator, "", model.AdminTypedActivityRequest{AdminActivityForm: form, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	_, key, _ := strings.Cut(created.ID, ":")
	if !strings.HasPrefix(key, "discountSale_p4_admin_") {
		t.Fatal("original activity prefix not preserved")
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Level = proto.Int32(20)
		p.Vip = proto.Int32(0)
		p.Diamond = proto.Int64(100)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	info := &pb.CSActivityShopInfo{ActivityId: proto.String(key)}
	if _, err := f.s.p4Commerce(f.ctx, f.id, info, 0, time.Now()); err != nil {
		t.Fatal("original info cannot consume new typed configuration", err)
	}
	buy := &pb.CSGetActivityShopReward{ActivityId: proto.String(key), Index: proto.Int32(1), CostResources: []*pb.BaseResource{{Id: proto.String("diamond"), Num: proto.Int32(10)}}, GetResources: []*pb.BaseResource{{Id: proto.String("item_005"), Num: proto.Int32(2)}}}
	beforeP, beforeG := f.profile(t)
	beforeRaw, _ := json.Marshal(beforeG)
	if _, err := f.s.p4Commerce(f.ctx, f.id, buy, 710001, time.Now()); err == nil {
		t.Fatal("VIP0 bought VIP2 product")
	}
	afterP, afterG := f.profile(t)
	afterRaw, _ := json.Marshal(afterG)
	if !proto.Equal(beforeP, afterP) || string(beforeRaw) != string(afterRaw) {
		t.Fatal("failed VIP eligibility changed currency, stock or inventory")
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Vip = proto.Int32(2); return nil }); err != nil {
		t.Fatal(err)
	}
	output, err := f.s.p4Commerce(f.ctx, f.id, buy, 710001, time.Now())
	if err != nil {
		t.Fatal("qualified original purchase", err)
	}
	paidP, paidG := f.profile(t)
	if paidP.GetDiamond() != 90 || simpleOwned(paidP, paidG, "item_005") != 2 {
		t.Fatal("actual original transaction reward/cost mismatch")
	}
	rows, err := f.s.AdminActivityInstances(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	var current model.AdminActivityInstance
	for _, row := range rows {
		if row.ID == created.ID {
			current = row
		}
	}
	if !current.Frozen || current.Editable {
		t.Fatal("actual consumer did not freeze source")
	}
	if _, err := f.s.AdminSaveTypedActivity(f.ctx, operator, current.ID, model.AdminTypedActivityRequest{AdminActivityForm: current.Form, Version: current.Version, Enabled: true}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("frozen form overwritten", err)
	}
	f.restart(t)
	replay, err := f.s.p4Commerce(f.ctx, f.id, buy, 710001, time.Now())
	if err != nil || !proto.Equal(output[0], replay[0]) {
		t.Fatal("original commerce cold receipt", err)
	}
	coldP, coldG := f.profile(t)
	if coldP.GetDiamond() != paidP.GetDiamond() || simpleOwned(coldP, coldG, "item_005") != 2 {
		t.Fatal("cold purchase paid twice")
	}
	if err := f.s.AdminDeleteTypedActivity(f.ctx, operator, current.ID, current.Version); err != nil {
		t.Fatal(err)
	}
	if p1Count(t, &db.P4CommerceState{}, "player_id = ? AND rule_id = ?", f.id, key) != 1 {
		t.Fatal("archive deleted player quota/history")
	}
}

// TestAdminTypedLoginOriginalConsumerAndTemplateWhitelist checks original login messages and no fabricated initial reward.
func TestAdminTypedLoginOriginalConsumerAndTemplateWhitelist(t *testing.T) {
	f, operator := adminModesFixture(t)
	form := adminModeForm("login-rewards")
	form.Milestones = []model.AdminActivityMilestone{{Goal: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 13}}}}
	wrong := form
	wrong.TemplateID = "turntable-original"
	if _, err := f.s.AdminSaveTypedActivity(f.ctx, operator, "", model.AdminTypedActivityRequest{AdminActivityForm: wrong, Enabled: true}); !errors.Is(err, constUtil.ErrAdminInput) {
		t.Fatal("cross-mode original art template accepted", err)
	}
	created, err := f.s.AdminSaveTypedActivity(f.ctx, operator, "", model.AdminTypedActivityRequest{AdminActivityForm: form, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	out, err := f.s.p4Welfare(f.ctx, f.id, &pb.CSLoginRewardInfo{}, 0, time.Now())
	if err != nil || len(out[0].(*pb.SCLoginRewardInfo).GetItems()) != 1 {
		t.Fatal("original login query cannot find typed source", err)
	}
	if _, err := f.s.p4Welfare(f.ctx, f.id, &pb.CSGetLoginReward{}, 720001, time.Now()); err == nil {
		t.Fatal("unearned typed login awarded")
	}
	f.event(t, "typed-original-login-day", constUtil.P1EventLoginDay, 1, time.Now())
	claimed, err := f.s.p4Welfare(f.ctx, f.id, &pb.CSGetLoginReward{}, 720001, time.Now())
	if err != nil {
		t.Fatal("earned original login claim", err)
	}
	p, _ := f.profile(t)
	if p.GetCoin() != 13 {
		t.Fatal("typed login actual amount")
	}
	f.restart(t)
	again, err := f.s.p4Welfare(f.ctx, f.id, &pb.CSGetLoginReward{}, 720001, time.Now())
	if err != nil || !proto.Equal(claimed[0], again[0]) {
		t.Fatal("login cold receipt", err)
	}
	modes := f.s.AdminActivityModes()
	found := false
	for _, mode := range modes {
		if mode.Mode == "exchange" {
			for _, condition := range mode.Conditions {
				if condition.Key == "minVip" {
					found = len(condition.Options) == 13 && condition.Options[12].Value == 12
				}
			}
		}
	}
	if !found {
		t.Fatal("VIP options do not follow original 0..12 table")
	}
	_ = created
}

// TestAdminActivityReadableAvailabilityAndPrivateHistory keeps disabled valid activities usable while separating unsupported and incomplete records.
func TestAdminActivityReadableAvailabilityAndPrivateHistory(t *testing.T) {
	f, operator := adminModesFixture(t)
	form := adminModeForm("exchange")
	form.Products = []model.AdminActivityProduct{{ID: "catalogue_product", Name: "确认的奖励", Index: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "item_005", Amount: 1}}, Cost: model.AdminResourceAmount{ResourceID: "diamond", Amount: 10}, DailyLimit: 1, Stock: -1, MinLevel: 1, DiscountBps: 10000, Weight: 1}}
	created, err := f.s.AdminSaveTypedActivity(f.ctx, operator, "", model.AdminTypedActivityRequest{AdminActivityForm: form})
	if err != nil {
		t.Fatal(err)
	}
	if created.Enabled || !created.Availability.Usable {
		t.Fatalf("关闭的完整活动被错误分类: %+v", created.Availability)
	}
	raw, _, err := adminActivityRaw(db.GetIns().WithContext(f.ctx), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := adminActivityReadableName("exchange · limitExchange_p4_gem_202610", "commerce:limitExchange_p4_gem_202610", "exchange", "exchange"); got != "宝石兑换" {
		t.Fatal(got)
	}
	if got := adminActivityReadableName("我的周末活动", raw.ID, "exchange", "exchange"); got != "我的周末活动" {
		t.Fatal("覆盖运营名称", got)
	}
	private := raw
	private.ID = "commerce:limitExchange_p4_lp-testhistory"
	if state := f.s.adminActivityAvailability(db.GetIns().WithContext(f.ctx), private, form, true); state.Usable || !strings.Contains(state.Reason, "个人活动期") {
		t.Fatalf("个人期未独立标记: %+v", state)
	}
	if state := f.s.adminActivityAvailability(db.GetIns().WithContext(f.ctx), raw, form, false); state.Usable || !strings.Contains(state.Reason, "尚未适配") {
		t.Fatal(state)
	}
	blank := adminModeForm("login-rewards")
	draft, err := f.s.AdminSaveTypedActivity(f.ctx, operator, "", model.AdminTypedActivityRequest{AdminActivityForm: blank})
	if err != nil {
		t.Fatal(err)
	}
	if draft.Availability.Usable || !draft.Editable || len(draft.Availability.Missing) == 0 {
		t.Fatalf("空草稿应可补齐且不能启用: %+v", draft)
	}
	if _, err := f.s.AdminTypedActivityState(f.ctx, operator, draft.ID, model.AdminActivityStateRequest{Version: draft.Version, Enabled: true}); err == nil {
		t.Fatal("空奖历通过启用入口")
	}
	blank.Milestones = []model.AdminActivityMilestone{{Goal: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 1}}}}
	fixed, err := f.s.AdminSaveTypedActivity(f.ctx, operator, draft.ID, model.AdminTypedActivityRequest{AdminActivityForm: blank, Version: draft.Version})
	if err != nil || !fixed.Availability.Usable || fixed.Enabled {
		t.Fatalf("补齐后不能回正常列表或意外启用: %+v %v", fixed, err)
	}
}
