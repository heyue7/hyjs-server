package service

import (
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strconv"
	"sync"
	"testing"
	"time"
)

// TestP5GemSupply verifies every six-color selection without injected boxes or gems.
// The finite coin fixture checks transaction conditions; HTTP verifies its actual source.
func TestP5GemSupply(t *testing.T) {
	f := p4Setup(t)
	if err := f.s.InitializeP5(f.ctx); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	key := constUtil.P5GemSupplyPrefix + now.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
	listing := p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, now).(*pb.SCActivityShopInfo)
	if len(listing.GetShopItemList()) != 5 {
		t.Fatal("five original gem packages")
	}
	prices := []int32{2000000, 7000000, 8000000, 9000000, 10000000}
	ids := []string{"item_137", "item_342", "item_343", "item_344", "item_345"}
	for i, item := range listing.GetShopItemList() {
		if item.GetGetResource()[0].GetId() != ids[i] || item.GetGetResource()[0].GetNum() != 1 || item.GetCostResource()[0].GetNum() != prices[i] {
			t.Fatal("original manufacturing anchor", item)
		}
	}
	var stamp int64 = 100
	for color := int32(1); color <= 6; color++ {
		a, err := LoginAccount(f.ctx, "gem-supply-six-color"+strconv.Itoa(int(color)), "isolated-only")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := CreatePlayer(f.ctx, a.Id, "原六色兑换", "hero_043"); err != nil {
			t.Fatal(err)
		}
		if err := updateProfile(f.ctx, a.Id, func(p *pb.Player, _ *model.GameState) error {
			p.Level = proto.Int32(100)
			p.Coin = proto.Int64(100000000)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		for index, item := range listing.GetShopItemList() {
			req := &pb.CSBuyActivityShop{ActivityId: proto.String(key), Index: item.Index, Num: proto.Int32(1), CostResources: item.GetCostResource(), GetResources: item.GetGetResource()}
			stamp++
			out, err := f.s.p4Commerce(f.ctx, a.Id, req, stamp, now)
			if err != nil {
				t.Fatal(err)
			}
			if out[0].(*pb.SCGetActivityShopReward).GetGet().GetSimps()[0].GetId() != ids[index] {
				t.Fatal("box purchase")
			}
			stamp++
			choice := color
			if index == 0 {
				choice = 0
			}
			opened, err := f.s.p5BoxUse(f.ctx, a.Id, &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String(ids[index]), Num: proto.Int32(1)}, GetItemIndex: proto.Int32(choice)}, stamp)
			if err != nil {
				t.Fatal(err)
			}
			gains := opened[0].(*pb.SCItemUse).GetGet().GetSimps()
			if len(gains) != 1 || gains[0].GetNum() != 1 {
				t.Fatal("one actual original stone", gains)
			}
			row := f.s.table("MineSkill", "MineItemID", gains[0].GetId())
			want := []string{"5", "7", "8", "9", "10"}[index]
			if row["Type"] != "magicGem" || row["Level"] != want || (index > 0 && row["Color"] != strconv.Itoa(int(color))) {
				t.Fatal("original level/color", row)
			}
			if _, err := f.s.p4Commerce(f.ctx, a.Id, req, stamp+1, now); err == nil {
				t.Fatal("daily supply limit bypass")
			}
		}
	}
	// A false quote and an eight-frame duplicate remain atomic under player locks.
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.Level = proto.Int32(100)
		p.Coin = proto.Int64(100000000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	item := listing.GetShopItemList()[4]
	request := &pb.CSBuyActivityShop{ActivityId: proto.String(key), Index: item.Index, Num: proto.Int32(1), CostResources: item.GetCostResource(), GetResources: item.GetGetResource()}
	bad := proto.Clone(request).(*pb.CSBuyActivityShop)
	bad.CostResources = []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(1)}}
	if _, err := f.s.p4Commerce(f.ctx, f.id, bad, 9000, now); err == nil {
		t.Fatal("false quote accepted")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := f.s.p4Commerce(f.ctx, f.id, request, 9001, now); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	p, g := f.profile(t)
	if p.GetCoin() != 90000000 || simpleOwned(p, g, "item_345") != 1 {
		t.Fatal("same frame duplicated cost/gain")
	}
	var rule db.P4CommerceRule
	if err := db.GetIns().First(&rule, "id=?", key).Error; err != nil {
		t.Fatal(err)
	}
	var rules p4CommerceRules
	if err := json.Unmarshal(rule.Rules, &rules); err != nil {
		t.Fatal(err)
	}
	rules.Products[4].Cost = "coin,1"
	changed, _ := json.Marshal(rules)
	if err := db.GetIns().Model(&rule).Update("rules", changed).Error; err != nil {
		t.Fatal(err)
	}
	current := p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, now).(*pb.SCActivityShopInfo)
	if current.GetShopItemList()[4].GetCostResource()[0].GetNum() != 10000000 {
		t.Fatal("live edit changed frozen price")
	}
	// Only an isolated previous-day eligibility cursor changes; all event times stay real.
	for day := 1; day <= 3; day++ {
		var progressRow db.P4CommerceState
		if err := db.GetIns().First(&progressRow, "player_id=? AND rule_id=?", f.id, key).Error; err != nil {
			t.Fatal(err)
		}
		var progress p4CommerceProgress
		if err := json.Unmarshal(progressRow.Payload, &progress); err != nil {
			t.Fatal(err)
		}
		progress.Day = "20000101"
		progressRow.Payload, _ = json.Marshal(progress)
		if err := db.GetIns().Save(&progressRow).Error; err != nil {
			t.Fatal(err)
		}
		_, err := f.s.p4Commerce(f.ctx, f.id, request, int64(9001+day), now)
		if (day <= 2 && err != nil) || (day == 3 && err == nil) {
			t.Fatalf("monthly3 cap day%d: %v", day, err)
		}
	}
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error { return f.s.seedP5GemSupply(tx, now) }); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().First(&rule, "id=?", key).Error; err != nil {
		t.Fatal(err)
	}
	if string(rule.Rules) != string(changed) {
		t.Fatal("seed overwrote operator")
	}
	t.Log(fmt.Sprintf("all5 boxes/24 six-color choices, random original5, anchored prices, quote/limit/concurrency/freeze and operator preservation"))
}
