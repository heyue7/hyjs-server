package service

import (
	"math"
	"math/big"
	"reflect"
	"testing"

	"hyjs-server/model"
)

func TestGGOTradeConservesAllColorsAndVersions(t *testing.T) {
	price := [6]int64{3, 9, 30, 90, 300, 900}
	a := &model.GGOFieldState{Chips: [6]int64{10, 1, 0, 0, 0, 1}, EconomyVersion: "v1", ExchangeValues: price, Revision: 7}
	b := &model.GGOFieldState{Chips: [6]int64{0, 0, 6, 0, 0, 0}, EconomyVersion: "v1", ExchangeValues: price, Revision: 9}
	beforeA, beforeB := *a, *b
	offers := [2][6]int64{{3, 0, 0, 0, 0, 0}, {0, 0, 2, 0, 0, 0}}
	after, err := ggoTradeAfter([2]*model.GGOFieldState{a, b}, offers, true)
	if err != nil {
		t.Fatal(err)
	}
	for i := range offers[0] {
		if beforeA.Chips[i]+beforeB.Chips[i] != after[0].Chips[i]+after[1].Chips[i] {
			t.Fatal("chip conservation", i)
		}
	}
	if !reflect.DeepEqual(*a, beforeA) || !reflect.DeepEqual(*b, beforeB) || after[0].Revision != 8 || after[1].Revision != 10 {
		t.Fatal("calculation mutated source or missed revision")
	}
	if protected, _, err := ggoProtectedChips(after[0].Chips, 3); err != nil || protected != ([6]int64{0, 0, 2, 0, 0, 1}) {
		t.Fatal("protected chips", protected, err)
	}
	tooMany := offers
	tooMany[0][0] = 11
	if _, err = ggoTradeAfter([2]*model.GGOFieldState{a, b}, tooMany, true); err == nil {
		t.Fatal("unowned chips accepted")
	}
	if _, err = ggoTradeAfter([2]*model.GGOFieldState{a, b}, [2][6]int64{{1}, {}}, false); err == nil {
		t.Fatal("disabled gift accepted")
	}
	b.EconomyVersion = "v2"
	if _, err = ggoTradeAfter([2]*model.GGOFieldState{a, b}, offers, true); err == nil {
		t.Fatal("mixed carried economic versions accepted")
	}
	allA, allB := a.Chips, b.Chips
	swap, err := ggoTradeAfter([2]*model.GGOFieldState{a, b}, [2][6]int64{allA, allB}, true)
	if err != nil || swap[0].EconomyVersion != "v2" || swap[1].EconomyVersion != "v1" {
		t.Fatal("complete version exchange", err)
	}
}

func TestGGOTradeValueAndOverflow(t *testing.T) {
	state := &model.GGOFieldState{Chips: [6]int64{0, 0, 0, 0, 0, math.MaxInt64}, ExchangeValues: [6]int64{3, 9, 30, 90, 300, 900}, EconomyVersion: "v1"}
	value, err := ggoTradeOfferValue(state, state.Chips)
	want := new(big.Int).Mul(big.NewInt(math.MaxInt64), big.NewInt(900)).String()
	if err != nil || value != want {
		t.Fatal("large quote value truncated", value, err)
	}
	other := &model.GGOFieldState{Chips: [6]int64{1}, ExchangeValues: state.ExchangeValues, EconomyVersion: "v1"}
	if _, err = ggoTradeAfter([2]*model.GGOFieldState{state, other}, [2][6]int64{{}, {1}}, true); err == nil {
		t.Fatal("terminal total overflow accepted")
	}
}
