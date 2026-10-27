package service

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sync"
	"testing"
)

func frozenWindowPatchFixture() FrozenJSONWindowPatch {
	return FrozenJSONWindowPatch{StartPath: []string{"startAt"}, EndPath: []string{"endAt"}, ClaimEndPath: []string{"rewardEndAt"}, Identities: []FrozenJSONIdentity{{Path: []string{"id"}, Expected: "period-1"}}, Expected: FrozenJSONWindow{100, 200, 300}, Target: FrozenJSONWindow{400, 500, 600}, MinMillis: 1, MaxMillis: 1000}
}

func TestFrozenJSONWindowPreservesRewardsAndUnknownFields(t *testing.T) {
	raw := []byte(`{ "id":"period-1","startAt":100,"endAt":200,"rewardEndAt":300,"rewards":[{"amount":9007199254740993,"weight":9223372036854775807,"nested":{"startAt":100,"note":"保留"}}],"unknown":{"decimal":1.234567890123456789,"flag":true,"empty":[],"null":null} }`)
	patch := frozenWindowPatchFixture()
	result, changed, err := PatchFrozenJSONWindow(raw, patch)
	if err != nil || !changed {
		t.Fatal(changed, err)
	}
	decode := func(raw []byte) map[string]any {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		var value map[string]any
		if err := decoder.Decode(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before, after := decode(raw), decode(result)
	for _, field := range []string{"rewards", "unknown", "id"} {
		if !reflect.DeepEqual(before[field], after[field]) {
			t.Fatal("unrelated JSON changed", field)
		}
	}
	if after["startAt"] != json.Number("400") || after["endAt"] != json.Number("500") || after["rewardEndAt"] != json.Number("600") {
		t.Fatal(after)
	}
	repeated, changed, err := PatchFrozenJSONWindow(result, patch)
	if err != nil || changed || !bytes.Equal(result, repeated) {
		t.Fatal("repeat was not byte-preserving", changed, err)
	}
	if !bytes.Contains(raw, []byte(`"startAt":100`)) {
		t.Fatal("input mutated")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, changed, err := PatchFrozenJSONWindow(raw, patch)
			if err != nil || !changed || !bytes.Equal(got, result) {
				t.Error("parallel pure conversion differs", err)
			}
		}()
	}
	wg.Wait()
}

func TestFrozenJSONWindowExplicitNestedAndFormMappings(t *testing.T) {
	for _, kind := range []string{"form", "pool"} {
		t.Run(kind, func(t *testing.T) {
			patch := frozenWindowPatchFixture()
			raw := []byte(`{"templateId":"form-1","startAt":100,"endAt":200,"claimEndAt":300,"products":[{"reward":"a,10"}]}`)
			if kind == "form" {
				patch.ClaimEndPath = []string{"claimEndAt"}
				patch.Identities = []FrozenJSONIdentity{{Path: []string{"templateId"}, Expected: "form-1"}}
			} else {
				raw = []byte(`{"pool":{"id":"pool-1","startAt":100,"endAt":200,"rewardEndAt":300},"rewards":[{"resourceId":"item_001","amount":10,"weight":100}]}`)
				patch.StartPath = []string{"pool", "startAt"}
				patch.EndPath = []string{"pool", "endAt"}
				patch.ClaimEndPath = []string{"pool", "rewardEndAt"}
				patch.Identities = []FrozenJSONIdentity{{Path: []string{"pool", "id"}, Expected: "pool-1"}}
			}
			if _, changed, err := PatchFrozenJSONWindow(raw, patch); err != nil || !changed {
				t.Fatal(changed, err)
			}
		})
	}
}

func TestFrozenJSONWindowRejectsInvalidAndStaleInputs(t *testing.T) {
	valid := `{"id":"period-1","startAt":100,"endAt":200,"rewardEndAt":300}`
	cases := map[string]string{
		"missing":  `{"id":"period-1","startAt":100,"endAt":200}`,
		"fraction": `{"id":"period-1","startAt":100.0,"endAt":200,"rewardEndAt":300}`,
		"exponent": `{"id":"period-1","startAt":1e2,"endAt":200,"rewardEndAt":300}`,
		"string":   `{"id":"period-1","startAt":"100","endAt":200,"rewardEndAt":300}`,
		"overflow": `{"id":"period-1","startAt":9223372036854775808,"endAt":200,"rewardEndAt":300}`,
		"identity": `{"id":"other","startAt":100,"endAt":200,"rewardEndAt":300}`,
		"mixedCAS": `{"id":"period-1","startAt":400,"endAt":200,"rewardEndAt":300}`,
		"stale":    `{"id":"period-1","startAt":101,"endAt":201,"rewardEndAt":301}`,
		"broken":   `{"id":"period-1",`, "array": `[]`, "null": `null`, "empty": "",
		"trailing": valid + `{}`, "duplicate": `{"id":"other","id":"period-1","startAt":100,"endAt":200,"rewardEndAt":300}`,
		"nestedDuplicate": `{"id":"period-1","startAt":100,"endAt":200,"rewardEndAt":300,"other":{"a":1,"a":2}}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			result, changed, err := PatchFrozenJSONWindow([]byte(raw), frozenWindowPatchFixture())
			if err == nil || changed || result != nil {
				t.Fatal("invalid frozen accepted", changed, err)
			}
		})
	}
	mutations := map[string]func(*FrozenJSONWindowPatch){
		"targetOrder":      func(p *FrozenJSONWindowPatch) { p.Target.EndAt = p.Target.StartAt },
		"targetClaim":      func(p *FrozenJSONWindowPatch) { p.Target.ClaimEndAt = p.Target.EndAt - 1 },
		"targetBounds":     func(p *FrozenJSONWindowPatch) { p.Target.ClaimEndAt = 1001 },
		"oldOrder":         func(p *FrozenJSONWindowPatch) { p.Expected.StartAt = 0 },
		"missingIdentity":  func(p *FrozenJSONWindowPatch) { p.Identities = nil },
		"missingMapping":   func(p *FrozenJSONWindowPatch) { p.StartPath = nil },
		"duplicateMapping": func(p *FrozenJSONWindowPatch) { p.EndPath = p.StartPath },
		"missingParent":    func(p *FrozenJSONWindowPatch) { p.StartPath = []string{"absent", "startAt"} },
		"maxRange":         func(p *FrozenJSONWindowPatch) { p.MaxMillis = 253402300800000 },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			patch := frozenWindowPatchFixture()
			mutate(&patch)
			if _, _, err := PatchFrozenJSONWindow([]byte(valid), patch); err == nil {
				t.Fatal("invalid patch accepted")
			}
		})
	}
	patch := frozenWindowPatchFixture()
	patch.SkipEmpty = true
	raw := []byte("  ")
	if result, changed, err := PatchFrozenJSONWindow(raw, patch); err != nil || changed || !bytes.Equal(raw, result) {
		t.Fatal("explicit empty skip failed", changed, err)
	}
}
