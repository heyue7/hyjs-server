package db

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// The database owns this small projection, including legacy/raw profile writes.
// MySQL stores it once per profile write; peers never read complete inventories.
func migrateProfileSceneScope(d *gorm.DB) error {
	mysql := d.Dialector.Name() == "mysql"
	if mysql && d.Migrator().HasColumn(&Profile{}, "scene_scope") {
		return nil
	}
	if !mysql {
		var columns []struct{ Name string }
		if err := d.Raw("PRAGMA table_xinfo(profiles)").Scan(&columns).Error; err != nil {
			return err
		}
		for _, column := range columns {
			if column.Name == "scene_scope" {
				return nil
			}
		}
	}
	schema := profileSchema
	if mysql {
		schema = mysqlProfileSchema
	}
	start := strings.Index(schema, " scene_scope ")
	if start < 0 {
		return fmt.Errorf("profiles scene projection definition is missing")
	}
	definition := schema[start:]
	end := strings.Index(definition, " ELSE '' END) ")
	if end < 0 {
		return fmt.Errorf("profiles scene projection definition is incomplete")
	}
	suffix := "VIRTUAL"
	if mysql {
		suffix = "STORED"
	}
	end += len(" ELSE '' END) ") + len(suffix)
	return d.Exec("ALTER TABLE profiles ADD COLUMN " + strings.TrimSpace(definition[:end])).Error
}
