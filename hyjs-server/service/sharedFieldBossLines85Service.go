package service

import (
	"errors"
	"strings"

	"gorm.io/gorm"
	"hyjs-server/db"
)

// migrateSharedFieldBossLines85 keeps each old generation on line one. Startup
// changes the existing key before any independent line can accept a fight.
func migrateSharedFieldBossLines85(tx *gorm.DB) error {
	if !tx.Migrator().HasTable(&db.SharedFieldBoss55{}) {
		return tx.AutoMigrate(&db.SharedFieldBoss55{})
	}
	switch tx.Dialector.Name() {
	case "mysql":
		var columns []string
		if err := tx.Raw("SELECT COLUMN_NAME FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='shared_field_boss55' AND INDEX_NAME='PRIMARY' ORDER BY SEQ_IN_INDEX").Scan(&columns).Error; err != nil {
			return err
		}
		key := strings.Join(columns, ",")
		switch key {
		case "server_id,scene,pos_id":
			if tx.Migrator().HasColumn(&db.SharedFieldBoss55{}, "line") {
				if err := tx.Exec("ALTER TABLE shared_field_boss55 DROP PRIMARY KEY, ADD PRIMARY KEY (server_id,scene,line,pos_id)").Error; err != nil {
					return err
				}
			} else if err := tx.Exec("ALTER TABLE shared_field_boss55 ADD COLUMN line BIGINT NOT NULL DEFAULT 1, DROP PRIMARY KEY, ADD PRIMARY KEY (server_id,scene,line,pos_id)").Error; err != nil {
				return err
			}
		case "server_id,scene,line,pos_id":
		default:
			return errors.New("shared boss primary key does not match the migration source")
		}
	case "sqlite":
		var columns []struct {
			Name string
			PK   int
		}
		if err := tx.Raw("PRAGMA table_info(shared_field_boss55)").Scan(&columns).Error; err != nil {
			return err
		}
		lineKey := false
		for _, column := range columns {
			if column.Name == "line" && column.PK > 0 {
				lineKey = true
			}
		}
		if !lineKey {
			// SQLite cannot extend a primary key in place. The startup transaction keeps
			// the old row payloads intact and commits the replacement table atomically.
			if err := tx.AutoMigrate(&db.SharedFieldBoss55{}); err != nil {
				return err
			}
			if err := tx.Transaction(func(t *gorm.DB) error {
				if err := t.Exec("ALTER TABLE shared_field_boss55 RENAME TO shared_field_boss55_legacy85").Error; err != nil {
					return err
				}
				if err := t.Migrator().CreateTable(&db.SharedFieldBoss55{}); err != nil {
					return err
				}
				if err := t.Exec("INSERT INTO shared_field_boss55 (server_id,scene,line,pos_id,uuid,payload,hp,death_rules73,respawn_at,killer_id) SELECT server_id,scene,line,pos_id,uuid,payload,hp,death_rules73,respawn_at,killer_id FROM shared_field_boss55_legacy85").Error; err != nil {
					return err
				}
				return t.Exec("DROP TABLE shared_field_boss55_legacy85").Error
			}); err != nil {
				return err
			}
		}
	default:
		return errors.New("unsupported shared boss migration database")
	}
	return tx.AutoMigrate(&db.SharedFieldBoss55{})
}

// fieldBossCurrentLine85 uses the persisted server-selected line, including
// disconnect recovery. Client parameters cannot choose another spawn namespace.
func (s *GameService) fieldBossCurrentLine85(tx *gorm.DB, id int64) (int64, error) {
	if !gameSchemaHasTable(tx, &db.P10ScenePresence{}) {
		return 1, nil
	}
	var presence db.P10ScenePresence
	err := tx.Select("line").Where("player_id=?", id).First(&presence).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	if presence.Line < 1 {
		return 1, nil
	}
	return int64(presence.Line), nil
}
