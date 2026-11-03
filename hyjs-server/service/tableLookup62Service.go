package service

// gameTableLookup indexes the immutable catalogue used by a running service.
// Runtime overlays keep their own rows and fall back when a table was replaced.
type gameTableLookup struct {
	tables map[string]gameTableLookupTable
}

type gameTableLookupTable struct {
	rows    []map[string]string
	columns map[string]map[string]map[string]string
	groups  map[string]map[string][]map[string]string
}

// InitializeGameTableLookup runs after startup catalogue enrichment, before requests.
// Returned rows are the original maps; duplicate and empty keys retain first-row semantics.
func (s *GameService) InitializeGameTableLookup() {
	lookup := &gameTableLookup{tables: make(map[string]gameTableLookupTable, len(s.Tables))}
	for name, rows := range s.Tables {
		entry := gameTableLookupTable{rows: rows, columns: make(map[string]map[string]map[string]string, 2)}
		keys := []string{"id", "StaticName"}
		if name == "Title" {
			keys = append(keys, "titleId")
		}
		for _, key := range keys {
			present := false
			for _, row := range rows {
				if _, present = row[key]; present {
					break
				}
			}
			if !present {
				continue
			}
			column := make(map[string]map[string]string, len(rows))
			for _, row := range rows {
				value := row[key]
				if _, found := column[value]; !found {
					column[value] = row
				}
			}
			entry.columns[key] = column
		}
		if name == "TalentAdvenced" {
			// Preserve source order and row identity for multiple nodes per hero.
			// This catalogue, including its grouping key, is immutable after startup.
			byHero := make(map[string][]map[string]string)
			for _, row := range rows {
				byHero[row["warId"]] = append(byHero[row["warId"]], row)
			}
			entry.groups = map[string]map[string][]map[string]string{"warId": byHero}
		}
		lookup.tables[name] = entry
	}
	s.tableLookup = lookup
}

// indexedTableGroup observes the same catalogue identity guard as indexedTable.
// A replacement runtime table uses its original linear-scan path.
func (s *GameService) indexedTableGroup(name, key, value string) ([]map[string]string, bool) {
	if s.tableLookup == nil {
		return nil, false
	}
	entry, found := s.tableLookup.tables[name]
	rows := s.Tables[name]
	if !found || len(rows) != len(entry.rows) || len(rows) > 0 && &rows[0] != &entry.rows[0] {
		return nil, false
	}
	group, found := entry.groups[key]
	if !found {
		return nil, false
	}
	return group[value], true
}

func (s *GameService) indexedTable(name, key, value string) (map[string]string, bool) {
	if s.tableLookup == nil {
		return nil, false
	}
	entry, found := s.tableLookup.tables[name]
	rows := s.Tables[name]
	if !found || len(rows) != len(entry.rows) || len(rows) > 0 && &rows[0] != &entry.rows[0] {
		return nil, false
	}
	column, found := entry.columns[key]
	if !found {
		return nil, false
	}
	return column[value], true
}
