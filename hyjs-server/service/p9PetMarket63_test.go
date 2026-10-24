package service

import (
	"encoding/json"
	"hyjs-server/db"
	"testing"
)

func TestPetMarket63SlotInventory(t *testing.T) {
	s := &GameService{}
	cfg := &p9MarketConfig{Slots: 6, Offers: []p9MarketOffer{{ID: "fragment", Resource: "frag_pet_001", Amount: 1, Currency: "coinpet", Price: 10, Stock: 1, Weight: 1}, {ID: "material", Resource: "item_124", Amount: 1, Currency: "coinpet", Price: 20, Stock: 1, Weight: 1}}}
	state := &db.P9PetMarketState{WeekKey: "20261019"}
	if err := s.p9MarketSelect(state, cfg); err != nil {
		t.Fatal(err)
	}
	selected, bought, err := s.p9MarketInventory(state, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 6 {
		t.Fatalf("slots=%d", len(selected))
	}
	fragmentCount := 0
	seen := map[string]bool{}
	for _, offer := range selected {
		key := p9MarketOfferKey(offer)
		if offer.SlotID == "" || seen[key] {
			t.Fatal("slot identity collision")
		}
		seen[key] = true
		if offer.ID == "fragment" {
			fragmentCount++
		}
	}
	if fragmentCount < 1 || fragmentCount > 4 {
		t.Fatalf("fragments=%d", fragmentCount)
	}
	// Two material slots necessarily repeat the product and keep distinct quotes and stock.
	first, second := selected[4], selected[5]
	if first.ID != second.ID || p9MarketQuoteSKU(state, first) == p9MarketQuoteSKU(state, second) {
		t.Fatal("repeated material lost slot identity")
	}
	bought[p9MarketOfferKey(first)] = 1
	state.Bought, _ = json.Marshal(bought)
	list, err := s.p9MarketListing(state, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if list.ShopItemList[4].GetCanBuy() != 0 || list.ShopItemList[5].GetCanBuy() != 1 {
		t.Fatal("purchase affected sibling slot")
	}
	rawCfg, _ := json.Marshal(cfg)
	var nextCfg p9MarketConfig
	json.Unmarshal(rawCfg, &nextCfg)
	nextCfg.Offers[1].Price = 30
	list, err = s.p9MarketListing(state, cfg)
	if err != nil || list.ShopItemList[5].Price[0].GetNum() != 20 {
		t.Fatal("accepted quote changed", err)
	}
	cfg = &nextCfg
	old := state.Generation
	if err := s.p9MarketSelect(state, cfg); err != nil {
		t.Fatal(err)
	}
	if state.Generation != old+1 || string(state.Bought) != "{}" {
		t.Fatal("refresh did not start new slot epoch")
	}
}

func TestPetMarket63LegacyInventory(t *testing.T) {
	s := &GameService{}
	offer := p9MarketOffer{ID: "old", Resource: "item_124", Amount: 1, Currency: "coinpet", Price: 20, Stock: 1, Weight: 1}
	cfg := &p9MarketConfig{Slots: 6, Offers: []p9MarketOffer{offer}}
	raw, _ := json.Marshal([]p9MarketOffer{offer})
	state := &db.P9PetMarketState{Selected: raw, SelectedHash: p6Digest(raw), Bought: []byte(`{"old":1}`), WeekKey: "20261019", Generation: 3}
	list, err := s.p9MarketListing(state, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if list.ShopItemList[0].GetCanBuy() != 0 || p9MarketOfferKey(offer) != "old" {
		t.Fatal("legacy stock was reset")
	}
	frozen, _ := json.Marshal(state)
	var restored db.P9PetMarketState
	json.Unmarshal(frozen, &restored)
	list, err = s.p9MarketListing(&restored, cfg)
	if err != nil || list.ShopItemList[0].Price[0].GetNum() != 20 || list.ShopItemList[0].GetCanBuy() != 0 {
		t.Fatal("legacy cold quote changed", err)
	}
}
