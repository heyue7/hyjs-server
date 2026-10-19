package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
	"time"
)

func TestReRushShopNativeCataloguePurchaseAndCold(t *testing.T) {
	f := p4Setup(t)
	now := time.Now()
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Level = proto.Int32(22)
		p.Point = proto.Int32(4000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	list := func() []*pb.ShopItem {
		t.Helper()
		out, err := f.s.p4Commerce(f.ctx, f.id, &pb.CSShopProductList{ShopType: proto.Int32(5)}, 0, now)
		if err != nil {
			t.Fatal(err)
		}
		return out[0].(*pb.SCShopProductList).GetShopItemList()
	}
	items := list()
	if len(items) != 8 {
		t.Fatal("eight source products missing", len(items))
	}
	goods := []string{"item_005", "item_008", "item_012", "item_007", "item_006", "item_013", "item_009", "item_010"}
	var selected *pb.ShopItem
	for _, item := range items {
		_, key, ok := adminLiveShopParseSKU(item.GetId())
		if !ok {
			t.Fatal("live source quote missing")
		}
		index := -1
		for i := range goods {
			if key == fmt.Sprintf("offline_rush_shop_%d", i) {
				index = i
				break
			}
		}
		if index < 0 || item.GetItem()[0].GetId() != goods[index] || item.GetItem()[0].GetNum() != 10 || item.GetExchangeList()[0].GetId() != "point" || item.GetExchangeList()[0].GetNum() != int32(500+index*100) || item.GetDayTotalTimes() != 1000 || item.GetPriority() != int32(1000+(index%4)*1000+index) {
			t.Fatal("source catalogue mismatch", item)
		}
		if index == 0 {
			selected = item
		}
	}
	beforeP, beforeG := f.profile(t)
	before := simpleOwned(beforeP, beforeG, "item_005")
	request := &pb.CSShopBuy{ShopType: proto.Int32(5), ShopItemId: selected.Id, BuyCount: proto.Int32(2)}
	paid, err := f.s.p4Commerce(f.ctx, f.id, request, 98001, now)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := f.s.p4Commerce(f.ctx, f.id, request, 98001, now)
	if err != nil || !proto.Equal(paid[0], retry[0]) {
		t.Fatal("source purchase retry", err)
	}
	p, g := f.profile(t)
	if p.GetPoint() != 3000 || simpleOwned(p, g, "item_005")-before != 20 {
		t.Fatal("actual point charge/reward duplicated or wrong")
	}
	for _, item := range list() {
		_, key, _ := adminLiveShopParseSKU(item.GetId())
		if key == "offline_rush_shop_0" && item.GetDayLeftTimes() != 998 {
			t.Fatal("daily source quota")
		}
	}
	f.restart(t)
	retry, err = f.s.p4Commerce(f.ctx, f.id, request, 98001, now)
	if err != nil || !proto.Equal(paid[0], retry[0]) {
		t.Fatal("cold source receipt", err)
	}
	var row db.P4CommerceRule
	if err = db.GetIns().Where("kind=?", "point").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&row).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	if err = f.s.seedP4Commerce(db.GetIns().WithContext(f.ctx), now.AddDate(0, 1, 0)); err != nil {
		t.Fatal(err)
	}
	db.GetIns().First(&row, "id=?", row.ID)
	if row.Enabled || p1Count(t, &db.P4CommerceRule{}, "kind=?", "point") != 1 {
		t.Fatal("restart/month seed resurrected or replaced operator stop")
	}
}

func TestReRushShopLegacyHistoryAndOperatorPreserved(t *testing.T) {
	f := p4Setup(t)
	now := time.Now()
	if err := db.GetIns().Where("kind=?", "point").Delete(&db.P4CommerceRule{}).Error; err != nil {
		t.Fatal(err)
	}
	legacy := p4CommerceRules{Products: []p4Product{p4BaseProduct("p4Point_1", "item_021,1", "point,100", 1, 22), p4BaseProduct("p4Point_2", "equip_037,1", "item_021,10", 2, 22)}}
	legacy.Products[1].Priority = 2001
	legacy.Products[1].DailyLimit = 1
	cfg := db.P4CommerceRule{ID: "p4:point:202610", Kind: "point", ShopType: 5, Enabled: false, StartAt: now.Add(-time.Hour).UnixMilli(), EndAt: now.Add(time.Hour).UnixMilli(), ClaimEndAt: now.Add(time.Hour).UnixMilli(), Rules: adminJSON(legacy), Source: "本地兼容服目录/限购/库存：旧自动占位"}
	cfg.Frozen = adminJSON(cfg)
	frozen := append([]byte{}, cfg.Frozen...)
	if err := db.GetIns().Create(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	state := db.P4CommerceState{PlayerID: f.id, RuleID: cfg.ID, Payload: adminJSON(p4CommerceProgress{Day: p1Day(now), Bought: map[string]int32{"p4Point_1": 3}, Total: map[string]int32{"p4Point_1": 3}, Version: 1})}
	stock := db.P4CommerceStock{RuleID: cfg.ID, ProductID: "p4Point_1", Used: 7}
	if err := db.GetIns().Create(&state).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Create(&stock).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.seedReRushShop(db.GetIns().WithContext(f.ctx), now); err != nil {
		t.Fatal(err)
	}
	var current db.P4CommerceRule
	db.GetIns().First(&current, "id=?", cfg.ID)
	var rules p4CommerceRules
	_ = json.Unmarshal(current.Rules, &rules)
	if !current.Enabled || len(rules.Products) != 8 || !rules.LiveEditable || !bytes.Equal(current.Frozen, frozen) || !strings.Contains(current.Source, constUtil.ReRushShopPolicy) {
		t.Fatal("safe catalogue upgrade")
	}
	var retained db.P4CommerceState
	db.GetIns().First(&retained, "player_id=? AND rule_id=?", f.id, cfg.ID)
	var retainedStock db.P4CommerceStock
	db.GetIns().First(&retainedStock, "rule_id=? AND product_id=?", cfg.ID, stock.ProductID)
	if !bytes.Equal(state.Payload, retained.Payload) || retainedStock.Used != 7 {
		t.Fatal("legacy quota/stock changed")
	}
	// Editing the old placeholder before migration remains an explicit operator choice.
	legacy.Products[0].Cost = "point,101"
	cfg.Rules = adminJSON(legacy)
	cfg.Frozen = nil
	if err := db.GetIns().Save(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.seedReRushShop(db.GetIns().WithContext(f.ctx), now); err != nil {
		t.Fatal(err)
	}
	db.GetIns().First(&current, "id=?", cfg.ID)
	if current.Enabled || !bytes.Equal(current.Rules, cfg.Rules) {
		t.Fatal("operator catalogue overwritten")
	}
}
