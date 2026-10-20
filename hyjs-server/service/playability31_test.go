package service

import (
	"context"
	"encoding/json"
	"errors"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
	"time"
)

func TestPlayability31NativeRemovalPreservesHistory(t *testing.T) {
	f, operator := adminNativeFixture(t, false)
	item := adminNativeFind(t, f, operator, "vip-benefits")
	// 启用配置必须先停用，旧版本和只读账号不能移除。
	if item.Enabled {
		if err := f.s.AdminRemoveNativeActivity(f.ctx, operator, item.ID, item.Version); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
			t.Fatal(err)
		}
		next, err := f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: item.Version, Enabled: false, Alias: item.Alias, Form: item.Form})
		if err != nil {
			t.Fatal(err)
		}
		item = *next
	}
	if err := f.s.AdminRemoveNativeActivity(f.ctx, operator, item.ID, "stale"); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal(err)
	}
	if err := f.s.AdminRemoveNativeActivity(f.ctx, nil, item.ID, item.Version); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatal(err)
	}
	var before int64
	if err := db.GetIns().Model(&db.Profile{}).Count(&before).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.AdminRemoveNativeActivity(f.ctx, operator, item.ID, item.Version); err != nil {
		t.Fatal(err)
	}
	rows, err := f.s.AdminNativeActivities(f.ctx, operator)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.ID == item.ID {
			t.Fatal("removed row remained visible")
		}
	}
	if _, err = f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: item.Version, Enabled: true, Form: item.Form}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale editor revived removed activity", err)
	}
	var after int64
	if err = db.GetIns().Model(&db.Profile{}).Count(&after).Error; err != nil || after != before {
		t.Fatal("removal changed profiles", err)
	}
	var meta db.AdminActivityPresentation
	if err = db.GetIns().First(&meta, "activity_id=?", item.ID).Error; err != nil || !meta.Archived {
		t.Fatal(err)
	}
}

func TestPlayability31ShopPotionOnlyAndUse(t *testing.T) {
	f := p4Setup(t)
	p4Fund(t, f)
	if err := db.GetIns().Model(&db.P4CommerceRule{}).Where("kind = ?", "ordinary").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	// 原药剂购买只增加库存，随后使用才增加25体力。
	before, _ := f.profile(t)
	out, err := f.s.p4Commerce(f.ctx, f.id, &pb.CSShopBuy{ShopType: proto.Int32(1), ShopItemId: proto.String("itemShopId_4"), BuyCount: proto.Int32(1)}, 9001, LocalPlayNow(f.ctx))
	if err != nil {
		t.Fatal(err)
	}
	after, _ := f.profile(t)
	if after.GetStamina() != before.GetStamina() {
		t.Fatal("buy directly changed stamina")
	}
	if len(out) == 0 {
		t.Fatal("missing purchase result")
	}
	reply, err := f.s.itemUse(f.ctx, f.id, &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_002"), Num: proto.Int32(1)}})
	if err != nil {
		t.Fatal(err)
	}
	used, _ := f.profile(t)
	if used.GetStamina() != after.GetStamina()+25 || reply[0].(*pb.SCItemUse).GetCost().GetId() != "item_002" {
		t.Fatal("potion use did not use native item")
	}
	if _, err = f.s.p4Commerce(f.ctx, f.id, &pb.CSShopBuy{ShopType: proto.Int32(1), ShopItemId: proto.String("itemShopId_2"), BuyCount: proto.Int32(1)}, 9002, LocalPlayNow(f.ctx)); err == nil {
		t.Fatal("retired direct stamina SKU bought")
	}
}

func TestPlayability31FieldReconnectReleasesOnlyUnsettledOccupancy(t *testing.T) {
	f := p1Setup(t)
	for _, load := range []func(string) error{f.s.LoadP5Tables, f.s.LoadP8Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeDailyCopy, f.s.InitializeP8Gather, f.s.InitializeP8Life, f.s.InitCooking} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(30); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.fieldTransfer(f.ctx, f.id, &pb.CSTransferScene{Target: proto.String("field_001")}); err != nil {
		t.Fatal(err)
	}
	_, g := f.profile(t)
	target := g.Field.Monsters[0].GetUuid()
	starts, err := f.s.fieldFightStart(f.ctx, f.id, &pb.CSSidePveFightStart{MonsterId: proto.String(target)})
	if err != nil {
		t.Fatal(err)
	}
	start := starts[0].(*pb.SCSidePveFightStart)
	before, bg := f.profile(t)
	if bg.Field.Fight == nil || bg.Field.Side.GetState() != 6 {
		t.Fatal("missing active fight")
	}
	messages, err := f.s.fieldScene(f.ctx, f.id, true)
	if err != nil {
		t.Fatal(err)
	}
	after, ag := f.profile(t)
	if ag.Field.Fight != nil || ag.Field.Side.GetState() != 0 || fieldFind(ag, target).GetState() != 1 {
		t.Fatal("reconnect remained occupied")
	}
	if !proto.Equal(before, after) || ag.Field.RewardKills != bg.Field.RewardKills || ag.Field.Side.GetCurHp() != bg.Field.Side.GetCurHp() || ag.Field.Side.GetTiredness() != bg.Field.Side.GetTiredness() {
		t.Fatal("reconnect granted resources or healed")
	}
	if len(messages) < 2 {
		t.Fatal("native scene recovery frames missing")
	}
	if _, err = f.s.fieldFightEnd(f.ctx, f.id, &pb.CSSidePveFightEnd{FightId: start.FightId}); err == nil {
		t.Fatal("abandoned result accepted")
	}
	if _, err = f.s.fieldTransfer(f.ctx, f.id, &pb.CSTransferScene{Target: proto.String("town")}); err != nil {
		t.Fatal("recovered player cannot leave", err)
	}
}

// 当前客户端只看到本渠道卡商品，原渠道记录和个人活动副本仍在库内。
func TestPlayability31NativeListUsesClientChannelAndPublicSources(t *testing.T) {
	f, operator := adminNativeFixture(t, true)
	var before int64
	if err := db.GetIns().Model(&db.PaymentProduct{}).Count(&before).Error; err != nil {
		t.Fatal(err)
	}
	rows, err := f.s.AdminNativeActivities(f.ctx, operator)
	if err != nil {
		t.Fatal(err)
	}
	cards := 0
	for _, row := range rows {
		if strings.Contains(row.ID, "_lp-") {
			t.Fatal("private calendar leaked into global list")
		}
		if row.Kind == "month-card" || row.Kind == "permanent-card" {
			cards++
			var product db.PaymentProduct
			if err := db.GetIns().First(&product, "id=?", strings.TrimPrefix(row.ID, "product:")).Error; err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(product.Channels, "gf_default") {
				t.Fatal("unavailable channel shown")
			}
		}
	}
	if cards != 2 {
		t.Fatal("expected one native card per kind", cards)
	}
	var after int64
	db.GetIns().Model(&db.PaymentProduct{}).Count(&after)
	if after != before {
		t.Fatal("channel cleanup deleted payment records")
	}
}

func TestPlayability31GuildBoxNewRulesKeepCurrentDay(t *testing.T) {
	f, ids, now := p13GuildDonateBoxSetup(t)
	actor := adminGuildTestActor(t, constUtil.AdminRoleOperator)
	if err := f.s.InitializeAdminGuildSettings(f.ctx); err != nil {
		t.Fatal(err)
	}
	p13GuildDonateBoxEnable(t, now)
	p13GuildCall(t, f, ids[0], &pb.CSUnionDonate{DonateType: proto.String("coin")}, 1000, now)
	before := p13GuildCall(t, f, ids[0], &pb.CSUnionStore{}, 0, now).(*pb.SCUnionStore)
	setting, err := f.s.AdminGuildSettings(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	tiers := []model.AdminGuildDonateBoxTier{{Threshold: 1, Reward: "coin,11"}, {Threshold: 2, Reward: "coin,22"}, {Threshold: 3, Reward: "coin,33"}, {Threshold: 4, Reward: "coin,44"}}
	_, err = f.s.AdminSaveGuildSettings(f.ctx, actor, model.AdminGuildSettingSaveRequest{Version: setting.Version, Reason: "调整四档宝箱", Policies: &model.AdminGuildPolicySave{DonateBox: &model.AdminGuildDonateBoxPolicy{Enabled: true, Tiers: tiers}}})
	if err != nil {
		t.Fatal(err)
	}
	current := p13GuildCall(t, f, ids[0], &pb.CSUnionStore{}, 0, now).(*pb.SCUnionStore)
	if !proto.Equal(before, current) {
		t.Fatal("publication rewrote already accumulating day")
	}
	next := now.Add(24 * time.Hour)
	preview := p13GuildCall(t, f, ids[0], &pb.CSUnionStore{}, 0, next).(*pb.SCUnionStore)
	if preview.GetBoxes()[0].GetScore() != 1 || preview.GetBoxes()[0].GetReward() != "coin,11" {
		t.Fatal("new day ignored saved tiers", preview)
	}
	p13GuildCall(t, f, ids[0], &pb.CSUnionDonate{DonateType: proto.String("coin")}, 1001, next)
	earned := p13GuildCall(t, f, ids[0], &pb.CSUnionStore{}, 0, next).(*pb.SCUnionStore)
	if earned.GetBoxes()[0].GetState() != 1 {
		t.Fatal("real new-day donation not counted", earned)
	}
	var policy db.P13GuildDonateBoxPolicy
	if err := db.GetIns().First(&policy, 1).Error; err != nil {
		t.Fatal(err)
	}
	var stored []model.AdminGuildDonateBoxTier
	if json.Unmarshal([]byte(policy.RulesJSON), &stored) != nil || stored[0].Reward != "coin,11" {
		t.Fatal("rules not durable")
	}
}

func TestPlayability31GuildDonationUsesPublishedConfig(t *testing.T) {
	f, ids := p13GuildSetup(t)
	now := time.Now()
	actor := adminGuildTestActor(t, constUtil.AdminRoleOperator)
	if err := f.s.InitializeAdminGuildSettings(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP13GuildDonationBadge(f.ctx); err != nil {
		t.Fatal(err)
	}
	adminGuildTestCreate(t, f, ids[0], "规则生效验证")
	before, err := f.s.AdminGuildSettings(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.AdminSaveGuildSettings(f.ctx, actor, model.AdminGuildSettingSaveRequest{Version: before.Version, Reason: "调整贡献与徽章", Fields: []model.AdminGuildConfigValue{{Key: "UnionDonateCoinContributVal", Value: "17"}}, Policies: &model.AdminGuildPolicySave{DonationBadge: &model.AdminGuildDonationBadgePolicy{Enabled: true, CoinCost: 100000, BadgeAmount: 3, DailyLimit: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	request := &model.GamePacket{Body: &pb.CSUnionDonate{DonateType: proto.String("coin")}}
	runtime, err := f.s.guildRuntimeForRequest(f.ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	now = time.Now().Add(time.Second)
	playerBefore, _ := p10LuckyProfile(t, ids[0])
	out := p13GuildCall(t, &p1Fixture{s: runtime, ctx: f.ctx}, ids[0], request.Body, 100, now).(*pb.SCUnionDonate)
	playerAfter, _ := p10LuckyProfile(t, ids[0])
	if out.GetContributionAdd() != 17 || playerAfter.GetContribution() != playerBefore.GetContribution()+17 || playerAfter.GetCoin() != playerBefore.GetCoin()-100000 {
		t.Fatal("published donation rules did not affect actual debit and contribution", out)
	}
	var source db.P13GuildDonationBadgeSource
	if err := db.GetIns().First(&source, "player_id=?", ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	if source.BadgeAmount != 3 || source.DailyLimit != 2 {
		t.Fatal("published badge reward was ignored")
	}
	fresh, err := f.s.AdminGuildSettings(f.ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.AdminSaveGuildSettings(f.ctx, actor, model.AdminGuildSettingSaveRequest{Version: fresh.Version, Reason: "更新后续徽章", Policies: &model.AdminGuildPolicySave{DonationBadge: &model.AdminGuildDonationBadgePolicy{Enabled: true, CoinCost: 100000, BadgeAmount: 4, DailyLimit: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err = f.s.guildRuntimeForRequest(f.ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	p13GuildCall(t, &p1Fixture{s: runtime, ctx: f.ctx}, ids[0], request.Body, 101, now.Add(time.Second))
	var sources []db.P13GuildDonationBadgeSource
	db.GetIns().Where("player_id=?", ids[0]).Order("ordinal").Find(&sources)
	if len(sources) != 2 || sources[0].BadgeAmount != 3 || sources[1].BadgeAmount != 4 {
		t.Fatal("new reward rewrote frozen earlier donation", sources)
	}
}
