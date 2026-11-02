package db

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const sqliteProfileLevelIndexPrefix = "\nCREATE INDEX IF NOT EXISTS idx_profiles_server_level "

// Create the table before adding the index so an older table can acquire the
// generated column before any index references it.
func sqliteProfileTableSchema() (string, error) {
	table, _, ok := strings.Cut(profileSchema, sqliteProfileLevelIndexPrefix)
	if !ok {
		return "", fmt.Errorf("profiles player-level index definition is missing")
	}
	return table, nil
}

// player_level is derived by the database for both normal and raw writes.
func migrateProfilePlayerLevel(d *gorm.DB) error {
	var (
		schema, suffix string
		columnExists   bool
	)
	switch d.Dialector.Name() {
	case "mysql":
		schema, suffix = mysqlProfileSchema, " STORED,"
		columnExists = d.Migrator().HasColumn(&Profile{}, "player_level")
	case "sqlite":
		schema, suffix = profileSchema, " VIRTUAL,"
		var columns []struct{ Name string }
		if err := d.Raw("PRAGMA table_xinfo(profiles)").Scan(&columns).Error; err != nil {
			return err
		}
		for _, column := range columns {
			if column.Name == "player_level" {
				columnExists = true
				break
			}
		}
	default:
		return fmt.Errorf("unsupported profiles player-level database %q", d.Dialector.Name())
	}
	if !columnExists {
		start := strings.Index(schema, " player_level ")
		if start < 0 {
			return fmt.Errorf("profiles player-level definition is missing")
		}
		definition := schema[start:]
		end := strings.Index(definition, suffix)
		if end < 0 {
			return fmt.Errorf("profiles player-level definition is incomplete")
		}
		end += len(suffix) - 1 // omit the comma separating table columns
		if err := d.Exec("ALTER TABLE profiles ADD COLUMN " + strings.TrimSpace(definition[:end])).Error; err != nil {
			return err
		}
	}
	const indexName = "idx_profiles_server_level"
	if !d.Migrator().HasIndex(&Profile{}, indexName) {
		indexSQL := "CREATE INDEX " + indexName + " ON profiles (server_id, player_level, id)"
		if d.Dialector.Name() == "sqlite" {
			_, indexDefinition, ok := strings.Cut(profileSchema, sqliteProfileLevelIndexPrefix)
			if !ok {
				return fmt.Errorf("profiles player-level index definition is missing")
			}
			indexSQL = strings.TrimSpace(sqliteProfileLevelIndexPrefix) + " " + strings.TrimSpace(indexDefinition)
		}
		if err := d.Exec(indexSQL).Error; err != nil {
			return err
		}
		if !d.Migrator().HasIndex(&Profile{}, indexName) {
			return fmt.Errorf("profiles player-level index was not created")
		}
	}
	return nil
}
