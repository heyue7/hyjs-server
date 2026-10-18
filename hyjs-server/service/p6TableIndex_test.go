package service

import (
	"reflect"
	"testing"
	"time"
)

func TestP6FrozenTableIndexesRetainAllOriginalRowsAndDuplicateOrder(t *testing.T) {
	s := p6OriginalTables(t)
	e := &combatRoundEngine{service: s}
	for _, definition := range []struct{ name, key string }{{"Hero", "id"}, {"Skill", "SkillId"}, {"SkillCast", "SkillCastId"}, {"Buff", "Id"}, {"Talent", "Id"}, {"GameConfigure", "StaticName"}} {
		for _, row := range s.Tables[definition.name] {
			id := row[definition.key]
			if !reflect.DeepEqual(e.table(definition.name, definition.key, id), s.table(definition.name, definition.key, id)) {
				t.Fatal("original indexed row disagrees", definition, id)
			}
		}
		if e.table(definition.name, definition.key, "__original_missing__") != nil {
			t.Fatal("missing ID created a row")
		}
	}
	s = &GameService{Tables: map[string][]map[string]string{"duplicates": {nil, {"id": "same", "value": "first"}, {"id": "same", "value": "later"}, {"other": "missing-key"}, {"id": ""}}}}
	e = &combatRoundEngine{service: s}
	for _, id := range []string{"same", "", "absent"} {
		if !reflect.DeepEqual(e.table("duplicates", "id", id), s.table("duplicates", "id", id)) {
			t.Fatal("first duplicate or nil row changed", id)
		}
	}
	if e.table("duplicates", "id", "same")["value"] != "first" {
		t.Fatal("duplicate order changed")
	}
	// Indexes are calculation-local and cannot introduce a live-config cross-input alias.
	second := &combatRoundEngine{service: &GameService{Tables: map[string][]map[string]string{"duplicates": {{"id": "same", "value": "independent"}}}}}
	if second.table("duplicates", "id", "same")["value"] != "independent" {
		t.Fatal("frozen calculations shared an index")
	}
}

func TestP6OriginalLookupIndexMeasurements(t *testing.T) {
	s := p6OriginalTables(t)
	ids := make([]string, 0, len(s.Tables["Buff"]))
	for _, row := range s.Tables["Buff"] {
		ids = append(ids, row["Id"])
	}
	direct := time.Now()
	for repeat := 0; repeat < 20; repeat++ {
		for _, id := range ids {
			if s.table("Buff", "Id", id) == nil {
				t.Fatal(id)
			}
		}
	}
	linearElapsed := time.Since(direct)
	indexed := time.Now()
	e := &combatRoundEngine{service: s}
	for repeat := 0; repeat < 20; repeat++ {
		for _, id := range ids {
			if e.table("Buff", "Id", id) == nil {
				t.Fatal(id)
			}
		}
	}
	t.Logf("actual original Buff lookups %d: linear=%s calculation-local index=%s", 20*len(ids), linearElapsed, time.Since(indexed))
}

func TestP6TableIndexSeesExplicitMechanismProbeTableReplacement(t *testing.T) {
	s := &GameService{Tables: map[string][]map[string]string{"Buff": {{"Id": "original", "value": "first"}}}}
	e := &combatRoundEngine{service: s}
	if e.table("Buff", "Id", "probe") != nil {
		t.Fatal("probe appeared before explicit preparation")
	}
	// Add one isolated mechanism fixture after the first original lookup.
	s.Tables["Buff"] = append(s.Tables["Buff"], map[string]string{"Id": "probe", "value": "appended"})
	if e.table("Buff", "Id", "probe")["value"] != "appended" || e.table("Buff", "Id", "original")["value"] != "first" {
		t.Fatal("appended fixture or original row changed")
	}
	// Replace the slice at the same size, then remove all rows. No old values
	// survive either preparation and the first-duplicate contract still holds.
	s.Tables["Buff"] = []map[string]string{{"Id": "probe", "value": "replacement"}, {"Id": "probe", "value": "duplicate"}}
	if e.table("Buff", "Id", "probe")["value"] != "replacement" || e.table("Buff", "Id", "original") != nil {
		t.Fatal("same-size replacement reused the old index")
	}
	s.Tables["Buff"] = nil
	if e.table("Buff", "Id", "probe") != nil || e.table("Buff", "Id", "") != nil {
		t.Fatal("removed fixture retained stale rows")
	}
}
