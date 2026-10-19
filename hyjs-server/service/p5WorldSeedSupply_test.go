package service

import (
	"bytes"
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

func TestP5WorldSeedSupplyActualPurchaseNodeCostsFrozenAndCold(t *testing.T) {
	f := p4Setup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP5(f.ctx); err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	key := p4ExchangeKey(at)
	child := constUtil.P5WorldSeedSupplyPrefix + at.In(time.FixedZone("UTC+8", 28800)).Format("200601")
	base := p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, at).(*pb.SCActivityShopInfo)
	var cfg db.P4CommerceRule
	if err := db.GetIns().First(&cfg, "id=?", child).Error; err != nil {
		t.Fatal(err)
	}
	if cfg.Enabled || len(cfg.Frozen) != 0 || cfg.Source != constUtil.P5WorldSeedSupplySource {
		t.Fatal("unpublished source granted/froze itself", cfg)
	}
	var rules p4CommerceRules
	if json.Unmarshal(cfg.Rules, &rules) != nil || len(rules.Products) != 1 {
		t.Fatal("source catalog")
	}
	item := rules.Products[0]
	if item.ID != "item_017" || item.Rewards != "item_017,1" || item.Cost != "coin,500000" || item.MinLevel != 25 || item.DailyLimit != 3 || item.TotalLimit != 60 {
		t.Fatal("declared local defaults", item)
	}
	if err := db.GetIns().Model(&cfg).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	// These are finite isolated level/capital prerequisites only. Native acceptance
	// uses actual original star-chest coins and keeps the same publication source.
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(24)
		p.Coin = proto.Int64(2000000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	listing := p4Trade(t, f, &pb.CSActivityShopInfo{ActivityId: proto.String(key)}, 0, at).(*pb.SCActivityShopInfo)
	if len(listing.ShopItemList) != len(base.ShopItemList)+1 {
		t.Fatal("new source did not append exactly one address")
	}
	for i, old := range base.ShopItemList {
		// The isolated player has advanced from level1 to24 since the first
		// query, so the original eligibility status legitimately changes.
		expected := proto.Clone(old).(*pb.ActivityShopItem)
		expected.Status = listing.ShopItemList[i].Status
		if !proto.Equal(expected, listing.ShopItemList[i]) {
			t.Fatal("existing product/index changed", i)
		}
	}
	added := listing.ShopItemList[len(base.ShopItemList)]
	if added.GetName() != "世界之种" || added.GetGetResource()[0].GetId() != "item_017" {
		t.Fatal("original display/inventory identity", added)
	}
	request := &pb.CSBuyActivityShop{ActivityId: proto.String(key), Index: added.Index, Num: proto.Int32(3), CostResources: added.CostResource, GetResources: added.GetResource}
	if _, err := f.s.p4Commerce(f.ctx, f.id, request, 29101, at); err == nil {
		t.Fatal("local level25 not enforced")
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(25); return nil }); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	replies := make([]proto.Message, 8)
	failures := make([]error, 8)
	for i := range replies {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			out, err := f.s.p4Commerce(f.ctx, f.id, request, 29101, at)
			failures[i] = err
			if err == nil {
				replies[i] = out[0]
			}
		}(i)
	}
	group.Wait()
	for i, err := range failures {
		if err != nil || !proto.Equal(replies[0], replies[i]) {
			t.Fatal("same original purchase not once", i, err)
		}
	}
	player, game := f.profile(t)
	if player.GetCoin() != 500000 || simpleOwned(player, game, "item_017") != 3 {
		t.Fatal("source currency/material conservation", player.GetCoin(), game.Items)
	}
	if _, err := f.s.p4Commerce(f.ctx, f.id, &pb.CSBuyActivityShop{ActivityId: request.ActivityId, Index: request.Index, Num: proto.Int32(1), CostResources: request.CostResources, GetResources: request.GetResources}, 29102, at); err == nil {
		t.Fatal("daily3 not enforced")
	}
	mainGUID := player.GetHeroWarId()
	beforeBattle, err := f.s.battlePlayer(player, game)
	if err != nil {
		t.Fatal(err)
	}
	for node := int32(2); node <= 4; node++ {
		out, e := f.s.lightenWorldSeed(f.ctx, f.id, &pb.CSLightenWorldSeed{Id: proto.Int32(node - 1)}, int64(29200+node))
		if e != nil {
			t.Fatal(e)
		}
		got := out[0].(*pb.SCLightenWorldSeed)
		if !p1SameResources(got.Cost, []*pb.BaseResource{{Id: proto.String("item_017"), Num: proto.Int32(1)}}) {
			t.Fatal("original node1 cost changed", got)
		}
	}
	player, game = f.profile(t)
	if player.GetCurrentWorldSeedId() != 4 || simpleOwned(player, game, "item_017") != 0 || player.GetHeroWarId() != mainGUID || player.GetCoin() != 500000 {
		t.Fatal("source not used by original nodes", player, game.Items)
	}
	afterBattle, err := f.s.battlePlayer(player, game)
	if err != nil {
		t.Fatal(err)
	}
	a, b := beforeBattle.Formation[0].GetProp(), afterBattle.Formation[0].GetProp()
	if b.GetMdef()-a.GetMdef() != 10 || b.GetHp()-a.GetHp() != 170 || b.GetAtt()-a.GetAtt() != 15 {
		t.Fatal("original node properties missing", a, b)
	}
	// Current operator edits do not change the already published price/quantity.
	rules.Products[0].Cost = "coin,1"
	rules.Products[0].Rewards = "item_017,99"
	raw, _ := json.Marshal(rules)
	if err = db.GetIns().Model(&cfg).Updates(map[string]any{"rules": raw, "enabled": false}).Error; err != nil {
		t.Fatal(err)
	}
	stable, _ := json.Marshal([]any{player, game})
	connection, _ := db.GetIns().DB()
	connection.Close()
	f.open(t)
	if err = f.s.InitializeP4(f.ctx); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(replies[0], p4Trade(t, f, request, 29101, at)) {
		t.Fatal("cold source replay changed")
	}
	player, game = f.profile(t)
	again, _ := json.Marshal([]any{player, game})
	if !bytes.Equal(stable, again) {
		t.Fatal("cold source retry duplicated coin/material or reset nodes")
	}
	var saved db.P4CommerceRule
	if err = db.GetIns().First(&saved, "id=?", child).Error; err != nil || saved.Enabled || len(saved.Frozen) == 0 {
		t.Fatal("operator policy not retained", err)
	}
}
