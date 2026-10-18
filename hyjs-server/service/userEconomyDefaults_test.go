package service

import (
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestUserEconomyUnconfiguredLoginRewardInfoHiddenAndClaimClosed(t *testing.T) {
	f := p4Setup(t)
	player, game := f.profile(t)
	before, _ := json.Marshal([]any{player, game})
	out, handled, err := f.s.DispatchP4(f.ctx, f.id, &pb.CSLoginRewardInfo{}, 0)
	if err != nil || !handled || len(out) != 1 {
		t.Fatal("empty login reward query", err)
	}
	view := out[0].(*pb.SCLoginRewardInfo)
	if view.GetIsShow() != 0 || view.GetHasReward() != 0 || len(view.GetItems()) != 0 {
		t.Fatal("unconfigured login rewards advertised", view)
	}
	if out, _, err := f.s.DispatchP4(f.ctx, f.id, &pb.CSGetLoginReward{}, 736001); err == nil || out != nil {
		t.Fatal("hidden query enabled a claim")
	}
	player, game = f.profile(t)
	after, _ := json.Marshal([]any{player, game})
	if string(before) != string(after) {
		t.Fatal("empty query or rejected claim changed profile")
	}
	if err := f.s.seedP4Welfare(db.GetIns(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P4WelfareRule{}).Where("kind=?", constUtil.P4WelfareLogin).Updates(map[string]any{"enabled": true, "rules": []byte("invalid-json")}).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.s.DispatchP4(f.ctx, f.id, &pb.CSLoginRewardInfo{}, 0); err == nil {
		t.Fatal("invalid live rules hidden as unconfigured")
	}
}

func TestUserEconomyUnconfiguredShopLoginCatalogueAndPurchaseClosed(t *testing.T) {
	f := p4Setup(t)
	beforePlayer, beforeGame := f.profile(t)
	before, _ := json.Marshal([]any{beforePlayer, beforeGame})
	queries := []proto.Message{&pb.CSShopProductList{ShopType: proto.Int32(1)}, &pb.CSShopProductList{ShopType: proto.Int32(4)}, &pb.CSShopProductList{ShopType: proto.Int32(5)}, &pb.CSShopProductList{ShopType: proto.Int32(7)}}
	for _, query := range queries {
		var out []proto.Message
		var err error
		if query.(*pb.CSShopProductList).GetShopType() == 1 {
			out, err = f.s.shopProducts(f.ctx, f.id, query.(*pb.CSShopProductList))
		} else {
			out, _, err = f.s.DispatchP4(f.ctx, f.id, query, 735001)
		}
		if err != nil || len(out) != 1 {
			t.Fatal("empty catalogue blocked original query", query, err)
		}
		for _, item := range out[0].(*pb.SCShopProductList).GetShopItemList() {
			if item.GetId() != "itemShopId_12" {
				t.Fatal("retired invented product returned", item.GetId())
			}
		}
	}
	if out, _, err := f.s.DispatchP4(f.ctx, f.id, &pb.CSShopBuy{ShopType: proto.Int32(1), ShopItemId: proto.String("itemShopId_1"), BuyCount: proto.Int32(1)}, 735002); err == nil || out != nil {
		t.Fatal("empty query enabled a purchase")
	}
	afterPlayer, afterGame := f.profile(t)
	after, _ := json.Marshal([]any{afterPlayer, afterGame})
	if string(before) != string(after) {
		t.Fatal("empty queries or rejected purchase changed profile")
	}
	// Only the explicit not-open condition becomes empty; corrupt live rules still fail.
	if err := f.s.seedP4Commerce(db.GetIns(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P4CommerceRule{}).Where("kind=?", "ordinary").Updates(map[string]any{"enabled": true, "rules": []byte("invalid-json")}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.shopProducts(f.ctx, f.id, &pb.CSShopProductList{ShopType: proto.Int32(1)}); err == nil {
		t.Fatal("corrupt live configuration hidden as empty")
	}
}

// TestUserEconomyUnconfiguredSourcesStayClosed 检查新期不会把用户撤掉的自设投放重新开启。
func TestUserEconomyUnconfiguredSourcesStayClosed(t *testing.T) {
	f := p4Setup(t)
	now := time.Now()
	for _, at := range []time.Time{now, now.AddDate(0, 1, 0)} {
		for _, seed := range []func(time.Time) error{
			func(at time.Time) error { return f.s.seedP4Welfare(db.GetIns(), at) },
			func(at time.Time) error { return f.s.seedP4Commerce(db.GetIns(), at) },
			func(at time.Time) error { return f.s.seedP4Periods(db.GetIns(), at) },
			func(at time.Time) error { return f.s.seedP4Rotations(db.GetIns(), at) },
		} {
			if err := seed(at); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, table := range []any{&db.P4WelfareRule{}, &db.P4CommerceRule{}, &db.P4RecruitRotation{}} {
		var count int64
		if err := db.GetIns().Model(table).Where("enabled = ?", true).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("unconfigured source reopened %T count=%d err=%v", table, count, err)
		}
	}
	// 停止自设限时轮换后，原常驻普通/强者目录仍必须能打开。
	out, err := f.s.recruitInfo(f.ctx, f.id, &pb.CSWarriorRecruitGet{CostType: []int32{1, 2}})
	if err != nil {
		t.Fatal("disabled rotations blocked normal recruit", err)
	}
	entries := out[0].(*pb.SCWarriorRecruitGet).GetRecruit()
	if len(entries) != 2 {
		t.Fatalf("normal recruitment missing %v", entries)
	}
	for _, entry := range entries {
		if len(entry.GetRewardList()) == 0 || entry.GetChangeTime() != 0 {
			t.Fatal("normal recruit did not fall back to persistent pool", entry)
		}
	}
	// 后续用户明确配置和启用的规则，重启/重seed不能重新关掉或覆盖。
	var rule db.P4CommerceRule
	if err := db.GetIns().Order("id").First(&rule).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&rule).Updates(map[string]any{"enabled": true, "source": "user-configured-preserve"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.seedP4Commerce(db.GetIns(), now); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&rule, "id = ?", rule.ID).Error; err != nil || !rule.Enabled || rule.Source != "user-configured-preserve" {
		t.Fatal("explicit future operations rule was overwritten", err)
	}
}
