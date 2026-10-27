package service

import (
	"encoding/json"
	"testing"
)

func TestTrialShopRepricePreservesOtherTerms(t *testing.T) {
	input := []byte(`{"extra":{"future":true},"products":[{"id":"a","rewards":"frag_equip_037,10","cost":"point,15000","extra":7},{"id":"b","rewards":"frag_equip_038,10","cost":"point,15000"},{"id":"c","rewards":"frag_equip_039,10","cost":"point,15000"},{"id":"d","rewards":"frag_equip_040,10","cost":"point,15000"},{"id":"e","rewards":"item_006,500","cost":"point,3000","dailyLimit":3}]}`)
	out, err := trialShopReprice(input)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Extra    map[string]bool `json:"extra"`
		Products []struct {
			Cost       string `json:"cost"`
			Stars      int    `json:"minTrialStars"`
			Extra      int    `json:"extra"`
			DailyLimit int    `json:"dailyLimit"`
		} `json:"products"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	for i, stars := range []int{135, 165, 135, 195} {
		if got.Products[i].Cost != "point,150" || got.Products[i].Stars != stars {
			t.Fatalf("red fragment %d: %+v", i, got.Products[i])
		}
	}
	if !got.Extra["future"] || got.Products[0].Extra != 7 || got.Products[4].Cost != "point,3000" || got.Products[4].DailyLimit != 3 {
		t.Fatal("unrelated catalogue terms changed")
	}
	again, err := trialShopReprice(out)
	if err != nil || string(again) != string(out) {
		t.Fatalf("replay is not idempotent: %v", err)
	}
	if _, err = trialShopReprice([]byte(`{"products":[]}`)); err == nil {
		t.Fatal("missing product accepted")
	}
}
