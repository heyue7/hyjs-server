package service

import (
	_ "embed"
	"encoding/json"
)

//go:embed data/growth74_table_rows.json
var growth74TableRows []byte

//go:embed data/growth74_resources.json
var growth74ResourcesJSON []byte

// InstallGrowth74Resources adds ordinary original-format choice/consumable rows.
// The same rows ship in the APK; no new inventory or selection mechanism is added.
func (s *GameService) InstallGrowth74Resources() {
	var resources struct {
		Items []map[string]string
		Uses  []map[string]string
	}
	if err := json.Unmarshal(growth74ResourcesJSON, &resources); err != nil {
		panic(err)
	}
	items, uses := resources.Items, resources.Uses
	for _, r := range items {
		r["intro"] = r["desc"]
		s.growth74Upsert("Item", "itemId", r)
		for _, prefix := range []string{"name_", "desc_"} {
			value := r["name"]
			if prefix == "desc_" {
				value = r["desc"]
			}
			s.growth74Upsert("Text", "id", map[string]string{"id": prefix + r["itemId"], "text": value})
		}
	}
	for _, r := range uses {
		s.growth74Upsert("ItemUse", "id", r)
	}
	var tables map[string]struct {
		Key  string
		Rows []map[string]string
	}
	if err := json.Unmarshal(growth74TableRows, &tables); err != nil {
		panic(err)
	}
	for name, patch := range tables {
		for _, r := range patch.Rows {
			for i, old := range s.Tables[name] {
				if old[patch.Key] == r[patch.Key] {
					s.Tables[name][i] = r
					break
				}
			}
		}
	}
}

func (s *GameService) growth74Upsert(table, key string, row map[string]string) {
	for i, old := range s.Tables[table] {
		if old[key] == row[key] {
			s.Tables[table][i] = row
			return
		}
	}
	s.Tables[table] = append(s.Tables[table], row)
}

var growth74BoxCategories = func() map[string]string {
	var data struct{ Items []map[string]string }
	if err := json.Unmarshal(growth74ResourcesJSON, &data); err != nil {
		panic(err)
	}
	out := map[string]string{}
	for _, r := range data.Items {
		out[r["itemId"]] = r["category"]
	}
	return out
}()
