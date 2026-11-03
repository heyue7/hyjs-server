package service

import (
	"fmt"
	"reflect"
	"testing"
)

func linearTable62(rows []map[string]string, key, value string) map[string]string {
	for _, row := range rows {
		if row[key] == value {
			return row
		}
	}
	return nil
}

func TestTableLookup62CatalogueAndOverlay(t *testing.T) {
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	s.Tables["Duplicate62"] = []map[string]string{{"id": "same", "value": "first"}, {"id": "same", "value": "second"}, nil, {"id": "", "value": "empty"}}
	s.InitializeGameTableLookup()
	for name, rows := range s.Tables {
		for _, key := range []string{"id", "StaticName", "value"} {
			values := []string{"", "missing-key-62"}
			for _, row := range rows {
				values = append(values, row[key])
			}
			for _, value := range values {
				if !reflect.DeepEqual(s.table(name, key, value), linearTable62(rows, key, value)) {
					t.Fatalf("lookup changed %s/%s/%s", name, key, value)
				}
			}
		}
	}
	overlay := make(map[string][]map[string]string, len(s.Tables))
	for name, rows := range s.Tables {
		overlay[name] = rows
	}
	overlay["Duplicate62"] = []map[string]string{{"id": "same", "value": "new overlay"}}
	view := s.withRuntimeTables(overlay)
	if view.table("Duplicate62", "id", "same")["value"] != "new overlay" || s.table("Duplicate62", "id", "same")["value"] != "first" {
		t.Fatal("runtime overlay leaked or retained a stale index")
	}
	if _, indexed := view.indexedTable("Hero", "id", "hero_001"); !indexed {
		t.Fatal("unchanged table did not reuse startup index")
	}
}

func TestTableLookup62TitleIDAndOverlay(t *testing.T) {
	s := &GameService{Tables: map[string][]map[string]string{
		"Title": {{"titleId": "same", "value": "first"}, {"titleId": "same", "value": "second"}},
		"Other": {{"titleId": "same", "value": "other"}},
	}}
	s.InitializeGameTableLookup()
	if row, indexed := s.indexedTable("Title", "titleId", "same"); !indexed || row["value"] != "first" {
		t.Fatalf("title index did not retain first row: %v, indexed=%v", row, indexed)
	}
	if row, indexed := s.indexedTable("Title", "titleId", "missing"); !indexed || row != nil {
		t.Fatalf("title index missing-key result: %v, indexed=%v", row, indexed)
	}
	if _, indexed := s.indexedTable("Other", "titleId", "same"); indexed {
		t.Fatal("titleId index added outside Title catalogue")
	}
	overlay := map[string][]map[string]string{
		"Title": {{"titleId": "same", "value": "overlay"}, {"titleId": "same", "value": "later overlay"}},
		"Other": s.Tables["Other"],
	}
	view := s.withRuntimeTables(overlay)
	if _, indexed := view.indexedTable("Title", "titleId", "same"); indexed {
		t.Fatal("replacement title table retained stale index")
	}
	if view.table("Title", "titleId", "same")["value"] != "overlay" || s.table("Title", "titleId", "same")["value"] != "first" {
		t.Fatal("title overlay did not retain isolated first-row lookup")
	}
}

func BenchmarkTableLookup62(b *testing.B) {
	rows := make([]map[string]string, 6000)
	for i := range rows {
		rows[i] = map[string]string{"id": fmt.Sprintf("skill_%05d", i), "value": fmt.Sprint(i)}
	}
	s := &GameService{Tables: map[string][]map[string]string{"SkillCast": rows}}
	s.InitializeGameTableLookup()
	b.Run("linear", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if linearTable62(rows, "id", "skill_05999") == nil {
				b.Fatal("missing row")
			}
		}
	})
	b.Run("indexed", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if s.table("SkillCast", "id", "skill_05999") == nil {
				b.Fatal("missing row")
			}
		}
	})
}
