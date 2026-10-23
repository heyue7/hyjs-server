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
