package db

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"hyjs-server/config"
	"regexp"
)

// AccountsTableName qualifies only recognized account-owned tables in the optional same-MySQL shared schema.
func AccountsTableName(name string) string {
	database := ""
	if config.AppConfigIns != nil {
		database = config.AppConfigIns.Game.AccountsDatabase
	}
	if database == "" || GetIns().Dialector.Name() != "mysql" {
		return name
	}
	return database + "." + name
}

// AccountsTable keeps cross-schema account, wallet and role changes on the caller's single transaction.
func AccountsTable(tx *gorm.DB, name string) *gorm.DB { return tx.Table(AccountsTableName(name)) }

// InitializeGameAccountsSchema creates global identity tables without rebuilding legacy profile saves.
func InitializeGameAccountsSchema(ctx context.Context) error {
	d := GetIns().WithContext(ctx)
	database := ""
	if config.AppConfigIns != nil {
		database = config.AppConfigIns.Game.AccountsDatabase
	}
	if database != "" && (!regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`).MatchString(database) || d.Dialector.Name() != "mysql") {
		return errors.New("共享账号库仅支持同一MySQL实例的合法数据库名称")
	}
	if d.Dialector.Name() == "mysql" {
		// The configured database must already exist and be granted to the application account; no CREATE DATABASE privileges are required.
		statements := []string{
			"CREATE TABLE IF NOT EXISTS " + AccountsTableName("game_accounts") + " (id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY, username VARBINARY(1024) NOT NULL, nickname VARCHAR(96) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL, password_salt LONGBLOB NOT NULL, password_hash LONGBLOB NOT NULL, enabled BOOLEAN NOT NULL, protected BOOLEAN NOT NULL, auth_revision BIGINT NOT NULL DEFAULT 0, revision BIGINT NOT NULL, created_at BIGINT NOT NULL, UNIQUE KEY game_accounts_username (username)) ENGINE=InnoDB",
			"CREATE TABLE IF NOT EXISTS " + AccountsTableName("game_account_roles") + " (account_id BIGINT NOT NULL, server_id BIGINT NOT NULL, player_id BIGINT NOT NULL, name VARCHAR(96) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NOT NULL, level BIGINT NOT NULL, PRIMARY KEY (account_id, server_id)) ENGINE=InnoDB",
		}
		for _, statement := range statements {
			if err := d.Exec(statement).Error; err != nil {
				return err
			}
		}
		return nil
	}
	return d.AutoMigrate(&GameAccount{}, &GameAccountRole{})
}
