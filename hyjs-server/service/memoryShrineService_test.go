package service

import (
	"encoding/json"
	"hyjs-server/db"
	"reflect"
	"testing"
)

func TestMemoryShrine55Offers(t *testing.T) {
	rules := p4CommerceRules{MemoryPolicyVersion: 1}
	for i, id := range []string{"frag_a", "frag_b", "frag_c", "frag_d", "item_a"} {
		p := p4BaseProduct(id, id+",1", "light,1", int32(i+1), 16)
		p.DailyLimit = 1
		rules.Products = append(rules.Products, p)
	}
	for version := int64(1); version < 101; version++ {
		state := &p4CommerceProgress{Version: version, Bought: map[string]int32{}}
		if err := memoryRefresh55(rules, state); err != nil {
			t.Fatal(err)
		}
		offers := memoryProducts55(db.P4CommerceRule{Kind: "memory"}, rules, state)
		if len(offers) != 6 {
			t.Fatal(len(offers))
		}
		frags := 0
		seen := map[string]bool{}
		for _, p := range offers {
			if seen[p.ID] {
				t.Fatal("duplicate slot")
			}
			seen[p.ID] = true
			if p.SourceProductID != "item_a" {
				frags++
			}
			if !p4Visible(db.P4CommerceRule{Kind: "memory"}, state, p) {
				t.Fatal("invisible")
			}
		}
		if frags < 1 || frags > 4 {
			t.Fatal(frags)
		}
		first := offers[4]
		second := offers[5]
		cfg := db.P4CommerceRule{Kind: "memory"}
		state.Bought[p4SlotKey(cfg, state, first)] = 1
		if state.Bought[p4SlotKey(cfg, state, second)] != 0 {
			t.Fatal("shared purchase key")
		}
		raw, _ := json.Marshal(state)
		var restored p4CommerceProgress
		if err := json.Unmarshal(raw, &restored); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(restored.Offers[5], state.Offers[5]) {
			t.Fatal("lost slot")
		}
		old := offers[0]
		state.Version++
		if err := memoryRefresh55(rules, state); err != nil {
			t.Fatal(err)
		}
		if p4Visible(cfg, state, old) {
			t.Fatal("stale offer visible")
		}
	}
}
