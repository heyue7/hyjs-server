package service

import (
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

func TestActivity77ShopsApplyAndWeeklyPurchase(t *testing.T) {
	f, actor := reuseFixture(t, false)
	if err := f.s.InitializeClientUpdates(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Save(&db.ClientUpdatePolicy{ID: 1, Maintenance: false}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyActivity77Shops(f.ctx, actor); err == nil {
		t.Fatal("non-maintenance apply accepted")
	}
	if err := db.GetIns().Save(&db.ClientUpdatePolicy{ID: 1, Maintenance: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeAdminShopAvailability(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := initializeGGOAccountWeekly(db.GetIns()); err != nil {
		t.Fatal(err)
	}
	p4Fund(t, f)
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error { p.Score = proto.Int32(100000); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().AutoMigrate(&db.GameAccountRole{}); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.Profile{}).Where("id=?", f.id).Update("account_id", 77).Error; err != nil {
		t.Fatal(err)
	}
	var point db.P4CommerceRule
	if err := db.GetIns().Where("kind=?", "point").First(&point).Error; err != nil {
		t.Fatal(err)
	}
	var source p4CommerceRules
	resources := []string{"item_007", "item_013", "item_010", "item_005", "item_009", "item_008", "item_012", "item_006", "item_017", "frag_equip_013", "frag_equip_025", "frag_equip_037", "frag_equip_038", "frag_equip_039", "frag_equip_040"}
	for i, id := range resources {
		source.Products = append(source.Products, p4BaseProduct(fmt.Sprintf("trial77_%d", i), id+",1", "point,150", int32(i+1), 1))
	}
	point.Rules, _ = json.Marshal(source)
	if err := db.GetIns().Save(&point).Error; err != nil {
		t.Fatal(err)
	}
	window := point
	window.ID = "shop_schedule_point_activity77"
	window.Frozen, _ = json.Marshal(window)
	if err := db.GetIns().Create(&window).Error; err != nil {
		t.Fatal(err)
	}
	form := model.AdminActivityForm{Mode: "shop-window"}
	form.Options.ShopKey = "point"
	formJSON, _ := json.Marshal(form)
	occurrence := db.AdminActivityOccurrence{ID: "activity77-window", ServerID: f.s.OriginalServerID(), Mode: "shop-window", ConsumerKey: window.ID, ConsumerID: "commerce:" + window.ID, FormJSON: formJSON}
	if err := db.GetIns().Create(&occurrence).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyActivity77Shops(f.ctx, actor); err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyActivity77Shops(f.ctx, actor); err != nil {
		t.Fatal("retry", err)
	}
	if err := db.GetIns().First(&point, "id=?", point.ID).Error; err != nil {
		t.Fatal(err)
	}
	var got p4CommerceRules
	if err := json.Unmarshal(point.Rules, &got); err != nil {
		t.Fatal(err)
	}
	for i, want := range []int32{3, 3, 6, 9, 12, 18, 21, 24, 90, 30, 60, 135, 165, 135, 195} {
		if got.Products[i].MinTrialStars != want || got.Products[i].Cost != source.Products[i].Cost {
			t.Fatal("trial threshold or price", got.Products[i])
		}
	}
	if err := db.GetIns().First(&window, "id=?", window.ID).Error; err != nil {
		t.Fatal(err)
	}
	var frozen db.P4CommerceRule
	if err := json.Unmarshal(window.Frozen, &frozen); err != nil {
		t.Fatal(err)
	}
	var frozenRules p4CommerceRules
	if err := json.Unmarshal(frozen.Rules, &frozenRules); err != nil || !frozenRules.LiveEditable || frozenRules.Products[7].MinTrialStars != 24 {
		t.Fatal("frozen trial consumer", err, frozenRules)
	}
	shop, err := f.s.AdminShop(f.ctx, "score")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, item := range shop.Items {
		if item.AccountWeeklyLimit == 1 {
			count++
		}
	}
	if count != 5 {
		t.Fatalf("weekly bundles %d", count)
	}
	now := time.Now()
	list := p4Trade(t, f, &pb.CSShopProductList{ShopType: proto.Int32(7)}, 0, now).(*pb.SCShopProductList)
	var target *pb.ShopItem
	for _, item := range list.ShopItemList {
		if len(item.Item) == 1 && item.Item[0].GetId() == "item_006" && item.Item[0].GetNum() == 1000 {
			target = item
		}
	}
	if target == nil || target.GetDayLeftTimes() != 1 {
		t.Fatal("weekly offer missing", target)
	}
	buy := &pb.CSShopBuy{ShopType: proto.Int32(7), ShopItemId: target.Id, BuyCount: proto.Int32(1)}
	p4Trade(t, f, buy, 77001, now)
	if _, err := f.s.p4Commerce(f.ctx, f.id, buy, 77002, now); err == nil {
		t.Fatal("weekly duplicate accepted")
	}
	list = p4Trade(t, f, &pb.CSShopProductList{ShopType: proto.Int32(7)}, 0, now.Add(24*time.Hour)).(*pb.SCShopProductList)
	for _, item := range list.ShopItemList {
		if item.GetId() == target.GetId() && item.GetDayLeftTimes() != 0 {
			t.Fatal("weekly cap lost on next role day")
		}
	}
	var score db.P4CommerceRule
	if err := db.GetIns().Where("kind=?", "score").First(&score).Error; err != nil {
		t.Fatal(err)
	}
	var scoreRules p4CommerceRules
	if err := json.Unmarshal(score.Rules, &scoreRules); err != nil {
		t.Fatal(err)
	}
	for i := range scoreRules.Products {
		if scoreRules.Products[i].ID == "score_crystal77_1000" {
			scoreRules.Products[i].MinLevel = 1
		}
	}
	score.Rules, _ = json.Marshal(scoreRules)
	if err := db.GetIns().Save(&score).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.ApplyActivity77Shops(f.ctx, actor); err == nil {
		t.Fatal("conflicting weekly offer accepted")
	}

}
