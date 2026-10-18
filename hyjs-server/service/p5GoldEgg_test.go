package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"sync"
	"testing"
	"time"
)

func TestP5GoldEggOriginalAll54AndOneUse(t *testing.T) {
	f := p4Setup(t)
	now := time.Now()
	key := p4ExchangeKey(now)
	old := p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, now).(*pb.SCActivityShopInfo)
	if err := f.s.InitializeP5(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		p.Coin = proto.Int64(20000000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	listing := p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, now).(*pb.SCActivityShopInfo)
	if len(listing.GetShopItemList()) != len(old.GetShopItemList())+1 {
		t.Fatal("append new supply without losing old products")
	}
	for i, item := range old.GetShopItemList() {
		if listing.GetShopItemList()[i].GetIndex() != item.GetIndex() {
			t.Fatal("renumbered old receipt indices")
		}
	}
	item := listing.GetShopItemList()[len(listing.GetShopItemList())-1]
	if item.GetGetResource()[0].GetId() != "item_360" || item.GetCostResource()[0].GetNum() != 4000000 || item.GetIndex() != 11 {
		t.Fatal("named source and append index", item)
	}
	buy := &pb.CSBuyActivityShop{ActivityId: proto.String(key), Index: item.Index, Num: proto.Int32(1), CostResources: item.CostResource, GetResources: item.GetResource}
	p4Trade(t, f, buy, 1, now)
	use := &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_360"), Num: proto.Int32(1)}}
	bad := proto.Clone(use).(*pb.CSItemUse)
	bad.CostItem.Num = proto.Int32(2)
	if _, e := f.s.p5GoldEggUse(f.ctx, f.id, bad, 2); e == nil {
		t.Fatal("original gold egg one-use violated")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := f.s.p5GoldEggUse(f.ctx, f.id, use, 3); errs <- e }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	p, g := f.profile(t)
	if simpleOwned(p, g, "item_360") != 0 {
		t.Fatal("actual consumption")
	}
	var cfg db.P5GoldEggRule
	if e := db.GetIns().First(&cfg, "id=?", constUtil.P5GoldEggRuleID).Error; e != nil {
		t.Fatal(e)
	}
	var options []p5GoldEggOption
	if json.Unmarshal(cfg.Frozen, &options) != nil || len(options) != 54 {
		t.Fatal("54 frozen original options")
	}
	var total int64
	for _, o := range options {
		total += o.Weight
		p := &pb.Player{EquipBagSize: proto.Int32(60), WarriorBagSize: proto.Int32(60), Level: proto.Int32(100)}
		g := &model.GameState{}
		if _, e := f.s.GrantFixedReward(p, g, o.Reward); e != nil {
			t.Fatalf("original %s %s: %v", o.ID, o.Reward, e)
		}
		if o.ID == "881" && o.Reward != "MineItem_028,1" {
			t.Fatal("narrow original punctuation repair")
		}
	}
	if total != 110150 {
		t.Fatal("original relative weight total")
	}
	if e := db.GetIns().Model(&cfg).Update("rules", []byte(`[]`)).Error; e != nil {
		t.Fatal(e)
	}
	// Current frozen options remain independently valid; malformed source bytes cannot change them.
	if e := db.GetIns().Transaction(func(tx *gorm.DB) error { _, e := f.s.p5GoldEggOptions(tx); return e }); e != nil {
		t.Fatal(e)
	}
	if e := db.GetIns().Model(&cfg).Update("enabled", false).Error; e != nil {
		t.Fatal(e)
	}
	prior, e := f.s.p5GoldEggUse(f.ctx, f.id, use, 3)
	if e != nil || prior[0].(*pb.SCItemUse).GetCost().GetNum() != 1 {
		t.Fatal("closed old exact receipt", e)
	}
	if _, e = f.s.p5GoldEggUse(f.ctx, f.id, use, 4); e == nil {
		t.Fatal("operator-off use")
	}
	listing = p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, now).(*pb.SCActivityShopInfo)
	if len(listing.GetShopItemList()) != 10 {
		t.Fatal("closed gold source visible")
	}
	pool, _ := db.GetIns().DB()
	pool.Close()
	f.open(t)
	if e = f.s.InitializeP5(f.ctx); e != nil {
		t.Fatal(e)
	}
	again, e := f.s.p5GoldEggUse(f.ctx, f.id, use, 3)
	if e != nil || !proto.Equal(prior[0], again[0]) {
		t.Fatal("cold original item receipt", e)
	}
	t.Log("54 original awards/110150 frozen relative weights/single action/8once/cold/off/late append indices/sourcecoin4m; original private param/rate/pity unknown")
}
