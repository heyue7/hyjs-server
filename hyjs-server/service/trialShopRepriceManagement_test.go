package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func TestTrialShopRepriceManagementCASAuditAndConsumer(t *testing.T) {
	f, actor := adminModesFixture(t)
	if err := f.s.InitializeAdminShopAvailability(f.ctx); err != nil {
		t.Fatal(err)
	}
	p4Fund(t, f)
	var point db.P4CommerceRule
	if err := db.GetIns().Where("kind=?", "point").First(&point).Error; err != nil {
		t.Fatal(err)
	}
	rules := p4CommerceRules{}
	for i, reward := range []string{"frag_equip_037,10", "frag_equip_038,10", "frag_equip_039,10", "frag_equip_040,10"} {
		rules.Products = append(rules.Products, p4BaseProduct("trialReprice"+string(rune('A'+i)), reward, "point,15000", int32(i+1), 1))
	}
	rules.Products = append(rules.Products, p4BaseProduct("otherTrialGood", "item_006,5", "point,30", 5, 1))
	point.Rules, _ = json.Marshal(rules)
	point.Frozen, _ = json.Marshal(point)
	if err := db.GetIns().Save(&point).Error; err != nil {
		t.Fatal(err)
	}
	var before []db.P4CommerceRule
	if err := db.GetIns().Where("id<>?", point.ID).Order("id").Find(&before).Error; err != nil {
		t.Fatal(err)
	}
	beforeRaw, _ := json.Marshal(before)
	shop, err := f.s.AdminShop(f.ctx, "point")
	if err != nil {
		t.Fatal(err)
	}
	oldVersion := shop.Version
	changed, err := f.s.ApplyTrialShopReprice(f.ctx, actor, oldVersion)
	if err != nil || changed.Version == oldVersion {
		t.Fatal("reprice CAS mutation", err)
	}
	if _, err = f.s.ApplyTrialShopReprice(f.ctx, actor, oldVersion); !errors.Is(err, constUtil.ErrAdminConfigConflict) {
		t.Fatal("stale version accepted", err)
	}
	var audit []db.AdminAudit
	if err = db.GetIns().Where("action=? AND target=?", "shop.trial.reprice", "point").Find(&audit).Error; err != nil || len(audit) != 1 || audit[0].Actor != actor.Username {
		t.Fatal("missing or duplicate committed audit", err, audit)
	}
	var after []db.P4CommerceRule
	if err = db.GetIns().Where("id<>?", point.ID).Order("id").Find(&after).Error; err != nil {
		t.Fatal(err)
	}
	afterRaw, _ := json.Marshal(after)
	if !bytes.Equal(beforeRaw, afterRaw) {
		t.Fatal("another catalogue changed")
	}
	var stored db.P4CommerceRule
	if err = db.GetIns().First(&stored, "id=?", point.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored.Frozen, point.Frozen) {
		t.Fatal("historical frozen catalogue changed")
	}
	_, consumer, err := f.s.p4CommerceSnapshot(stored)
	if err != nil || !consumer.LiveEditable || consumer.Products[0].Cost != "point,150" || consumer.Products[0].MinTrialStars != 135 || consumer.Products[4].Cost != "point,30" {
		t.Fatal("consumer kept frozen price or changed another good", err, consumer)
	}
	if err = db.GetIns().Create(&db.P7RushState{PlayerID: f.id, RecordStars: 135}).Error; err != nil {
		t.Fatal(err)
	}
	messages, err := f.s.p4Commerce(f.ctx, f.id, &pb.CSShopProductList{ShopType: proto.Int32(5)}, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	list := messages[0].(*pb.SCShopProductList).GetShopItemList()
	if len(list) != 5 || list[0].GetPrice()[0].GetNum() != 150 {
		t.Fatal("original listing missed reprice", list)
	}
	buy := &pb.CSShopBuy{ShopType: proto.Int32(5), ShopItemId: list[0].Id, BuyCount: proto.Int32(1)}
	p4Trade(t, f, buy, 73101, time.Now())
	player, game := f.profile(t)
	if player.GetPoint() != 850 || simpleOwned(player, game, "frag_equip_037") != 10 {
		t.Fatal("original purchase terms", player.GetPoint())
	}
	// A current-version replay is safe and never resets purchase history.
	if _, err = f.s.ApplyTrialShopReprice(f.ctx, actor, changed.Version); err != nil {
		t.Fatal(err)
	}
	refreshed := p4Trade(t, f, &pb.CSShopProductList{ShopType: proto.Int32(5)}, 0, time.Now()).(*pb.SCShopProductList)
	if refreshed.GetShopItemList()[0].GetCanBuy() != 2 {
		t.Fatal("reprice reset lifetime purchase")
	}
}
