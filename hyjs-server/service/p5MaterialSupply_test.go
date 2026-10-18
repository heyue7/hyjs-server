package service

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestP5MaterialSupplyFrozenLimits verifies all four declared local products.
// Finite balances here verify transaction invariants; natural source is HTTP tested separately.
func TestP5MaterialSupplyFrozenLimits(t *testing.T) {
	f := p4Setup(t)
	now := time.Now().Add(-24 * time.Hour).In(time.FixedZone("UTC+8", 8*3600))
	key := constUtil.P5SpecialtySupplyPrefix + now.Format("200601")
	query := &pb.CSActivityShopInfo{ActivityId: proto.String(key)}
	listing := p4Trade(t, f, query, 0, now).(*pb.SCActivityShopInfo)
	if len(listing.GetShopItemList()) != 4 {
		t.Fatal("four specialty materials missing")
	}
	tabs, err := f.s.P4ActivityTabs(f.ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tab := range tabs {
		if tab.GetActivityId() == p4ExchangeKey(now) && tab.GetName() == "资源兑换" {
			found = true
		}
	}
	if !found {
		t.Fatal("original exchange entry not shown")
	}
	p, g := f.profile(t)
	beforeP := proto.Clone(p)
	beforeG, _ := json.Marshal(g)
	firstItem := listing.GetShopItemList()[0]
	if firstItem.GetName() != "新手技能专精书" {
		t.Fatal("original readable book label missing")
	}
	firstRequest := &pb.CSBuyActivityShop{ActivityId: proto.String(key), Index: proto.Int32(1), Num: proto.Int32(1), CostResources: firstItem.GetCostResource(), GetResources: firstItem.GetGetResource()}
	if _, err := f.s.p4Commerce(f.ctx, f.id, firstRequest, 1, now); err == nil {
		t.Fatal("level/coin prerequisites bypassed")
	}
	p, g = f.profile(t)
	raw, _ := json.Marshal(g)
	if !proto.Equal(p, beforeP) || string(raw) != string(beforeG) {
		t.Fatal("ineligible trade changed profile")
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		p.Coin = proto.Int64(100000000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var first proto.Message
	for index, item := range listing.GetShopItemList() {
		request := &pb.CSBuyActivityShop{ActivityId: proto.String(key), Index: item.Index, Num: proto.Int32(1), CostResources: item.GetCostResource(), GetResources: item.GetGetResource()}
		bad := proto.Clone(request).(*pb.CSBuyActivityShop)
		bad.CostResources = []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(1)}}
		if _, err := f.s.p4Commerce(f.ctx, f.id, bad, int64(index+1), now); err == nil {
			t.Fatal("false local quote accepted")
		}
		var wg sync.WaitGroup
		results := make(chan proto.Message, 8)
		errs := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				out, e := f.s.p4Commerce(f.ctx, f.id, request, int64(index+1), now)
				errs <- e
				if e == nil {
					results <- out[0]
				}
			}()
		}
		wg.Wait()
		close(results)
		close(errs)
		for e := range errs {
			if e != nil {
				t.Fatal(e)
			}
		}
		var result proto.Message
		for r := range results {
			if result == nil {
				result = r
			}
			if !proto.Equal(result, r) {
				t.Fatal("material exchange receipt differed")
			}
		}
		if index == 0 {
			first = result
		}
		_, g = f.profile(t)
		if simpleOwned(nil, g, item.GetGetResource()[0].GetId()) != 10 {
			t.Fatal("material source granted multiple times")
		}
	}
	p, _ = f.profile(t)
	if p.GetCoin() != 100000000-750000 {
		t.Fatal("server quote total not consumed", p.GetCoin())
	}
	// Live price edits do not alter the frozen window or its historical receipts.
	var cfg db.P4CommerceRule
	if err := db.GetIns().First(&cfg, "id=?", key).Error; err != nil {
		t.Fatal(err)
	}
	var rules p4CommerceRules
	json.Unmarshal(cfg.Rules, &rules)
	rules.Products[0].Cost = "coin,1"
	cfg.Rules, _ = json.Marshal(rules)
	if err := db.GetIns().Model(&cfg).Update("rules", cfg.Rules).Error; err != nil {
		t.Fatal(err)
	}
	bulk := proto.Clone(firstRequest).(*pb.CSBuyActivityShop)
	bulk.Num = proto.Int32(19)
	p4Trade(t, f, bulk, 5, now)
	if _, err := f.s.p4Commerce(f.ctx, f.id, firstRequest, 6, now); err == nil {
		t.Fatal("daily quota exceeded")
	}
	if !proto.Equal(first, p4Trade(t, f, firstRequest, 1, now.AddDate(0, 1, 0))) {
		t.Fatal("month boundary changed old receipt")
	}
	p4Trade(t, f, firstRequest, 6, now.AddDate(0, 0, 1))
	_, g = f.profile(t)
	if simpleOwned(nil, g, "item_441") != 210 {
		t.Fatal("daily quota did not reset")
	}
	pool, _ := db.GetIns().DB()
	pool.Close()
	f.open(t)
	if !proto.Equal(first, p4Trade(t, f, firstRequest, 1, now)) {
		t.Fatal("cold material source replay changed")
	}
	t.Log("four original IDs, actual gold price, false quote/level failure, eight concurrent per product, frozen live edits, daily quota/reset, old-month receipt and cold reopen passed")
}
