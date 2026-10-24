package service

import (
	"context"
	"encoding/json"
	"math"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestP9MarketAndDecomposeLifecycle isolates eligibility and operator catalogue;
// actual items/currency, refunds, and lifetime receipts remain real transactions.
func TestP9MarketAndDecomposeLifecycle(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "p9-market.db")
	if err := db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	defer func() { pool, _ := db.GetIns().DB(); pool.Close() }()
	s := &GameService{}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP9Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func(context.Context) error{s.InitializeP1, s.InitializeP9Pets, s.InitializeP9PetNext, s.InitializeP9PetRecycle} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var live db.P9PetMarketRule
	if err := db.GetIns().First(&live, "id=?", constUtil.P9MarketRuleID).Error; err != nil {
		t.Fatal(err)
	}
	var cfg p9MarketConfig
	json.Unmarshal(live.Input, &cfg)
	fragments := 0
	for _, offer := range cfg.Offers {
		if len(offer.Resource) > 9 && offer.Resource[:9] == "frag_pet_" {
			fragments++
		}
	}
	if fragments != 78 || len(cfg.Offers) != 112 || cfg.RefreshCost != "diamond,150" || cfg.RefreshCap != 10 {
		t.Fatal("original full catalogue/refresh parameters", fragments, len(cfg.Offers))
	}
	// Exact original products selected as a finite operator window, not fabricated rewards.
	selected := []p9MarketOffer{}
	for _, offer := range cfg.Offers {
		if offer.Resource == "item_124" || offer.Resource == "frag_pet_001" {
			selected = append(selected, offer)
		}
	}
	cfg.Offers = selected
	cfg.Slots = 6
	input, _ := json.Marshal(cfg)
	// 此隔离用例显式发布两种原商品的六槽有放回窗口，生产默认关闭保持不变。
	if err := db.GetIns().Model(&live).Updates(map[string]interface{}{"input": input, "enabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	a, err := LoginAccount(ctx, "p9-market", "isolated-only")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreatePlayer(ctx, a.Id, "精灵币商店链", "hero_043"); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(ctx, a.Id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(75)
		p.Coin = proto.Int64(1000000)
		p.Diamond = proto.Int64(5000)
		p.PetBagSize = proto.Int32(100)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	read := func() (*pb.Player, *model.GameState) {
		t.Helper()
		var row db.Profile
		if err := db.GetIns().First(&row, a.Id).Error; err != nil {
			t.Fatal(err)
		}
		p, g, e := decodeProfile(&row)
		if e != nil {
			t.Fatal(e)
		}
		return p, g
	}
	call := func(request proto.Message, stamp int64) proto.Message {
		t.Helper()
		out, handled, err := s.DispatchP9PetRecycle(ctx, a.Id, request, stamp)
		if err != nil || !handled {
			t.Fatal(request, err)
		}
		return out[0]
	}
	first, err := s.p9PetFirst(ctx, a.Id, 1, &pb.CSPetFirstRecruit{})
	if err != nil {
		t.Fatal(err)
	}
	pet := first[0].(*pb.SCPetFirstRecruit).GetGet().GetPets()[0]
	decompose := &pb.CSPetDecompose{Guids: []string{pet.GetGuid()}}
	var wg sync.WaitGroup
	replies := make(chan proto.Message, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, e := s.p9PetDecompose(ctx, a.Id, decompose, 2)
			errs <- e
			if e == nil {
				replies <- out[0]
			}
		}()
	}
	wg.Wait()
	close(replies)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var decomposed proto.Message
	for r := range replies {
		if decomposed == nil {
			decomposed = r
		}
		if !proto.Equal(r, decomposed) {
			t.Fatal("decompose rerolled or duplicated")
		}
	}
	p, g := read()
	if p.GetCoinpet() != 100 {
		t.Fatal("original coinpet currency did not arrive", p.GetCoinpet())
	}
	var retired db.P9PetRetired
	if err := db.GetIns().First(&retired, "guid=?", pet.GetGuid()).Error; err != nil || retired.Coinpet != 100 || retired.Coin != 0 {
		t.Fatal("currency audit confused with coin", err)
	}
	listing := call(&pb.CSPetShopProductList{}, 3).(*pb.SCPetShopProductList)
	if listing.GetRefreshTimes() != 10 || len(listing.GetShopItemList()) != 6 {
		t.Fatal("native list refresh remaining")
	}
	var item *pb.ShopItem
	for _, sku := range listing.GetShopItemList() {
		if sku.GetItem()[0].GetId() == "item_124" {
			item = sku
		}
	}
	buy := &pb.CSPetShopBuy{ShopItemId: item.Id, BuyCount: proto.Int32(1)}
	purchased := call(buy, 4).(*pb.SCPetShopBuy)
	p, g = read()
	if p.GetCoinpet() != 80 || simpleOwned(p, g, "item_124") != 10 || purchased.GetCost()[0].GetId() != "coinpet" {
		t.Fatal("coinpet did not buy actual inventory")
	}
	if _, err := s.p9PetShopBuy(ctx, a.Id, buy, 5); err == nil {
		t.Fatal("sold-out slot sold again")
	}
	var target *pb.Pet
	if err := updateProfileWithTx(ctx, a.Id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		pets, e := s.P9GrantPet(tx, p, "pet_003", 1)
		if e == nil {
			target = pets[0]
		}
		return e
	}); err != nil {
		t.Fatal(err)
	}
	coinBefore := p.GetCoin()
	if _, err := s.p9PetLevelup(ctx, a.Id, &pb.CSPetLevelup{Guid: target.Guid, Cost: []*pb.BaseResource{{Id: proto.String("item_124"), Num: proto.Int32(1)}}}, 6); err != nil {
		t.Fatal(err)
	}
	refunded := call(&pb.CSPetDecompose{Guids: []string{target.GetGuid()}}, 7).(*pb.SCPetDecompose)
	p, g = read()
	if p.GetCoin() != coinBefore || simpleOwned(p, g, "item_124") != 10 || p.GetCoinpet() != 180 || len(refunded.GetGet().GetPets()) != 0 {
		t.Fatal("trained resources not exactly returned")
	}
	refresh := &pb.CSRefreshPetShop{}
	fresh := call(refresh, 8).(*pb.SCPetShopProductList)
	if fresh.GetRefreshTimes() != 9 || fresh.GetShopItemList()[0].GetId() == listing.GetShopItemList()[0].GetId() {
		t.Fatal("refresh did not change stock epoch")
	}
	if _, err := s.p9PetShopBuy(ctx, a.Id, buy, 9); err == nil {
		t.Fatal("stale SKU purchased after refresh")
	}
	if _, err := s.p9PetShopBuy(ctx, a.Id, buy, 4); err == nil {
		t.Fatal("old stock projection overwrote later epoch")
	}
	p, g = read()
	if p.GetDiamond() != 4850 {
		t.Fatal("original150 refresh fee")
	}
	pool, _ := db.GetIns().DB()
	pool.Close()
	if err := db.InitGame(ctx, path); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(call(refresh, 8), fresh) {
		t.Fatal("cold current refresh receipt changed")
	}
	if !proto.Equal(call(decompose, 2), decomposed) {
		t.Fatal("cold retirement receipt changed")
	}
	for i := 0; i < 9; i++ {
		call(refresh, int64(20+i))
	}
	if _, err := s.p9PetShopRefresh(ctx, a.Id, refresh, 30); err == nil {
		t.Fatal("weekly refresh count exceeded")
	}
	p, _ = read()
	if p.GetDiamond() != 3500 {
		t.Fatal("ten original refresh prices not exact")
	}
	// Move only the test's weekly eligibility cursor; no currency or stock is added.
	priorWeek := p9MarketWeek(time.Now().AddDate(0, 0, -7))
	if err := db.GetIns().Model(&db.P9PetMarketState{}).Where("player_id=?", a.Id).Update("week_key", priorWeek).Error; err != nil {
		t.Fatal(err)
	}
	rollover := call(&pb.CSPetShopProductList{}, 31).(*pb.SCPetShopProductList)
	if rollover.GetRefreshTimes() != 10 {
		t.Fatal("week rollover did not restore original refresh allowance")
	}
	if _, err := s.p9PetShopRefresh(ctx, a.Id, refresh, 28); err == nil {
		t.Fatal("old epoch refresh returned obsolete list")
	}
	// Coinpet overflow and unknown historic growth must leave the live instance,
	// original wallet, retirement audit and receipts untouched.
	var protected *pb.Pet
	if err := updateProfileWithTx(ctx, a.Id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		pets, err := s.P9GrantPet(tx, p, "pet_003", 1)
		if err != nil {
			return err
		}
		protected = pets[0]
		p.Coinpet = proto.Int64(math.MaxInt64)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var retiredCount int64
	db.GetIns().Model(&db.P9PetRetired{}).Count(&retiredCount)
	if _, err := s.p9PetDecompose(ctx, a.Id, &pb.CSPetDecompose{Guids: []string{protected.GetGuid()}}, 40); err == nil {
		t.Fatal("coinpet overflow committed")
	}
	p, _ = read()
	if p.GetCoinpet() != math.MaxInt64 {
		t.Fatal("overflow wallet changed")
	}
	var afterCount int64
	db.GetIns().Model(&db.P9PetRetired{}).Count(&afterCount)
	if afterCount != retiredCount {
		t.Fatal("rollback retirement audit changed")
	}
	if _, err := p9OwnedPet(db.GetIns(), a.Id, protected.GetGuid()); err != nil {
		t.Fatal("overflow removed live pet", err)
	}
	protected.Level = proto.Int32(2)
	data, _ := proto.Marshal(protected)
	if err := db.GetIns().Model(&db.P9PetInstance{}).Where("guid=?", protected.GetGuid()).Update("data", data).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(ctx, a.Id, func(p *pb.Player, _ *model.GameState) error { p.Coinpet = proto.Int64(180); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.p9PetDecompose(ctx, a.Id, &pb.CSPetDecompose{Guids: []string{protected.GetGuid()}}, 41); err == nil {
		t.Fatal("unknown old investment fabricated refunds")
	}
	t.Log("full78 fragment catalogue, actual first-pet decomposition→coinpet→shop material→growth→full refund, old GUID retirement, same-frame8 once, original150/weekly10 refresh, stock epochs and cold receipts passed")
}
