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

// TestP4GuildShopOriginalDualCost uses finite domain balances; the original native chain obtains both costs.
func TestP4GuildShopOriginalDualCost(t *testing.T) {
	f := p4Setup(t)
	if err := f.s.LoadP13GuildTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP13Guild(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{&db.P13Guild{ID: "p4-guild-one", Name: "商店有限公会", Level: 1}, &db.P13GuildMember{PlayerID: f.id, GuildID: "p4-guild-one", Contribution: 20}} {
		if err := db.GetIns().Create(value).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P13GuildPolicy{}).Where("id=1").Updates(map[string]any{"management_enabled": true, "economy_enabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(30)
		p.Contribution = proto.Int32(20)
		g.Items = nil
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	key := constUtil.P4GuildShopPrefix + now.In(time.FixedZone("UTC+8", 8*3600)).Format("200601")
	query := &pb.CSShopProductList{ShopType: proto.Int32(8)}
	if _, err := f.s.p4GuildShop(f.ctx, f.id, query, 0, now); err == nil {
		t.Fatal("default-off source accepted")
	}
	var cfg db.P4CommerceRule
	if err := db.GetIns().First(&cfg, "id=?", key).Error; err != nil || cfg.Enabled || len(cfg.Frozen) > 0 {
		t.Fatal("default-off source must stay editable", err)
	}
	if err := db.GetIns().Model(&cfg).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	listed, err := f.s.p4GuildShop(f.ctx, f.id, query, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	list := listed[0].(*pb.SCShopProductList)
	if list.GetContribution() != 20 || len(list.GetShopItemList()) != 1 {
		t.Fatal("original guild shop balance/catalog")
	}
	item := list.GetShopItemList()[0]
	if item.GetCondition().GetValue() != 1 || item.GetExchangeList()[0].GetId() != "item_311" || item.GetExchangeList()[1].GetId() != "contribution" || item.GetCanBuy() != 1 {
		t.Fatal("original costType3 and guild-level projection", item)
	}
	buy := &pb.CSShopBuy{ShopType: proto.Int32(8), ShopItemId: item.Id, BuyCount: proto.Int32(1)}
	before, gameBefore := f.profile(t)
	if _, err := f.s.p4GuildShop(f.ctx, f.id, buy, 9101, now); err == nil {
		t.Fatal("unfunded badge source accepted")
	}
	after, gameAfter := f.profile(t)
	if !proto.Equal(before, after) || len(gameBefore.Items) != len(gameAfter.Items) {
		t.Fatal("failed badge purchase deducted contribution")
	}
	if err := updateProfile(f.ctx, f.id, func(_ *pb.Player, g *model.GameState) error {
		g.Items = []*pb.SimpleItem{{Id: proto.String("item_311"), Num: proto.Int32(1)}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	bad := proto.Clone(buy).(*pb.CSShopBuy)
	bad.BuyCount = proto.Int32(2)
	if _, err = f.s.p4GuildShop(f.ctx, f.id, bad, 9101, now); err == nil {
		t.Fatal("original fixed-one buy changed")
	}
	// Concurrent HTTP requests must observe the receipt committed by the first gate holder.
	var wg sync.WaitGroup
	replies := make(chan *pb.SCShopBuy, 8)
	errors := make(chan error, 8)
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, callErr := f.s.p4GuildShop(f.ctx, f.id, buy, 9101, now)
			errors <- callErr
			if callErr == nil {
				replies <- result[0].(*pb.SCShopBuy)
			}
		}()
	}
	wg.Wait()
	close(errors)
	close(replies)
	for callErr := range errors {
		if callErr != nil {
			t.Fatal(callErr)
		}
	}
	var reply *pb.SCShopBuy
	for result := range replies {
		if reply == nil {
			reply = result
		} else if !proto.Equal(reply, result) {
			t.Fatal("concurrent committed original response changed")
		}
	}
	if reply.GetContribution() != 19 || len(reply.GetCost()) != 2 || reply.GetCost()[0].GetId() != "item_311" || reply.GetCost()[1].GetId() != "contribution" {
		t.Fatal("original callback requires ordered costs", reply)
	}
	for n := 0; n < 3; n++ {
		repeated, err := f.s.p4GuildShop(f.ctx, f.id, buy, 9101, now)
		if err != nil || !proto.Equal(reply, repeated[0]) {
			t.Fatal("committed transaction recovery", err)
		}
	}
	player, state := f.profile(t)
	var member db.P13GuildMember
	if err := db.GetIns().First(&member, f.id).Error; err != nil || member.Contribution != 20 || player.GetContribution() != 19 || simpleOwned(player, state, "item_311") != 0 || simpleOwned(player, state, "item_005") != 50 {
		t.Fatal("two true balances and cumulative contribution", err)
	}
	if _, err = f.s.p4GuildShop(f.ctx, f.id, buy, 9102, now); err == nil {
		t.Fatal("daily quota reused")
	}
	// Only a finite domain guild switch is prepared; the historical purchase remains owned by its real guild.
	if err = db.GetIns().Create(&db.P13Guild{ID: "p4-guild-two", Name: "第二有限公会", Level: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&member).Update("guild_id", "p4-guild-two").Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p4GuildShop(f.ctx, f.id, buy, 9101, now); err == nil {
		t.Fatal("old purchase crossed guild")
	}
	var raw p4GuildRules
	if err = json.Unmarshal(cfg.Rules, &raw); err != nil || raw.Products[0].ContributionCost != 1 || raw.Products[0].GuildLevel != 1 {
		t.Fatal("dual costs remain explicit configurable SQL", err)
	}
}
