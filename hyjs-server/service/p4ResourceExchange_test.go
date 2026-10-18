package service

import (
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestP4ResourceExchangeOriginalSingleType23(t *testing.T) {
	f := p4Setup(t)
	now := time.Now()
	key := p4ExchangeKey(now)
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		p.Coin = proto.Int64(100000000)
		g.Items = append(g.Items, &pb.SimpleItem{Id: proto.String("item_021"), Num: proto.Int32(10)})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	tabs, err := f.s.P4ActivityTabs(f.ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, tab := range tabs {
		if strings.HasPrefix(tab.GetActivityId(), "limitExchange_") {
			n++
			if tab.GetActivityId() != key {
				t.Fatal("original dictionary collision", tab)
			}
		}
	}
	if n != 1 {
		t.Fatal("exactly one original Type23", n)
	}
	listing := p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, now).(*pb.SCActivityShopInfo)
	if len(listing.GetShopItemList()) != 10 {
		t.Fatal("all base/gem/book products", listing)
	}
	if listing.GetShopItemList()[0].GetName() != constUtil.P4EssenceDisplayName {
		t.Fatal("original canonical equipment title", listing.GetShopItemList()[0])
	}
	var gem *pb.ActivityShopItem
	seen := map[int32]bool{}
	for _, item := range listing.GetShopItemList() {
		if seen[item.GetIndex()] {
			t.Fatal("outer indices overlap")
		}
		seen[item.GetIndex()] = true
		if item.GetGetResource()[0].GetId() == "item_137" {
			gem = item
		}
	}
	if gem == nil || gem.GetCostResource()[0].GetNum() != 2000000 {
		t.Fatal("original gem price")
	}
	req := &pb.CSBuyActivityShop{ActivityId: proto.String(key), Index: gem.Index, Num: proto.Int32(1), CostResources: gem.CostResource, GetResources: gem.GetResource}
	bad := proto.Clone(req).(*pb.CSBuyActivityShop)
	bad.Index = proto.Int32(1001)
	if _, err = f.s.p4Commerce(f.ctx, f.id, bad, 1, now); err == nil {
		t.Fatal("forged index")
	}
	bad = proto.Clone(req).(*pb.CSBuyActivityShop)
	bad.CostResources[0].Num = proto.Int32(1)
	if _, err = f.s.p4Commerce(f.ctx, f.id, bad, 1, now); err == nil {
		t.Fatal("forged quote")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := f.s.p4Commerce(f.ctx, f.id, req, 2, now); errs <- e }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	p, g := f.profile(t)
	if p.GetCoin() != 98000000 || simpleOwned(p, g, "item_137") != 1 {
		t.Fatal("exact once original cost/inventory")
	}
	child := constUtil.P5GemSupplyPrefix + now.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
	legacy := proto.Clone(req).(*pb.CSBuyActivityShop)
	legacy.ActivityId = proto.String(child)
	legacy.Index = proto.Int32(1)
	if _, err = f.s.p4Commerce(f.ctx, f.id, legacy, 3, now); err == nil {
		t.Fatal("new quota through legacy endpoint")
	}
	current := p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, now).(*pb.SCActivityShopInfo)
	for _, item := range current.GetShopItemList() {
		if item.GetIndex() == gem.GetIndex() && item.GetLeft() != 0 {
			t.Fatal("outer projection lost child quota")
		}
	}
	var state db.P4CommerceState
	if err = db.GetIns().First(&state, "player_id=? AND rule_id=?", f.id, child).Error; err != nil {
		t.Fatal(err)
	}
	var progress p4CommerceProgress
	if json.Unmarshal(state.Payload, &progress) != nil || progress.Total["item_137"] != 1 {
		t.Fatal("quota owner", string(state.Payload))
	}
	if err = db.GetIns().Model(&db.P4CommerceRule{}).Where("id=?", child).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	current = p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, now).(*pb.SCActivityShopInfo)
	if len(current.GetShopItemList()) != 5 {
		t.Fatal("closed child visible")
	}
	if _, err = f.s.p4Commerce(f.ctx, f.id, req, 4, now); err == nil {
		t.Fatal("closed child purchase")
	}
	if err = db.GetIns().Model(&db.P4ResourceExchangeDirectory{}).Where("id=?", key).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	prior := p4Trade(t, f, req, 2, now).(*pb.SCGetActivityShopReward)
	if prior.GetGet().GetSimps()[0].GetId() != "item_137" {
		t.Fatal("closed exact prior receipt")
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if err = f.s.InitializeP4(f.ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p4Commerce(f.ctx, f.id, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, now); err == nil {
		t.Fatal("restart reopened operator")
	}
	tabs, err = f.s.P4ActivityTabs(f.ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, tab := range tabs {
		if tab.GetActivityId() == key {
			t.Fatal("disabled directory visible")
		}
	}
	t.Log("single original Type23/all10 unique-index products/shared original limits/8once/frozen source/disabled child+directory/cold original receipt")
}
