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

func TestP8IngredientSupplyDefaultOffAppendFrozenAndAll10(t *testing.T) {
	f := p4Setup(t)
	now := time.Now()
	key := p4ExchangeKey(now)
	child := constUtil.P8IngredientSupplyPrefix + now.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
	base := p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, now).(*pb.SCActivityShopInfo)
	var cfg db.P4CommerceRule
	if e := db.GetIns().First(&cfg, "id=?", child).Error; e != nil {
		t.Fatal(e)
	}
	if cfg.Enabled || len(cfg.Frozen) != 0 {
		t.Fatal("defaultoff froze unpublished catalog")
	}
	for _, item := range base.GetShopItemList() {
		if strings.HasPrefix(item.GetGetResource()[0].GetId(), "FishItem_") {
			t.Fatal("defaultoff source visible")
		}
	}
	var rules p4CommerceRules
	if json.Unmarshal(cfg.Rules, &rules) != nil || len(rules.Products) != 10 {
		t.Fatal("all10 rawingredients")
	}
	if e := db.GetIns().Model(&cfg).Update("enabled", true).Error; e != nil {
		t.Fatal(e)
	}
	if e := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		p.Coin = proto.Int64(100000000)
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	listing := p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, now).(*pb.SCActivityShopInfo)
	if len(listing.GetShopItemList()) != len(base.GetShopItemList())+10 {
		t.Fatal("newsource lateappend")
	}
	for i, old := range base.GetShopItemList() {
		if old.GetIndex() != listing.GetShopItemList()[i].GetIndex() || old.GetGetResource()[0].GetId() != listing.GetShopItemList()[i].GetGetResource()[0].GetId() {
			t.Fatal("prior stableindex changed")
		}
	}
	var total int64
	var firstReq *pb.CSBuyActivityShop
	var first proto.Message
	for i, item := range listing.GetShopItemList()[len(base.GetShopItemList()):] {
		if item.GetName() == "" || strings.HasPrefix(item.GetName(), "FishItem_") {
			t.Fatal("original readable title")
		}
		req := &pb.CSBuyActivityShop{ActivityId: proto.String(key), Index: item.Index, Num: proto.Int32(1), CostResources: item.CostResource, GetResources: item.GetResource}
		bad := proto.Clone(req).(*pb.CSBuyActivityShop)
		bad.CostResources[0].Num = proto.Int32(1)
		if _, e := f.s.p4Commerce(f.ctx, f.id, bad, int64(i+1), now); e == nil {
			t.Fatal("fakecoinquote")
		}
		var wg sync.WaitGroup
		errs := make(chan error, 8)
		out := make(chan proto.Message, 8)
		for n := 0; n < 8; n++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, e := f.s.p4Commerce(f.ctx, f.id, req, int64(i+1), now)
				errs <- e
				if e == nil {
					out <- r[0]
				}
			}()
		}
		wg.Wait()
		close(errs)
		close(out)
		for e := range errs {
			if e != nil {
				t.Fatal(e)
			}
		}
		var expected proto.Message
		for r := range out {
			if expected == nil {
				expected = r
			} else if !proto.Equal(expected, r) {
				t.Fatal("duplicate ingredients")
			}
		}
		if i == 0 {
			firstReq = req
			first = expected
		}
		total += int64(item.GetCostResource()[0].GetNum())
		if _, e := f.s.p4Commerce(f.ctx, f.id, req, int64(i+101), now); e == nil {
			t.Fatal("dailycap")
		}
	}
	p, g := f.profile(t)
	if p.GetCoin() != 100000000-total || total != 18400000 || len(g.Cooking) != 10 {
		t.Fatal("originalrawinventory/coin", total, g.Cooking)
	}
	for _, v := range g.Cooking {
		want := int32(5)
		if v.GetId() == "FishItem_025" {
			want = 1
		}
		if v.GetNum() != want {
			t.Fatal("sourcequantity", v)
		}
	}
	// Once visible, hot price/quantity/gate edits leave the actual frozen catalog unchanged.
	rules.Products[0].Cost = "coin,1"
	rules.Products[0].Rewards = "FishItem_010,99"
	raw, _ := json.Marshal(rules)
	if e := db.GetIns().Model(&cfg).Update("rules", raw).Error; e != nil {
		t.Fatal(e)
	}
	frozen := p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, now).(*pb.SCActivityShopInfo)
	for _, v := range frozen.GetShopItemList() {
		if v.GetIndex() == firstReq.GetIndex() && (v.GetCostResource()[0].GetNum() != 400000 || v.GetGetResource()[0].GetNum() != 5) {
			t.Fatal("hotedited frozen rule")
		}
	}
	for day := 1; day < 4; day++ {
		var state db.P4CommerceState
		if e := db.GetIns().First(&state, "player_id=? AND rule_id=?", f.id, child).Error; e != nil {
			t.Fatal(e)
		}
		var progress p4CommerceProgress
		if e := json.Unmarshal(state.Payload, &progress); e != nil {
			t.Fatal(e)
		}
		progress.Day = "20000101"
		if e := saveP4Commerce(db.GetIns(), &state, &progress); e != nil {
			t.Fatal(e)
		}
		p4Trade(t, f, firstReq, int64(201+day), now)
	}
	var state db.P4CommerceState
	if e := db.GetIns().First(&state, "player_id=? AND rule_id=?", f.id, child).Error; e != nil {
		t.Fatal(e)
	}
	var progress p4CommerceProgress
	json.Unmarshal(state.Payload, &progress)
	progress.Day = "20000101"
	saveP4Commerce(db.GetIns(), &state, &progress)
	if _, e := f.s.p4Commerce(f.ctx, f.id, firstReq, 205, now); e == nil {
		t.Fatal("monthly4cap")
	}
	if e := db.GetIns().Model(&cfg).Update("enabled", false).Error; e != nil {
		t.Fatal(e)
	}
	if !proto.Equal(first, p4Trade(t, f, firstReq, 1, now)) {
		t.Fatal("offoldreceipt")
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	if e := f.s.InitializeP4(f.ctx); e != nil {
		t.Fatal(e)
	}
	db.GetIns().First(&cfg, "id=?", child)
	if cfg.Enabled {
		t.Fatal("operatoroff reenables")
	}
	if !proto.Equal(first, p4Trade(t, f, firstReq, 1, now)) {
		t.Fatal("coldoldsource")
	}
	t.Log("defaultoff editable/unfrozen;10canonicalnames/quantities/prices;stabletail;8eachonce/fakequotes/day1/month4/frozenhot/coldoff")
}
