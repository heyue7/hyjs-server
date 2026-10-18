package service

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

var p6OriginalTestCache struct {
	once   sync.Once
	tables map[string][]map[string]string
	err    error
}

func readP6OriginalTestTables() (map[string][]map[string]string, error) {
	tables := map[string][]map[string]string{}
	for _, name := range []string{"Hero", "Skill", "SkillCast", "Buff", "Talent", "GameConfigure", "CopyEnemy", "BuffInfExtend", "HeroAwakenAdd", "BootHero", "RecruitShowHero", "WantedEnemy"} {
		file, err := os.Open(filepath.Join("../../analysis/tables", "Table"+name+".tsv"))
		if err != nil {
			return nil, err
		}
		reader := csv.NewReader(file)
		reader.Comma = '\t'
		reader.FieldsPerRecord = -1
		rows, err := reader.ReadAll()
		file.Close()
		if err != nil {
			return nil, err
		}
		if len(rows) < 5 {
			return nil, fmt.Errorf("original test table %s has no header", name)
		}
		for _, row := range rows[5:] {
			entry := map[string]string{}
			for index, value := range row {
				if index < len(rows[1]) {
					entry[rows[1][index]] = value
				}
			}
			tables[name] = append(tables[name], entry)
		}
	}
	return tables, nil
}

func cloneP6OriginalTestTables(source map[string][]map[string]string) map[string][]map[string]string {
	result := make(map[string][]map[string]string, len(source))
	for name, rows := range source {
		copied := make([]map[string]string, len(rows))
		for index, row := range rows {
			copied[index] = make(map[string]string, len(row))
			for key, value := range row {
				copied[index][key] = value
			}
		}
		result[name] = copied
	}
	return result
}

func TestP6OriginalTableCachePreservesFreshSourceAndTestIsolation(t *testing.T) {
	fresh, err := readP6OriginalTestTables()
	if err != nil {
		t.Fatal(err)
	}
	first := p6OriginalTables(t)
	if !reflect.DeepEqual(first.Tables, fresh) {
		t.Fatal("cached original table order or data changed")
	}
	// Existing tests mutate rows and whole collections; neither may reach the cache.
	for name, rows := range first.Tables {
		rows[0]["isolated-probe"] = "changed"
		first.Tables[name] = append(rows, map[string]string{"extra": "changed"})
	}
	delete(first.Tables, "Hero")
	second := p6OriginalTables(t)
	if !reflect.DeepEqual(second.Tables, fresh) {
		t.Fatal("mutable table escaped test isolation")
	}
	base := combatTestTables(t)
	for name, rows := range base.Tables {
		if !reflect.DeepEqual(rows, second.Tables[name]) {
			t.Fatal("independent original parser disagrees", name)
		}
	}
}
