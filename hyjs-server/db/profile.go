package db

import (
	"context"
	_ "embed"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"path/filepath"
)

//go:embed sql/01_profiles.sql
var profileSchema string

// Profile 保留 Python 原型中的表和列，保证现有账号、密码摘要和存档可直接读取。
type Profile struct {
	AccountID             int64   `gorm:"column:account_id;not null;default:0;index" json:"accountId"`
	AccountAuthRevision   int64   `gorm:"column:account_auth_revision;not null;default:0" json:"-"`
	ServerID              int64   `gorm:"column:server_id;not null;default:1;index" json:"serverId"`
	Id                    int64   `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Uid                   string  `gorm:"column:uid;not null;uniqueIndex" json:"uid"`
	Session               []byte  `gorm:"column:session;not null" json:"-"`
	Player                *string `gorm:"column:player" json:"-"`
	Game                  *string `gorm:"column:game" json:"-"`
	CreatedAt             *int64  `gorm:"column:created_at;autoCreateTime:false" json:"created_at"`
	LastSproutLoginAt     int64   `gorm:"column:last_sprout_login_at;not null;default:0" json:"-"`
	ReturnSproutStartedAt int64   `gorm:"column:return_sprout_started_at;not null;default:0" json:"-"`
	PasswordSalt          []byte  `gorm:"column:password_salt" json:"-"`
	PasswordHash          []byte  `gorm:"column:password_hash" json:"-"`
}

func (Profile) TableName() string { return "profiles" }

// InitGame 使用单连接和立即事务串行写入本地存档，避免战斗结算发生更新丢失。
func InitGame(ctx context.Context, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	d, err := gorm.Open(sqlite.Open(path+"?_busy_timeout=10000&_txlock=immediate"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return err
	}
	sqlDB, err := d.DB()
	if err != nil {
		return err
	}
	sqlDB.SetMaxOpenConns(1)
	// SQLite 的表重建会丢失历史递增序号；按 Python Store 的方式只补齐缺失列。
	if err = d.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(profileSchema).Error; err != nil {
			return err
		}
		var columns []struct{ Name string }
		if err := tx.Raw("PRAGMA table_info(profiles)").Scan(&columns).Error; err != nil {
			return err
		}
		existing := make(map[string]bool, len(columns))
		for _, column := range columns {
			existing[column.Name] = true
		}
		for _, column := range []struct{ name, definition string }{
			{"server_id", "INTEGER NOT NULL DEFAULT 1"},
			{"account_id", "INTEGER NOT NULL DEFAULT 0"},
			{"account_auth_revision", "INTEGER NOT NULL DEFAULT 0"},
			{"password_salt", "BLOB"},
			{"password_hash", "BLOB"},
			{"game", "TEXT"},
			{"last_sprout_login_at", "INTEGER NOT NULL DEFAULT 0"},
			{"return_sprout_started_at", "INTEGER NOT NULL DEFAULT 0"},
		} {
			if !existing[column.name] {
				if err := tx.Exec("ALTER TABLE profiles ADD COLUMN " + column.name + " " + column.definition).Error; err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		sqlDB.Close()
		return err
	}
	dbIns = d
	return nil
}
