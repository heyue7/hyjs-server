package service

import (
	"context"
	"encoding/json"
	"reflect"
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

// localPetGrowthClockFixture keeps the finite domain prerequisites explicit.
// Level75, 100 original pet002 fragments and 500 diamonds are isolated test
// inventory, not an assertion of natural acquisition or a payment source.
func localPetGrowthClockFixture(t *testing.T) (*p1Fixture, int64, *db.Profile, context.Context) {
	t.Helper()
	f, id, owner := localClockFixture(t)
	if err := f.s.LoadP9Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{
		f.s.InitializeP5,
		f.s.InitializeP9Pets,
		f.s.InitializeP9PetNext,
		f.s.InitializeP9PetRecycle,
		f.s.InitializeP9CopyIntimacy,
		f.s.InitializeP9PetIntimacy,
	} {
		if err := initialize(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if !f.s.P1Ready {
		t.Fatal("P1 event recording must remain enabled for this regression")
	}
	if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(75)
		p.PetBagSize = proto.Int32(100)
		_, err := f.s.GrantFixedReward(p, g, "frag_pet_002,100;diamond,500")
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// Publish a finite original fragment plus material catalogue. Six slots retain
	// the production 1-4 fragment split, while every material slot is item124.
	var rule db.P9PetMarketRule
	if err := db.GetIns().First(&rule, "id=?", constUtil.P9MarketRuleID).Error; err != nil {
		t.Fatal(err)
	}
	priorInput := append([]byte(nil), rule.Input...)
	var cfg p9MarketConfig
	if err := json.Unmarshal(rule.Input, &cfg); err != nil {
		t.Fatal(err)
	}
	var selected []p9MarketOffer
	for _, offer := range cfg.Offers {
		if offer.Resource == "item_124" {
			selected = append(selected, offer)
		}
	}
	if len(selected) != 1 || selected[0].Currency != "coinpet" || selected[0].Price != 20 || selected[0].Amount != 10 || cfg.RefreshCost != "diamond,150" || cfg.RefreshCap != 10 {
		t.Fatal("original market fixture prerequisites changed", selected, cfg.RefreshCost, cfg.RefreshCap)
	}
	for _, offer := range cfg.Offers {
		if offer.Resource == "frag_pet_002" {
			selected = append(selected, offer)
			break
		}
	}
	if len(selected) != 2 {
		t.Fatal("original fragment offer missing")
	}
	cfg.Offers = selected
	cfg.Slots = 6
	input, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.P9PetMarketRule{}).Where("id=?", rule.ID).Updates(map[string]any{"input": input, "enabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.GetIns().Model(&db.P9PetMarketRule{}).Where("id=?", rule.ID).Updates(map[string]any{"input": priorInput, "enabled": rule.Enabled}).Error; err != nil {
			t.Error("restore finite market catalogue", err)
		}
		if err := db.GetIns().Model(&db.LocalPlayPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
			t.Error("restore default-off clock policy", err)
		}
	})

	localClockEnable(t)
	ctx := localClockBind(t, f, owner)
	view, err := f.s.LocalPlayTimeChange(ctx, id, "previousDay", "pet-clock-previous-day")
	if err != nil || view.OffsetDays != -1 {
		t.Fatal("actual owner previousDay action", view, err)
	}
	ctx = localClockBind(t, f, owner)
	if LocalPlayAt(ctx, time.Unix(1700000000, 0)).Unix() != time.Unix(1700000000, 0).AddDate(0, 0, -1).Unix() {
		t.Fatal("rebound owner did not load the persisted previous-day state")
	}
	return f, id, owner, ctx
}

// Retry the original same-stamp transaction concurrently; check its original
// primary response and inspect committed resources separately at each step.
func localPetGrowthClockEight(t *testing.T, call func() ([]proto.Message, error)) proto.Message {
	t.Helper()
	var workers sync.WaitGroup
	replies := make(chan proto.Message, 8)
	errors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			out, err := call()
			if err != nil {
				errors <- err
				return
			}
			if len(out) == 0 || out[0] == nil {
				errors <- p5Error()
				return
			}
			replies <- out[0]
		}()
	}
	workers.Wait()
	close(errors)
	close(replies)
	for err := range errors {
		t.Fatal("previous-day real transaction rolled back", err)
	}
	var first proto.Message
	var count int
	for response := range replies {
		count++
		if first == nil {
			first = response
		} else if !proto.Equal(first, response) {
			t.Fatal("same-stamp original response changed")
		}
	}
	if count != 8 {
		t.Fatal("not all same-stamp requests returned", count)
	}
	return first
}

func localPetGrowthClockFact(t *testing.T, id int64, source, kind string, amount int64, before, after time.Time) db.EventFact {
	t.Helper()
	var facts []db.EventFact
	if err := db.GetIns().Where("player_id=? AND source_key=? AND kind=?", id, source, kind).Find(&facts).Error; err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatal("same-stamp committed event count", source, kind, len(facts))
	}
	fact := facts[0]
	zone := time.FixedZone("UTC+8", 8*3600)
	beforeDay := before.In(zone).Format("2006-01-02")
	afterDay := after.In(zone).Format("2006-01-02")
	if fact.Amount != amount || fact.OccurredAt < before.UnixMilli() || fact.OccurredAt > after.UnixMilli() ||
		(fact.Day != beforeDay && fact.Day != afterDay) || fact.Day != time.UnixMilli(fact.OccurredAt).In(zone).Format("2006-01-02") ||
		fact.OccurredAt >= time.Now().Add(-23*time.Hour).UnixMilli() {
		t.Fatal("event did not use the actual bound personal date", fact, before, after)
	}
	return fact
}

func TestLocalPlayPetGrowthClockPreviousDayLifecycleAndMarket(t *testing.T) {
	f, id, owner, ctx := localPetGrowthClockFixture(t)
	before, gameBefore := f.profile(t)
	if simpleOwned(before, gameBefore, "frag_pet_002") != 100 {
		t.Fatal("finite original fragment prerequisite")
	}
	started := LocalPlayNow(ctx)

	// Preserve the future-event guard: the real wall clock remains a future
	// business event when the authenticated owner's play date is yesterday.
	err := f.s.p3Profile(ctx, id, func(tx *gorm.DB, _ *db.Profile, _ *pb.Player, _ *model.GameState) error {
		return f.s.RecordP1Event(tx, id, "pet-clock-wall-time-negative", "pet_decompose", 1, time.Now())
	})
	if err == nil {
		t.Fatal("real future timestamp was silently accepted for a previous-day owner")
	}
	var rejected int64
	if err = db.GetIns().Model(&db.EventFact{}).Where("player_id=? AND source_key=?", id, "pet-clock-wall-time-negative").Count(&rejected).Error; err != nil || rejected != 0 {
		t.Fatal("rejected future event left a committed fact", rejected, err)
	}

	combine := &pb.CSCombine{TargetId: proto.String("pet_002")}
	combined := localPetGrowthClockEight(t, func() ([]proto.Message, error) {
		return f.s.p9PetCombine(ctx, id, combine, 91001)
	}).(*pb.SCCombine)
	if len(combined.GetGet().GetPets()) != 1 || len(combined.GetCost()) != 1 || combined.GetCost()[0].GetNum() != 50 {
		t.Fatal("original fragment combine response", combined)
	}
	pet := combined.GetGet().GetPets()[0]
	if pet.GetId() != "pet_002" || pet.GetGuid() == "" {
		t.Fatal("combine did not create the actual original pet GUID", pet)
	}
	owned, err := P9PetList(db.GetIns(), id)
	if err != nil || len(owned) != 1 || !proto.Equal(owned[0], pet) {
		t.Fatal("combined GUID did not persist", owned, err)
	}
	p, game := f.profile(t)
	if simpleOwned(p, game, "frag_pet_002") != 50 || p.GetCoinpet() != before.GetCoinpet() {
		t.Fatal("combine replay duplicated fragment consumption or currency")
	}
	combineFact := localPetGrowthClockFact(t, id, "p9:fragment-combine:91001", "pet_fragment_consumed", 50, started, LocalPlayNow(ctx))

	// Reopen the real store before consuming this newly created GUID. The
	// still-current combine receipt must survive a cold load unchanged.
	f.restart(t)
	ctx = localClockBind(t, f, owner)
	replay, err := f.s.p9PetCombine(ctx, id, combine, 91001)
	if err != nil || !proto.Equal(replay[0], combined) {
		t.Fatal("cold current combine receipt", err)
	}
	decompose := &pb.CSPetDecompose{Guids: []string{pet.GetGuid()}}
	decomposed := localPetGrowthClockEight(t, func() ([]proto.Message, error) {
		return f.s.p9PetDecompose(ctx, id, decompose, 91002)
	}).(*pb.SCPetDecompose)
	p, game = f.profile(t)
	if p.GetCoinpet() != before.GetCoinpet()+100 || simpleOwned(p, game, "frag_pet_002") != 50 {
		t.Fatal("original decompose coinpet or replay cost changed", p.GetCoinpet())
	}
	var retired db.P9PetRetired
	if err = db.GetIns().First(&retired, "player_id=? AND guid=?", id, pet.GetGuid()).Error; err != nil || retired.Coinpet != 100 {
		t.Fatal("original GUID retirement and coinpet ledger", retired, err)
	}
	saved := &pb.Pet{}
	if err = proto.Unmarshal(retired.Data, saved); err != nil || !proto.Equal(saved, pet) {
		t.Fatal("retirement lost the actual combined native pet", err)
	}
	owned, err = P9PetList(db.GetIns(), id)
	if err != nil || len(owned) != 0 {
		t.Fatal("decompose retained or duplicated a live GUID", owned, err)
	}
	decomposeFact := localPetGrowthClockFact(t, id, "p9:decompose:91002", "pet_decompose", 1, started, LocalPlayNow(ctx))
	if _, err = f.s.p9PetCombine(ctx, id, combine, 91001); err == nil {
		t.Fatal("old combine response resurrected a retired GUID")
	}

	sell := &pb.CSPetFragSell{Sells: []*pb.BaseResource{{Id: proto.String("frag_pet_002"), Num: proto.Int32(5)}}}
	localPetGrowthClockEight(t, func() ([]proto.Message, error) {
		return f.s.p9PetFragSell(ctx, id, sell, 91003)
	})
	p, game = f.profile(t)
	if simpleOwned(p, game, "frag_pet_002") != 45 || p.GetCoin() != before.GetCoin()+10 || p.GetCoinpet() != before.GetCoinpet()+100 {
		t.Fatal("original fragment sale currency/cost or same-stamp replay")
	}
	sellFact := localPetGrowthClockFact(t, id, "p9:fragment-sell:91003", "pet_fragment_consumed", 5, started, LocalPlayNow(ctx))

	listing, err := f.s.p9PetShopList(ctx, id)
	if err != nil {
		t.Fatal("previous-day market list", err)
	}
	items := listing[0].(*pb.SCPetShopProductList).GetShopItemList()
	var material *pb.ShopItem
	for _, item := range items {
		if len(item.GetItem()) == 1 && item.GetItem()[0].GetId() == "item_124" {
			material = item
			break
		}
	}
	if len(items) != 6 || material == nil {
		t.Fatal("finite original market item", items)
	}
	buy := &pb.CSPetShopBuy{ShopItemId: material.Id, BuyCount: proto.Int32(1)}
	localPetGrowthClockEight(t, func() ([]proto.Message, error) {
		return f.s.p9PetShopBuy(ctx, id, buy, 91004)
	})
	p, game = f.profile(t)
	if p.GetCoinpet() != before.GetCoinpet()+80 || simpleOwned(p, game, "item_124") != simpleOwned(before, gameBefore, "item_124")+10 {
		t.Fatal("previous-day purchase did not commit exactly one original offer")
	}
	buyFact := localPetGrowthClockFact(t, id, "p9:shop-buy:91004", "spend:coinpet", 20, started, LocalPlayNow(ctx))
	refresh := &pb.CSRefreshPetShop{}
	refreshed := localPetGrowthClockEight(t, func() ([]proto.Message, error) {
		return f.s.p9PetShopRefresh(ctx, id, refresh, 91005)
	}).(*pb.SCPetShopProductList)
	p, game = f.profile(t)
	if p.GetDiamond() != before.GetDiamond()-150 || refreshed.GetRefreshTimes() != 9 || len(refreshed.GetShopItemList()) != 6 || refreshed.GetShopItemList()[0].GetId() == items[0].GetId() {
		t.Fatal("original150 refresh fee/epoch/replay", refreshed, p.GetDiamond())
	}
	refreshFact := localPetGrowthClockFact(t, id, "p9:shop-refresh:91005", constUtil.P1EventDiamondSpent, 150, started, LocalPlayNow(ctx))
	if _, err = f.s.p9PetShopBuy(ctx, id, buy, 91004); err == nil {
		t.Fatal("old market receipt overwrote the refreshed epoch")
	}

	// Reset through the actual tool and reopen the store. Old mutation receipts
	// retain yesterday's committed fact; they must not charge or redate it.
	stablePlayer := proto.Clone(p).(*pb.Player)
	stableGame := game
	view, err := f.s.LocalPlayTimeChange(ctx, id, "reset", "pet-clock-reset")
	if err != nil || view.OffsetDays != 0 {
		t.Fatal("actual owner reset", view, err)
	}
	f.restart(t)
	ctx = localClockBind(t, f, owner)
	if current, err := f.s.LocalPlayTimeState(ctx, id); err != nil || current.OffsetDays != 0 {
		t.Fatal("cold actual calendar reset", current, err)
	}
	if replay, err = f.s.p9PetDecompose(ctx, id, decompose, 91002); err != nil || !proto.Equal(replay[0], decomposed) {
		t.Fatal("cold retired GUID receipt", err)
	}
	if replay, err = f.s.p9PetShopRefresh(ctx, id, refresh, 91005); err != nil || !proto.Equal(replay[0], refreshed) {
		t.Fatal("cold current refresh receipt", err)
	}
	after, afterGame := f.profile(t)
	if !proto.Equal(stablePlayer, after) || !reflect.DeepEqual(stableGame, afterGame) {
		t.Fatal("calendar reset/cold receipts changed actual inventory or wallet")
	}
	for _, prior := range []db.EventFact{combineFact, decomposeFact, sellFact, buyFact, refreshFact} {
		var facts []db.EventFact
		if err = db.GetIns().Where("player_id=? AND source_key=? AND kind=?", id, prior.SourceKey, prior.Kind).Find(&facts).Error; err != nil || len(facts) != 1 || !reflect.DeepEqual(prior, facts[0]) {
			t.Fatal("old receipt changed its personal event date/amount or duplicated a fact", prior, facts, err)
		}
	}
}
