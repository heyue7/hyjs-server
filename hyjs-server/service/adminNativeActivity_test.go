package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// adminNativeFixture 复用隔离P4/P1数据库，只补本域策略表和后台展示表。
func adminNativeFixture(t *testing.T, payment bool) (*p1Fixture, *model.AdminPrincipal) {
	t.Helper()
	f := p4Setup(t)
	if payment {
		if err := f.s.LoadPaymentTables("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
		if err := f.s.InitializePayments(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	for _, initialize := range []func() error{func() error { return AdminInitialize(f.ctx, "") }, func() error { return InitializeAdminOperations(f.ctx) }, func() error { return f.s.InitializeActivities(f.ctx) }, func() error { return f.s.InitializeP4CopyBenefits(f.ctx) }, func() error { return InitializeAdminNativeActivitySettings(f.ctx) }, func() error { return InitializeAdminActivityModes(f.ctx) }} {
		if err := initialize(); err != nil {
			t.Fatal(err)
		}
	}
	user := db.AdminUser{Username: "native-operator-" + p6Digest([]byte(t.Name()))[:24], PasswordHash: "unused-fixture", Role: constUtil.AdminRoleOperator, Enabled: true}
	if err := db.GetIns().Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	return f, &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
}

func adminNativeFind(t *testing.T, f *p1Fixture, p *model.AdminPrincipal, kind string) model.AdminNativeActivityDTO {
	t.Helper()
	items, err := f.s.AdminNativeActivities(f.ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.Kind == kind {
			return item
		}
	}
	t.Fatalf("missing native activity kind %s", kind)
	return model.AdminNativeActivityDTO{}
}

// TestAdminNativeListRequiresAdmin 验证列表读取与保存使用同一后台账号口径。
func TestAdminNativeListRequiresAdmin(t *testing.T) {
	f, _ := adminNativeFixture(t, false)
	if _, err := f.s.AdminNativeActivities(f.ctx, nil); !errors.Is(err, constUtil.ErrAdminPermission) {
		t.Fatalf("nil principal accepted: %v", err)
	}
	viewer := db.AdminUser{Username: "native-viewer-" + p6Digest([]byte(t.Name()))[:24], PasswordHash: "unused-fixture", Role: constUtil.AdminRoleViewer, Enabled: true}
	if err := db.GetIns().Create(&viewer).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.AdminNativeActivities(f.ctx, &model.AdminPrincipal{UserID: viewer.ID, Username: viewer.Username, Role: viewer.Role}); err != nil {
		t.Fatalf("viewer read rejected: %v", err)
	}
}

func adminNativeNewPlayer(t *testing.T, f *p1Fixture, label string, vip int32, diamond int64) int64 {
	t.Helper()
	account, err := LoginAccount(f.ctx, label+"-"+fmt.Sprint(time.Now().UnixNano()), "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(f.ctx, account.Id, "精彩活动回归", "hero_043"); err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(f.ctx, account.Id, func(p *pb.Player, _ *model.GameState) error {
		p.Vip = proto.Int32(vip)
		p.Diamond = proto.Int64(diamond)
		p.Level = proto.Int32(200)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return account.Id
}

func adminNativeConfirmedOrder(t *testing.T, f *p1Fixture, playerID int64, productID string, cents int64) {
	t.Helper()
	var product db.PaymentProduct
	if err := db.GetIns().First(&product, "id = ?", productID).Error; err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(product)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	order := db.PaymentOrder{ID: fmt.Sprintf("native-order-%d-%d", playerID, time.Now().UnixNano()), PlayerID: playerID, RequestKey: time.Now().UnixNano() + playerID, RequestHash: "native-activity-fixture", ProductID: product.ID, Snapshot: string(raw), Status: constUtil.P3PaymentConfirmed, CreatedAt: now, ExpiresAt: now + 3600000, ConfirmedAt: now, Cents: cents, Credited: cents, Source: "fixture"}
	if err := db.GetIns().Create(&order).Error; err != nil {
		t.Fatal(err)
	}
}

// TestAdminNativeFundUpdatesUnclaimedConsumers 验证基金真实表更新、领取键稳定和空奖励拒绝启用。
func TestAdminNativeFundUpdatesUnclaimedConsumers(t *testing.T) {
	f, operator := adminNativeFixture(t, false)
	item := adminNativeFind(t, f, operator, "fund")
	if item.ID != "fund:open_server_fund" || item.Name != "开服基金" || item.Frozen {
		t.Fatalf("unexpected fund dto %+v", item)
	}
	now := time.Now().UnixMilli()
	item.Form.StartAt = now - 3600000
	item.Form.EndAt = now + 86400000
	item.Form.ClaimEndAt = now + 172800000
	item.Form.MinVip = 0
	item.Form.DiamondCost = 100
	foundFund, foundBenefit := false, false
	for i := range item.Form.Milestones {
		milestone := &item.Form.Milestones[i]
		switch {
		case strings.HasPrefix(milestone.Key, "openServerFund:"):
			milestone.Rewards = []model.AdminResourceAmount{{ResourceID: "coin", Amount: 555}}
			foundFund = true
		case strings.HasPrefix(milestone.Key, "everyoneBenefit:"):
			milestone.Rewards = []model.AdminResourceAmount{{ResourceID: "coin", Amount: 777}}
			if !foundBenefit {
				milestone.Goal = 1
				foundBenefit = true
			}
		}
	}
	if !foundFund || !foundBenefit {
		t.Fatal("fund milestone identities missing")
	}
	saved, err := f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: item.Version, Enabled: true, Alias: "基金自检", Form: item.Form})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Version == item.Version || saved.Form.Milestones[0].Key == "" {
		t.Fatal("fund save did not rotate version or preserve key")
	}
	// 原等级档目标不可改，即使版本正确也必须在服务端拒绝。
	changed := saved.Form
	changed.Milestones[0].Goal++
	if _, err = f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: saved.Version, Enabled: true, Form: changed}); !errors.Is(err, constUtil.ErrAdminInput) {
		t.Fatalf("fund threshold edit accepted: %v", err)
	}
	// 空奖励不能通过开启放行。
	empty := saved.Form
	empty.Milestones[0].Rewards = nil
	if _, err = f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: saved.Version, Enabled: true, Form: empty}); !errors.Is(err, constUtil.ErrAdminInput) {
		t.Fatalf("empty fund reward enabled: %v", err)
	}
	// 仅写入一份真实购买事实，不伪造支付或奖励。
	if err = db.GetIns().Create(&db.ActivityState{ActivityID: "open_server_fund", PlayerID: f.id, Bought: true, Claims: []byte("{}")}).Error; err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Level = proto.Int32(200)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	info, err := f.s.openServerFund(f.ctx, f.id, &pb.CSOpenServerFundInfo{})
	if err != nil {
		t.Fatal(err)
	}
	var fundItem, benefitItem *pb.ActivityItem
	for _, row := range info[0].(*pb.SCOpenServerFundInfo).GetOpenServerFundList() {
		if fundItem == nil {
			fundItem = row
		}
	}
	for _, row := range info[0].(*pb.SCOpenServerFundInfo).GetEveryoneBenefitList() {
		if benefitItem == nil {
			benefitItem = row
		}
	}
	if fundItem == nil || benefitItem == nil || fundItem.GetRewardList()[0].GetNum() != 555 || benefitItem.GetRewardList()[0].GetNum() != 777 {
		t.Fatalf("new fund configuration not consumed: %+v %+v", fundItem, benefitItem)
	}
	before, _ := f.profile(t)
	claimed, err := f.s.openServerFund(f.ctx, f.id, &pb.CSOpenServerFundReward{RewardType: proto.String("everyoneBenefit"), Index: proto.Int32(benefitItem.GetIndex())})
	if err != nil {
		t.Fatal(err)
	}
	if claimed[0].(*pb.SCOpenServerFundReward).GetGet() == nil {
		t.Fatal("fund benefit gain missing")
	}
	after, _ := f.profile(t)
	if after.GetCoin() != before.GetCoin()+777 {
		t.Fatalf("benefit reward not granted: before=%d after=%d", before.GetCoin(), after.GetCoin())
	}
	beforeFund, _ := f.profile(t)
	if _, err = f.s.openServerFund(f.ctx, f.id, &pb.CSOpenServerFundReward{RewardType: proto.String("openServerFund"), Index: proto.Int32(fundItem.GetIndex())}); err != nil {
		t.Fatal(err)
	}
	afterFund, _ := f.profile(t)
	if afterFund.GetCoin() != beforeFund.GetCoin()+555 {
		t.Fatalf("paid fund reward not granted: before=%d after=%d", beforeFund.GetCoin(), afterFund.GetCoin())
	}
	if _, err = f.s.openServerFund(f.ctx, f.id, &pb.CSOpenServerFundReward{RewardType: proto.String("everyoneBenefit"), Index: proto.Int32(benefitItem.GetIndex())}); err == nil {
		t.Fatal("fund benefit regranted")
	}
	var state db.ActivityState
	if err = db.GetIns().First(&state, "activity_id=? AND player_id=?", "open_server_fund", f.id).Error; err != nil {
		t.Fatal(err)
	}
	var claims map[string]bool
	if err = json.Unmarshal(state.Claims, &claims); err != nil {
		t.Fatal(err)
	}
	var selected db.ActivityReward
	if err = db.GetIns().Where("activity_id = ? AND reward_type = ?", "open_server_fund", "everyoneBenefit").Order("threshold ASC").First(&selected).Error; err != nil {
		t.Fatal(err)
	}
	stableKey := "everyoneBenefit:" + fmt.Sprint(selected.Threshold)
	if !claims[stableKey] {
		t.Fatalf("stable threshold claim key missing: %v", claims)
	}
}

// TestAdminNativeFirstRechargeFreezesObservedPlayers 验证首充旧观察快照保留，新角色立即使用新配置。
func TestAdminNativeFirstRechargeFreezesObservedPlayers(t *testing.T) {
	f, operator := adminNativeFixture(t, true)
	item := adminNativeFind(t, f, operator, "payment-first")
	if item.Frozen {
		t.Fatal("payment activity must remain live for unobserved players")
	}
	// 先观察一次，冻结该角色旧规则。
	if _, err := f.s.paymentActivity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(strings.TrimPrefix(item.ID, "payment:"))}, time.Now()); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	item.Form.StartAt = now - 3600000
	item.Form.EndAt = now + 86400000
	item.Form.ClaimEndAt = now + 172800000
	item.Form.Threshold = 1
	item.Form.Rewards = []model.AdminResourceAmount{{ResourceID: "diamond", Amount: 25}}
	saved, err := f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: item.Version, Enabled: true, Form: item.Form})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Frozen {
		t.Fatal("payment activity wrongly reported frozen")
	}
	old, err := f.s.paymentActivity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(strings.TrimPrefix(item.ID, "payment:"))}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if old[0].(*pb.SCActivityInfo).GetItemList()[0].GetRewardList()[0].GetId() == "diamond" {
		t.Fatal("observed player snapshot replaced by new configuration")
	}
	playerID := adminNativeNewPlayer(t, f, "native-payment", 0, 0)
	var product db.PaymentProduct
	if err := db.GetIns().Where("kind = ? AND enabled = ?", "once", true).First(&product).Error; err != nil {
		t.Fatal(err)
	}
	adminNativeConfirmedOrder(t, f, playerID, product.ID, 600)
	fresh, err := f.s.paymentActivity(f.ctx, playerID, &pb.CSActivityInfo{ActivityId: proto.String(strings.TrimPrefix(item.ID, "payment:"))}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	reward := fresh[0].(*pb.SCActivityInfo).GetItemList()[0]
	if reward.GetStatus() != 1 || reward.GetRewardList()[0].GetId() != "diamond" || reward.GetRewardList()[0].GetNum() != 25 {
		t.Fatalf("new player did not receive live rule: %+v", reward)
	}
	before, _ := func() (*pb.Player, *model.GameState) {
		var row db.Profile
		if err := db.GetIns().First(&row, playerID).Error; err != nil {
			t.Fatal(err)
		}
		p, g, err := decodeProfile(&row)
		if err != nil {
			t.Fatal(err)
		}
		return p, g
	}()
	claim, err := f.s.paymentActivity(f.ctx, playerID, &pb.CSActivityReward{ActivityId: proto.String(strings.TrimPrefix(item.ID, "payment:")), Index: proto.Int32(1)}, time.Now())
	if err != nil || claim[0].(*pb.SCActivityReward).GetGet() == nil {
		t.Fatal("first recharge claim failed", err)
	}
	var row db.Profile
	if err = db.GetIns().First(&row, playerID).Error; err != nil {
		t.Fatal(err)
	}
	after, _, err := decodeProfile(&row)
	if err != nil {
		t.Fatal(err)
	}
	if after.GetDiamond() != before.GetDiamond()+25 {
		t.Fatalf("first recharge new reward not granted: before=%d after=%d", before.GetDiamond(), after.GetDiamond())
	}
}

// TestAdminNativeVIPPolicyDrivesRealMembership 验证保存档位同时驱动展示、领取、扣费和真实消费事件。
func TestAdminNativeVIPPolicyDrivesRealMembership(t *testing.T) {
	f, operator := adminNativeFixture(t, true)
	item := adminNativeFind(t, f, operator, "vip-benefits")
	if item.Name != "会员福利" || len(item.Form.VipTiers) != 13 {
		t.Fatalf("unexpected vip policy %+v", item)
	}
	form := item.Form
	for i := range form.VipTiers {
		if form.VipTiers[i].Vip == 2 {
			form.VipTiers[i].DailyRewards = []model.AdminResourceAmount{{ResourceID: "coin", Amount: 7777}, {ResourceID: "item_005", Amount: 1}}
			form.VipTiers[i].WeekCost = model.AdminResourceAmount{ResourceID: "diamond", Amount: 150}
			form.VipTiers[i].WeekRewards = []model.AdminResourceAmount{{ResourceID: "item_006", Amount: 2}}
		}
	}
	saved, err := f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: item.Version, Enabled: true, Form: form})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Version == item.Version {
		t.Fatal("vip policy version did not rotate")
	}
	playerID := adminNativeNewPlayer(t, f, "native-vip", 2, 1000)
	var product db.PaymentProduct
	if err := db.GetIns().Where("kind = ?", "month").First(&product).Error; err != nil {
		t.Fatal(err)
	}
	adminNativeConfirmedOrder(t, f, playerID, product.ID, 3000)
	info, err := f.s.membership(f.ctx, playerID, &pb.CSGetVipBoonInfo{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	view := info[0].(*pb.SCGetVipBoonInfo)
	if view.GetDaily()[0].GetReward()[0].GetId() != "coin" || view.GetDaily()[0].GetReward()[0].GetNum() != 7777 || view.GetDaily()[0].GetReward()[1].GetId() != "item_005" {
		t.Fatalf("daily configured reward not projected: %+v", view.GetDaily())
	}
	if view.GetWeek()[0].GetCostResource()[0].GetId() != "diamond" || view.GetWeek()[0].GetCostResource()[0].GetNum() != 150 || view.GetWeek()[0].GetResource[0].GetId() != "item_006" || view.GetWeek()[0].GetResource[0].GetNum() != 2 {
		t.Fatalf("weekly configured reward not projected: %+v", view.GetWeek())
	}
	daily, err := f.s.membership(f.ctx, playerID, &pb.CSGetVipDailyBoonReward{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	dailyItem := int32(0)
	for _, item := range daily[0].(*pb.SCGetVipDailyBoonReward).GetGet().GetSimps() {
		if item.GetId() == "item_005" {
			dailyItem += item.GetNum()
		}
	}
	if dailyItem != 1 {
		t.Fatalf("daily claim did not use configured resources: %+v", daily[0])
	}
	week, err := f.s.membership(f.ctx, playerID, &pb.CSGetVipWeekBoonReward{Index: proto.Int32(1), CostResources: view.GetWeek()[0].GetCostResource(), GetResources: view.GetWeek()[0].GetResource}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	weekItem := int32(0)
	for _, item := range week[0].(*pb.SCGetVipWeekBoonReward).GetGet().GetSimps() {
		if item.GetId() == "item_006" {
			weekItem += item.GetNum()
		}
	}
	if weekItem != 2 {
		t.Fatalf("weekly claim did not use configured resources: %+v", week[0])
	}
	var row db.Profile
	if err = db.GetIns().First(&row, playerID).Error; err != nil {
		t.Fatal(err)
	}
	profile, _, err := decodeProfile(&row)
	if err != nil {
		t.Fatal(err)
	}
	if profile.GetDiamond() != 850 {
		t.Fatalf("weekly configured cost not deducted: %d", profile.GetDiamond())
	}
	var spent db.EventFact
	if err = db.GetIns().First(&spent, "player_id = ? AND kind = ?", playerID, constUtil.P1EventDiamondSpent).Error; err != nil {
		t.Fatal(err)
	}
	if spent.Amount != 150 {
		t.Fatalf("spend event did not use configured cost: %d", spent.Amount)
	}
	// 关闭策略后新角色显示不可领并拒绝领取，不改变真实购买资格。
	disabled := saved.Form
	if _, err = f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: saved.Version, Enabled: false, Form: disabled}); err != nil {
		t.Fatal(err)
	}
	closedID := adminNativeNewPlayer(t, f, "native-vip-closed", 2, 1000)
	adminNativeConfirmedOrder(t, f, closedID, product.ID, 3000)
	closed, err := f.s.membership(f.ctx, closedID, &pb.CSGetVipBoonInfo{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if closed[0].(*pb.SCGetVipBoonInfo).GetDaily()[0].GetStatus() != 0 || closed[0].(*pb.SCGetVipBoonInfo).GetWeek()[0].GetStatus() != 0 {
		t.Fatal("disabled vip policy still claimable")
	}
	if _, err = f.s.membership(f.ctx, closedID, &pb.CSGetVipDailyBoonReward{}, time.Now()); err == nil {
		t.Fatal("disabled vip policy granted daily reward")
	}
}

// TestAdminNativeFestivalWritesChildren 验证节日父子目录、固定子标题、真实消费和子冻结保护。
func TestAdminNativeFestivalWritesChildren(t *testing.T) {
	f, operator := adminNativeFixture(t, false)
	if err := f.s.seedP15Seasonal(db.GetIns().WithContext(f.ctx), time.Now()); err != nil {
		t.Fatal(err)
	}
	item := adminNativeFind(t, f, operator, "festival")
	if len(item.Form.Children) != 2 {
		t.Fatalf("festival children missing: %+v", item.Form.Children)
	}
	now := time.Now().UnixMilli()
	item.Form.StartAt = now - 3600000
	item.Form.EndAt = now + 86400000
	item.Form.ClaimEndAt = now + 172800000
	var periodChild, commerceChild string
	for i := range item.Form.Children {
		child := &item.Form.Children[i]
		child.Form.StartAt = item.Form.StartAt
		child.Form.EndAt = item.Form.EndAt
		child.Form.ClaimEndAt = item.Form.ClaimEndAt
		if child.Domain == "period" {
			periodChild = child.ID
			child.Form.Name = "登录奖励"
			child.Form.Milestones = []model.AdminActivityMilestone{{Goal: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "item_124", Amount: 16}}}}
		} else {
			commerceChild = child.ID
			child.Form.Name = "限时兑换"
			child.Form.Products = []model.AdminActivityProduct{{ID: "native_p15_product", Name: "专精石补给", Index: 1, Rewards: []model.AdminResourceAmount{{ResourceID: "item_470", Amount: 12}}, Cost: model.AdminResourceAmount{ResourceID: "coin", Amount: 200000}, DailyLimit: 3, TotalLimit: 30, Stock: -1, MinLevel: 1, DiscountBps: 10000, Weight: 1}}
		}
	}
	saved, err := f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: item.Version, Enabled: true, Form: item.Form})
	if err != nil {
		t.Fatal(err)
	}
	if !saved.Enabled || len(saved.Form.Children) != 2 {
		t.Fatalf("festival not saved: %+v", saved)
	}
	var bundle db.P15SeasonalBundle
	if err = db.GetIns().First(&bundle, "id = ?", strings.TrimPrefix(item.ID, "p15:")).Error; err != nil {
		t.Fatal(err)
	}
	var children []p15SeasonalChild
	if err = json.Unmarshal(bundle.Children, &children); err != nil {
		t.Fatal(err)
	}
	if children[0].Title != "登录奖励" || children[1].Title != "限时兑换" {
		t.Fatalf("child titles not fixed: %+v", children)
	}
	// 真实P1消费者读取同一子实例和修改后的奖励。
	periodInfo, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(periodChild)})
	if err != nil {
		t.Fatal(err)
	}
	periodItem := periodInfo[0].(*pb.SCActivityInfo).GetItemList()[0]
	if periodItem.GetRewardList()[0].GetId() != "item_124" || periodItem.GetRewardList()[0].GetNum() != 16 {
		t.Fatalf("festival period child not consumed: %+v", periodItem)
	}
	// 真实兑换消费者读取同一商品并完成实际扣费发奖。
	if err = updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Coin = proto.Int64(300000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	shopInfo := &pb.CSActivityShopInfo{ActivityId: proto.String(commerceChild)}
	if _, err = f.s.p4Commerce(f.ctx, f.id, shopInfo, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	buy := &pb.CSGetActivityShopReward{ActivityId: proto.String(commerceChild), Index: proto.Int32(1), CostResources: []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(200000)}}, GetResources: []*pb.BaseResource{{Id: proto.String("item_470"), Num: proto.Int32(12)}}}
	if _, err = f.s.p4Commerce(f.ctx, f.id, buy, 812345, time.Now()); err != nil {
		t.Fatal(err)
	}
	paid, paidGame := f.profile(t)
	if paid.GetCoin() != 100000 || simpleOwned(paid, paidGame, "item_470") != 12 {
		t.Fatalf("festival commerce child not consumed: coin=%d item=%d", paid.GetCoin(), simpleOwned(paid, paidGame, "item_470"))
	}
	// 子规则冻结后，父时间和子奖励都不能再改。
	var liveChild db.PeriodInstance
	if err = db.GetIns().First(&liveChild, "id = ?", periodChild).Error; err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(liveChild)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.PeriodInstance{}).Where("id = ?", liveChild.ID).Update("frozen", raw).Error; err != nil {
		t.Fatal(err)
	}
	frozen := adminNativeFind(t, f, operator, "festival")
	if !frozen.Frozen {
		t.Fatal("child frozen state not propagated to parent dto")
	}
	changed := frozen.Form
	changed.StartAt--
	if _, err = f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: frozen.Version, Enabled: true, Form: changed}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatalf("frozen festival rule change accepted: %v", err)
	}
}

// TestAdminNativeCelebrationConsumerAndFreeze 验证庆典按既有P1消费者保存并在冻结后拒绝规则变化。
func TestAdminNativeCelebrationConsumerAndFreeze(t *testing.T) {
	f, operator := adminNativeFixture(t, false)
	if err := f.s.seedP15Seasonal(db.GetIns().WithContext(f.ctx), time.Now()); err != nil {
		t.Fatal(err)
	}
	item := adminNativeFind(t, f, operator, "celebration")
	if item.Name != "庆典活动" || len(item.Form.Milestones) == 0 {
		t.Fatalf("unexpected celebration dto %+v", item)
	}
	now := time.Now().UnixMilli()
	item.Form.StartAt = now - 3600000
	item.Form.EndAt = now + 86400000
	item.Form.ClaimEndAt = now + 172800000
	item.Form.Milestones[0].Rewards = []model.AdminResourceAmount{{ResourceID: "item_047", Amount: 3}}
	saved, err := f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: item.Version, Enabled: true, Form: item.Form})
	if err != nil {
		t.Fatal(err)
	}
	info, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(strings.TrimPrefix(item.ID, "celebration:"))})
	if err != nil {
		t.Fatal(err)
	}
	reward := info[0].(*pb.SCActivityInfo).GetItemList()[0].GetRewardList()[0]
	if reward.GetId() != "item_047" || reward.GetNum() != 3 {
		t.Fatalf("celebration consumer did not use saved reward: %+v", reward)
	}
	var live db.PeriodInstance
	if err = db.GetIns().First(&live, "id = ?", strings.TrimPrefix(item.ID, "celebration:")).Error; err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(live)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.PeriodInstance{}).Where("id = ?", live.ID).Update("frozen", raw).Error; err != nil {
		t.Fatal(err)
	}
	frozen := adminNativeFind(t, f, operator, "celebration")
	if !frozen.Frozen || frozen.Form.StartAt != saved.Form.StartAt {
		t.Fatal("celebration freeze not reflected")
	}
	changed := frozen.Form
	changed.StartAt--
	if _, err = f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: frozen.Version, Enabled: true, Form: changed}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatalf("frozen celebration change accepted: %v", err)
	}
}

// TestAdminNativeFirstDoubleOnlyToggles 验证首充双倍只有启停与别名，来源和终身历史字段保持。
func TestAdminNativeFirstDoubleOnlyToggles(t *testing.T) {
	f, operator := adminNativeFixture(t, false)
	if err := f.s.InitializeFirstDouble(f.ctx); err != nil {
		t.Fatal(err)
	}
	item := adminNativeFind(t, f, operator, "first-double")
	if !item.Frozen || len(item.Form.Children) != 0 || item.Form.StartAt != 0 {
		t.Fatalf("first double must expose frozen empty form: %+v", item)
	}
	var before db.FirstDoublePolicy
	if err := db.GetIns().First(&before, "id = ?", 1).Error; err != nil {
		t.Fatal(err)
	}
	saved, err := f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: item.Version, Enabled: true, Alias: "首充别名", Form: item.Form})
	if err != nil {
		t.Fatal(err)
	}
	if !saved.Enabled || saved.Alias != "首充别名" {
		t.Fatalf("first double toggle failed: %+v", saved)
	}
	var after db.FirstDoublePolicy
	if err = db.GetIns().First(&after, "id = ?", 1).Error; err != nil {
		t.Fatal(err)
	}
	if after.Version != constUtil.FirstDoubleVersion || after.Source != constUtil.FirstDoubleSource || !after.Enabled {
		t.Fatal("first double policy source or version changed")
	}
	changed := saved.Form
	changed.StartAt = 1
	if _, err = f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: saved.Version, Enabled: true, Form: changed}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatalf("first double form edit accepted: %v", err)
	}
	if _, err = f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: item.Version, Enabled: false, Form: item.Form}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatalf("first double stale version accepted: %v", err)
	}
}

// TestAdminNativeCardUpdatesOnlyEconomicFields 验证卡类只改经济字段且真实领取使用新每日钻石。
func TestAdminNativeCardUpdatesOnlyEconomicFields(t *testing.T) {
	f, operator := adminNativeFixture(t, true)
	item := adminNativeFind(t, f, operator, "month-card")
	productID := strings.TrimPrefix(item.ID, "product:")
	// 先冻结保存前的身份字段，保存后逐项核对只有经济字段变化。
	var originalProduct db.PaymentProduct
	if err := db.GetIns().First(&originalProduct, "id = ?", productID).Error; err != nil {
		t.Fatal(err)
	}
	item.Form.Cents = 3100
	item.Form.Charge = 310
	item.Form.DailyDiamond = 333
	item.Form.Rewards = []model.AdminResourceAmount{{ResourceID: "item_005", Amount: 2}}
	saved, err := f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: item.Version, Enabled: true, Form: item.Form})
	if err != nil {
		t.Fatal(err)
	}
	var product db.PaymentProduct
	if err = db.GetIns().First(&product, "id = ?", productID).Error; err != nil {
		t.Fatal(err)
	}
	if product.Cents != 3100 || product.Charge != 310 || product.Free != 333 || product.Reward != "item_005,2" || !product.Enabled {
		t.Fatalf("card economic fields not saved: %+v", product)
	}
	if product.ID != originalProduct.ID || product.Channels != originalProduct.Channels || product.Shop != originalProduct.Shop || product.Currency != originalProduct.Currency || product.Kind != originalProduct.Kind || product.FirstDouble != originalProduct.FirstDouble {
		t.Fatal("card immutable identity fields changed")
	}
	if saved.Kind != "month-card" || saved.Frozen {
		t.Fatal("card dto kind changed")
	}
	// 新订单快照读取保存后的每日钻石，真实领取按该快照发放。
	raw, err := json.Marshal(product)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	order := db.PaymentOrder{ID: "native-card-order", PlayerID: f.id, RequestKey: now, RequestHash: "native-card-fixture", ProductID: product.ID, Snapshot: string(raw), Status: constUtil.P3PaymentConfirmed, CreatedAt: now, ExpiresAt: now + 3600000, ConfirmedAt: now, Cents: product.Cents, Credited: product.Charge, Source: "fixture"}
	if err = db.GetIns().Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.MonthCardEndTime = proto.Int64(now + constUtil.P3MonthCardDurationMS)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before, _ := f.profile(t)
	if _, err = f.s.membership(f.ctx, f.id, &pb.CSClaimCard{CardType: proto.String("month")}, time.Now()); err != nil {
		t.Fatal(err)
	}
	after, _ := f.profile(t)
	if after.GetDiamond() != before.GetDiamond()+333 {
		t.Fatalf("card daily diamond not used: before=%d after=%d", before.GetDiamond(), after.GetDiamond())
	}
}

// TestAdminNativeRecruitBuildsOriginalRotation 验证橙卡招募奖池构造和冻结保护。
func TestAdminNativeRecruitBuildsOriginalRotation(t *testing.T) {
	f, operator := adminNativeFixture(t, false)
	// 新服在任何玩家招募之前就能从后台配置已有的关闭轮换。
	startup := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	if err := InitializeAdminActivityModes(startup); err != nil {
		t.Fatal(err)
	}
	item := adminNativeFind(t, f, operator, "recruit-rotation")
	if item.Name != "橙卡招募" || len(item.Form.Prizes) == 0 || item.Enabled {
		t.Fatalf("unexpected rotation dto %+v", item)
	}
	form := item.Form
	form.Prizes[0].Weight++
	saved, err := f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: item.Version, Enabled: item.Enabled, Form: form})
	if err != nil {
		t.Fatal(err)
	}
	var live db.P4RecruitRotation
	rotationID := strings.TrimPrefix(item.ID, "recruit:")
	if err := db.GetIns().First(&live, "id = ?", rotationID).Error; err != nil {
		t.Fatal(err)
	}
	var rewards []db.GameplayPoolReward
	if err := json.Unmarshal(live.Rewards, &rewards); err != nil {
		t.Fatal(err)
	}
	for _, reward := range rewards {
		if reward.PoolID != "recruit_2" || reward.Amount != 1 || reward.Stock != 0 {
			t.Fatalf("rotation contract changed: %+v", reward)
		}
		hero := f.s.table("Hero", "id", reward.ResourceID)
		if hero == nil || (hero["quality"] != "13" && hero["quality"] != "18" && hero["quality"] != "20") || hero["heroClass"] != "0" || reward.Guaranteed != (hero["quality"] == "20") {
			t.Fatalf("rotation hero qualification changed: %+v", reward)
		}
	}
	invalid := form
	invalid.Prizes[0].ResourceID = "item_005"
	if _, err := f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: saved.Version, Enabled: saved.Enabled, Form: invalid}); !errors.Is(err, constUtil.ErrAdminInput) {
		t.Fatalf("invalid rotation hero accepted: %v", err)
	}
	frozenRaw := live.Rewards
	if err := db.GetIns().Model(&db.P4RecruitRotation{}).Where("id = ?", live.ID).Update("frozen", frozenRaw).Error; err != nil {
		t.Fatal(err)
	}
	frozen := adminNativeFind(t, f, operator, "recruit-rotation")
	if !frozen.Frozen {
		t.Fatal("frozen rotation not reported")
	}
	// 深拷贝奖池切片，避免改动候选表单时连带污染用于对照的原始表单。
	changed := frozen.Form
	changed.Prizes = append([]model.AdminActivityPrize(nil), frozen.Form.Prizes...)
	changed.Prizes[0].Weight++
	if _, err := f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: frozen.Version, Enabled: true, Form: changed}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatalf("frozen rotation change accepted: %v", err)
	}
	if _, err := f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: frozen.Version, Enabled: true, Form: frozen.Form}); err != nil {
		t.Fatalf("frozen rotation toggle/enable rejected: %v", err)
	}
}

// TestAdminNativeFrozenCopyOnlyToggles 验证冻结拒改、版本冲突、启停与别名不进入游戏协议。
func TestAdminNativeFrozenCopyOnlyToggles(t *testing.T) {
	f, operator := adminNativeFixture(t, false)
	item := adminNativeFind(t, f, operator, "copy-benefits")
	if item.Frozen {
		t.Fatal("seeded copy benefit unexpectedly frozen")
	}
	item.Form.DropFactor = 3
	saved, err := f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: item.Version, Enabled: item.Enabled, Alias: "福利别名校验", Form: item.Form})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Alias != "福利别名校验" {
		t.Fatal("unfrozen alias not returned")
	}
	var live db.P4CopyBenefitRule
	if err = db.GetIns().First(&live, "id = ?", strings.TrimPrefix(item.ID, "copy:")).Error; err != nil {
		t.Fatal(err)
	}
	if live.DropFactor != 3 {
		t.Fatal("unfrozen copy rule not written to original table")
	}
	if _, err = f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: item.Version, Enabled: item.Enabled, Form: item.Form}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatalf("stale version accepted: %v", err)
	}
	// 其他窗口关闭后冻结当前窗口，确保启停校验不受重叠窗口影响。
	if err = db.GetIns().Model(&db.P4CopyBenefitRule{}).Where("id <> ?", live.ID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.P4CopyBenefitRule{}).Where("id = ?", live.ID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().First(&live, "id = ?", live.ID).Error; err != nil {
		t.Fatal(err)
	}
	frozenRaw, err := json.Marshal(live)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.P4CopyBenefitRule{}).Where("id = ?", live.ID).Update("frozen", frozenRaw).Error; err != nil {
		t.Fatal(err)
	}
	frozen := adminNativeFind(t, f, operator, "copy-benefits")
	if !frozen.Frozen {
		t.Fatal("frozen copy rule not reported")
	}
	changed := frozen.Form
	changed.DropFactor = 4
	if _, err = f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: frozen.Version, Enabled: true, Form: changed}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatalf("frozen rule change accepted: %v", err)
	}
	info, err := f.s.P4CopyBenefitsInfo(f.ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(info[0].(*pb.SCActivityInfo).GetInformation(), "福利别名校验") {
		t.Fatal("admin alias leaked into game protocol")
	}
	off, err := f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: frozen.Version, Enabled: false, Alias: "只改别名", Form: frozen.Form})
	if err != nil {
		t.Fatal(err)
	}
	if off.Enabled || off.Alias != "只改别名" {
		t.Fatalf("frozen toggle or alias failed: %+v", off)
	}
	var meta db.AdminActivityPresentation
	if err = db.GetIns().First(&meta, "activity_id = ?", item.ID).Error; err != nil {
		t.Fatal(err)
	}
	if meta.Alias != "只改别名" {
		t.Fatal("alias not persisted in presentation")
	}
	if _, err = f.s.AdminSaveNativeActivity(f.ctx, operator, item.ID, model.AdminNativeActivitySaveRequest{Version: frozen.Version, Enabled: true, Form: frozen.Form}); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatalf("used version accepted after toggle: %v", err)
	}
}
