package db

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"time"
)

//go:embed sql/01_profiles.mysql.sql
var mysqlProfileSchema string

// MySQLConfig 将连接凭据保留在内存，调用者从环境变量加载密码。
type MySQLConfig struct{ Host, User, Password, Database string }

// OpenGameMySQL 创建连接；applySchema 决定是否创建并验证存档表。
func OpenGameMySQL(ctx context.Context, cfg MySQLConfig, applySchema bool) (*gorm.DB, error) {
	if cfg.Host == "" || cfg.User == "" || cfg.Database == "" || cfg.Password == "" {
		return nil, errors.New("Incomplete MySQL configuration; set HYJS_MYSQL_HOST/USER/DATABASE/PASSWORD")
	}
	c := driver.NewConfig()
	c.Net, c.Addr, c.User, c.Passwd, c.DBName = "tcp", cfg.Host, cfg.User, cfg.Password, cfg.Database
	c.Timeout, c.ReadTimeout, c.WriteTimeout = 10*time.Second, 30*time.Second, 30*time.Second
	c.Params = map[string]string{"charset": "utf8mb4", "sql_mode": "'STRICT_TRANS_TABLES,NO_ENGINE_SUBSTITUTION,NO_AUTO_VALUE_ON_ZERO'"}
	d, err := gorm.Open(mysql.Open(c.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, err
	}
	sqlDB, err := d.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(16)
	sqlDB.SetMaxIdleConns(8)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)
	if err = sqlDB.PingContext(ctx); err == nil && applySchema {
		err = d.WithContext(ctx).Exec(mysqlProfileSchema).Error
	}
	if err == nil && applySchema && !d.Migrator().HasColumn(&Profile{}, "server_id") {
		err = d.WithContext(ctx).Migrator().AddColumn(&Profile{}, "ServerID")
	}
	if err == nil && applySchema {
		for _, column := range []struct{ name, field string }{{"account_id", "AccountID"}, {"account_auth_revision", "AccountAuthRevision"}, {"last_sprout_login_at", "LastSproutLoginAt"}, {"return_sprout_started_at", "ReturnSproutStartedAt"}} {
			if !d.Migrator().HasColumn(&Profile{}, column.name) {
				if err = d.WithContext(ctx).Migrator().AddColumn(&Profile{}, column.field); err != nil {
					break
				}
			}
		}
	}
	if err == nil {
		err = validateMySQLProfiles(ctx, d)
	}
	if err != nil {
		sqlDB.Close()
		return nil, err
	}
	return d, nil
}

// InitExistingGameMySQL connects a maintenance command without creating or migrating tables.
func InitExistingGameMySQL(ctx context.Context, cfg MySQLConfig) error {
	d, err := OpenGameMySQL(ctx, cfg, false)
	if err == nil {
		dbIns = d
	}
	return err
}

// InitGameMySQL 使用 InnoDB 行锁承载所有账号存档事务。
func InitGameMySQL(ctx context.Context, cfg MySQLConfig) error {
	d, err := OpenGameMySQL(ctx, cfg, true)
	if err == nil {
		dbIns = d
	}
	return err
}

// validateMySQLProfiles 拒绝大小写折叠、短文本和非事务存储引擎，避免迁移悄悄改变数据。
func validateMySQLProfiles(ctx context.Context, d *gorm.DB) error {
	var engine string
	if err := d.WithContext(ctx).Raw("SELECT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='profiles'").Scan(&engine).Error; err != nil {
		return err
	}
	if engine != "InnoDB" {
		return errors.New("profiles must use InnoDB")
	}
	var columns []struct {
		ColumnName, ColumnType, IsNullable, Extra string
		CharacterSetName                          *string
	}
	if err := d.WithContext(ctx).Raw("SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, EXTRA, CHARACTER_SET_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='profiles'").Scan(&columns).Error; err != nil {
		return err
	}
	want := map[string]string{"id": "bigint", "uid": "varbinary(1024)", "session": "longblob", "player": "longtext", "game": "longtext", "created_at": "bigint", "password_salt": "longblob", "password_hash": "longblob"}
	for _, col := range columns {
		if expected, ok := want[col.ColumnName]; ok {
			if col.ColumnType != expected {
				return fmt.Errorf("profiles column %s has incompatible type", col.ColumnName)
			}
			if (col.ColumnName == "id" || col.ColumnName == "uid" || col.ColumnName == "session") && col.IsNullable != "NO" {
				return fmt.Errorf("profiles column %s must be NOT NULL", col.ColumnName)
			}
			if col.ColumnName == "id" && col.Extra != "auto_increment" {
				return errors.New("profiles id must auto increment")
			}
			if (col.ColumnName == "player" || col.ColumnName == "game") && (col.CharacterSetName == nil || *col.CharacterSetName != "utf8mb4") {
				return fmt.Errorf("profiles column %s requires utf8mb4", col.ColumnName)
			}
			delete(want, col.ColumnName)
		}
	}
	if len(want) != 0 {
		return errors.New("profiles schema is incomplete")
	}
	var indexes []struct {
		IndexName, ColumnName string
		NonUnique, SeqInIndex int
	}
	if err := d.WithContext(ctx).Raw("SELECT INDEX_NAME,COLUMN_NAME,NON_UNIQUE,SEQ_IN_INDEX FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='profiles'").Scan(&indexes).Error; err != nil {
		return err
	}
	var primary, uniqueUID bool
	counts := map[string]int{}
	for _, i := range indexes {
		counts[i.IndexName]++
	}
	for _, i := range indexes {
		if i.IndexName == "PRIMARY" && i.ColumnName == "id" && counts[i.IndexName] == 1 {
			primary = true
		}
		if i.NonUnique == 0 && i.ColumnName == "uid" && counts[i.IndexName] == 1 {
			uniqueUID = true
		}
	}
	if !primary || !uniqueUID {
		return errors.New("profiles requires id primary key and unique binary uid")
	}
	return nil
}
