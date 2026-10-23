package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

func TestMemoryShrine55Flow(t *testing.T) {
	f := p4Setup(t)
	p4Fund(t, f)
	now := time.Now()
	if err := db.GetIns().Model(&db.P4CommerceRule{}).Where("kind = ?", "memory").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	q := &pb.CSRefreshShopQuery{ShopType: proto.Int32(3)}
	p4Trade(t, f, q, 0, now)
	var row db.P4CommerceState
	if err := db.GetIns().Where("player_id = ?", f.id).First(&row).Error; err != nil {
		t.Fatal(err)
	}
	var cfg db.P4CommerceRule
	if err := db.GetIns().Where("id = ?", row.RuleID).First(&cfg).Error; err != nil {
		t.Fatal(err)
	}
	var rules p4CommerceRules
	json.Unmarshal(cfg.Rules, &rules)
	var material p4Product
	for i := range rules.Products {
		if rules.Products[i].Rewards == "item_013,1" {
			rules.Products[i].Stock = 2
			rules.Products[i].TotalLimit = 2
			material = rules.Products[i]
		}
	}
	if material.ID == "" {
		t.Fatal("material missing")
	}
	raw, _ := json.Marshal(rules)
	if err := db.GetIns().Model(&cfg).Updates(map[string]any{"rules": raw, "frozen": nil}).Error; err != nil {
		t.Fatal(err)
	}
	var state p4CommerceProgress
	json.Unmarshal(row.Payload, &state)
	state.Slots = nil
	state.Offers = []memoryOffer55{{ID: "memory55:1:1:" + material.ID, ProductID: material.ID}, {ID: "memory55:1:2:" + material.ID, ProductID: material.ID}}
	if err := saveP4Commerce(db.GetIns(), &row, &state); err != nil {
		t.Fatal(err)
	}
	list := p4Trade(t, f, q, 0, now).(*pb.SCRefreshShopQuery).GetShopItemList()
	if len(list) != 2 {
		t.Fatal(len(list))
	}
	buy := &pb.CSRefreshShopBuy{ShopType: proto.Int32(3), ShopItemId: list[0].Id}
	p4Trade(t, f, buy, 55001, now)
	p4Trade(t, f, buy, 55001, now)
	f.open(t)
	p4Trade(t, f, &pb.CSRefreshShopBuy{ShopType: proto.Int32(3), ShopItemId: list[1].Id}, 55002, now)
	if err := db.GetIns().Where("player_id = ? AND rule_id = ?", f.id, row.RuleID).First(&row).Error; err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(row.Payload, &state)
	if state.Total[material.ID] != 2 {
		t.Fatal(state.Total)
	}
	var stock db.P4CommerceStock
	if err := db.GetIns().Where("rule_id = ? AND product_id = ?", cfg.ID, material.ID).First(&stock).Error; err != nil {
		t.Fatal(err)
	}
	if stock.Used != 2 {
		t.Fatal(stock.Used)
	}
}
