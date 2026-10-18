package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP5AwakeMaterialOriginalChoiceAndActualFusionUse(t *testing.T) {
	f := p4Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{f.s.InitializeP5, f.s.InitializeWarriorGrowth, f.s.InitializeWarriorAwake, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready = true
	var target string
	// The existing awakening qualification is prepared through the actual talent
	// and two awakening APIs. Only its original donor/material prerequisites and
	// level/capital are finite fixtures; no awakening stage/fusion value is set.
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(100)
		p.Coin = proto.Int64(4000000)
		gain, e := f.s.GrantFixedReward(p, g, "hero_001,8;item_005,30;item_320,2000;item_313,130;item_312,45")
		if e != nil {
			return e
		}
		target = gain.Wars[0].GetGuid()
		w, e := p5Warrior(g, target)
		if e != nil {
			return e
		}
		w.Level = proto.Int32(100)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	actualContext := func(request proto.Message) context.Context {
		return context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, request)
	}
	talent := &pb.CSWarriorTalent{Guid: proto.String(target), Talent: proto.String("1"), Cost: []*pb.BaseResource{{Id: proto.String("coin"), Num: proto.Int32(10000)}, {Id: proto.String("item_005"), Num: proto.Int32(30)}}}
	if _, err := f.s.warriorTalent(actualContext(talent), f.id, talent); err != nil {
		t.Fatal(err)
	}
	awake := &pb.CSWarriorAwake{Guid: proto.String(target)}
	for stage := int64(1); stage <= 2; stage++ {
		if _, err := f.s.warriorAwake(actualContext(awake), f.id, awake, stage); err != nil {
			t.Fatal(err)
		}
	}
	player, game := f.profile(t)
	if simpleOwned(player, game, "item_312") != 0 || simpleOwned(player, game, "item_313") != 0 {
		t.Fatal("finite qualification materials were not fully consumed")
	}
	at := time.Now()
	key := p4ExchangeKey(at)
	child := constUtil.P5AwakeMaterialSupplyPrefix + at.In(time.FixedZone("UTC+8", 28800)).Format("200601")
	base := p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, at).(*pb.SCActivityShopInfo)
	var cfg db.P4CommerceRule
	if err := db.GetIns().First(&cfg, "id=?", child).Error; err != nil {
		t.Fatal(err)
	}
	var rules p4CommerceRules
	if json.Unmarshal(cfg.Rules, &rules) != nil || len(rules.Products) != 1 || cfg.Enabled || len(cfg.Frozen) != 0 {
		t.Fatal("choice source not defaultoff/unfrozen")
	}
	product := rules.Products[0]
	if product.ID != "item_330" || product.Cost != "coin,1000000" || product.MinLevel != 70 || product.DailyLimit != 1 || product.TotalLimit != 30 {
		t.Fatal("local default or original level gate changed", product)
	}
	// One finite prepublished operator variant permits all three original choices
	// in this request. The production default remains1box/day and30/month.
	rules.Products[0].DailyLimit = 3
	raw, _ := json.Marshal(rules)
	if err := db.GetIns().Model(&cfg).Updates(map[string]any{"enabled": true, "rules": raw}).Error; err != nil {
		t.Fatal(err)
	}
	listing := p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, at).(*pb.SCActivityShopInfo)
	if len(listing.ShopItemList) != len(base.ShopItemList)+1 {
		t.Fatal("choice source did not append once")
	}
	for i, old := range base.ShopItemList {
		if !proto.Equal(old, listing.ShopItemList[i]) {
			t.Fatal("old source moved/changed", i)
		}
	}
	item := listing.ShopItemList[len(base.ShopItemList)]
	request := &pb.CSBuyActivityShop{ActivityId: proto.String(key), Index: item.Index, Num: proto.Int32(3), CostResources: item.CostResource, GetResources: item.GetResource}
	beforeCoin := player.GetCoin()
	p4Trade(t, f, request, 31001, at)
	player, game = f.profile(t)
	if beforeCoin-player.GetCoin() != 3000000 || simpleOwned(player, game, "item_330") != 3 {
		t.Fatal("actual coin/box transaction")
	}
	invalid := &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_330"), Num: proto.Int32(1)}, GetItemIndex: proto.Int32(4)}
	if _, err := f.s.p5BoxUse(f.ctx, f.id, invalid, 31002); err == nil {
		t.Fatal("non-original choice accepted")
	}
	wanted := []*pb.BaseResource{{Id: proto.String("item_312"), Num: proto.Int32(2)}, {Id: proto.String("item_315"), Num: proto.Int32(8)}, {Id: proto.String("item_313"), Num: proto.Int32(8)}}
	for i, resource := range wanted {
		request := &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_330"), Num: proto.Int32(1)}, GetItemIndex: proto.Int32(int32(i + 1))}
		out, err := f.s.p5BoxUse(f.ctx, f.id, request, int64(31010+i))
		if err != nil {
			t.Fatal(err)
		}
		gain := out[0].(*pb.SCItemUse).GetGet()
		if len(gain.Simps) != 1 || gain.Simps[0].GetId() != resource.GetId() || gain.Simps[0].GetNum() != resource.GetNum() {
			t.Fatal("original selected amount changed", i, gain)
		}
	}
	player, game = f.profile(t)
	if simpleOwned(player, game, "item_330") != 0 || simpleOwned(player, game, "item_312") != 2 || simpleOwned(player, game, "item_315") != 8 || simpleOwned(player, game, "item_313") != 8 {
		t.Fatal("original selection inventory")
	}
	fuse := &pb.CSAwakeFuse{Guid: proto.String(target), Cost: []*pb.BaseResource{{Id: proto.String("item_312"), Num: proto.Int32(1)}}}
	out, err := f.s.warriorAwakeFuse(actualContext(fuse), f.id, fuse, 31100)
	if err != nil {
		t.Fatal(err)
	}
	current := out[0].(*pb.SCAwakeFuse).GetNewWarrior()
	if current.GetGuid() != target || current.GetAwake().GetStage() != 2 || current.GetAwake().GetFuseLevel() <= 0 {
		t.Fatal("actual book did not reach original fusion consumer", current)
	}
	player, game = f.profile(t)
	if simpleOwned(player, game, "item_312") != 1 {
		t.Fatal("fusion source book was not consumed exactly1")
	}
}
