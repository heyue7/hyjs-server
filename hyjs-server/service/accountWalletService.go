package service

import (
	"context"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"math"
)

// InitializeAccountWallets preserves each legacy balance/entry byte value and consolidates funds once under an account row lock.
func (s *GameService) InitializeAccountWallets(ctx context.Context) error {
	d := db.GetIns().WithContext(ctx)
	if err := initializeAccountWalletSchema(ctx); err != nil {
		return err
	}
	var cursor int64
	for {
		var owners []db.Profile
		if err := d.Where("id>? AND account_id>0", cursor).Order("id").Limit(200).Find(&owners).Error; err != nil {
			return err
		}
		if len(owners) == 0 {
			return nil
		}
		for _, owner := range owners {
			if err := d.Transaction(func(tx *gorm.DB) error {
				var legacy db.LocalWalletBalance
				err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&legacy, "player_id=?", owner.Id).Error
				if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				if errors.Is(err, gorm.ErrRecordNotFound) {
					legacy = db.LocalWalletBalance{PlayerID: owner.Id}
				}
				if legacy.BalanceCents < 0 || legacy.Revision < 0 {
					return ErrLocalWalletInvalid
				}
				marker := db.AccountWalletMigration{ServerID: AdminProfileServerID(owner), PlayerID: owner.Id, AccountID: owner.AccountID}
				var existing db.AccountWalletMigration
				err = db.AccountsTable(tx, "account_wallet_migrations").Clauses(clause.Locking{Strength: "UPDATE"}).Where("server_id=? AND player_id=?", marker.ServerID, marker.PlayerID).First(&existing).Error
				if err == nil {
					if existing.AccountID != owner.AccountID || existing.Revision != legacy.Revision || existing.BalanceCents != legacy.BalanceCents {
						return errors.New("旧钱包在迁移后发生变化，请先核对历史账务")
					}
					return nil
				}
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				balance, err := accountWalletBalance(tx, owner.AccountID, true)
				if err != nil {
					return err
				}
				if balance.BalanceCents > math.MaxInt64-legacy.BalanceCents {
					return ErrLocalWalletInvalid
				}
				var entries []db.LocalWalletEntry
				if err = tx.Where("player_id=?", owner.Id).Order("revision,`key`").Find(&entries).Error; err != nil {
					return err
				}
				for _, entry := range entries {
					global := accountWalletEntryFromLegacy(owner, entry)
					global.Key = fmt.Sprintf("legacy:%d:%d:%s", marker.ServerID, marker.PlayerID, entry.Key)
					global.LegacyKey = entry.Key
					if err = db.AccountsTable(tx, "account_wallet_entries").Create(&global).Error; err != nil {
						return err
					}
				}
				balance.BalanceCents += legacy.BalanceCents
				if legacy.Revision > balance.Revision {
					balance.Revision = legacy.Revision
				}
				if err = db.AccountsTable(tx, "account_wallet_balances").Save(balance).Error; err != nil {
					return err
				}
				marker.BalanceCents, marker.Revision, marker.EntryCount = legacy.BalanceCents, legacy.Revision, int64(len(entries))
				return db.AccountsTable(tx, "account_wallet_migrations").Create(&marker).Error
			}); err != nil {
				return err
			}
		}
		cursor = owners[len(owners)-1].Id
	}
}
func initializeAccountWalletSchema(ctx context.Context) error {
	d := db.GetIns().WithContext(ctx)
	if d.Dialector.Name() == "mysql" {
		statements := []string{
			"CREATE TABLE IF NOT EXISTS " + db.AccountsTableName("account_wallet_balances") + " (account_id BIGINT NOT NULL PRIMARY KEY,balance_cents BIGINT NOT NULL,revision BIGINT NOT NULL) ENGINE=InnoDB",
			"CREATE TABLE IF NOT EXISTS " + db.AccountsTableName("account_wallet_entries") + " (account_id BIGINT NOT NULL,`key` VARCHAR(240) NOT NULL,server_id BIGINT NOT NULL,player_id BIGINT NOT NULL,legacy_key VARCHAR(160) NOT NULL,kind VARCHAR(16) NOT NULL,delta_cents BIGINT NOT NULL,balance_after BIGINT NOT NULL,revision BIGINT NOT NULL,order_id VARCHAR(64) NULL,at BIGINT NOT NULL,source VARCHAR(64) NOT NULL,reason_id VARCHAR(64) NOT NULL,label VARCHAR(255) CHARACTER SET utf8mb4 NOT NULL,PRIMARY KEY(account_id,`key`),UNIQUE KEY account_wallet_order(order_id)) ENGINE=InnoDB",
			"CREATE TABLE IF NOT EXISTS " + db.AccountsTableName("account_wallet_migrations") + " (server_id BIGINT NOT NULL,player_id BIGINT NOT NULL,account_id BIGINT NOT NULL,balance_cents BIGINT NOT NULL,revision BIGINT NOT NULL,entry_count BIGINT NOT NULL,PRIMARY KEY(server_id,player_id)) ENGINE=InnoDB",
		}
		for _, statement := range statements {
			if err := d.Exec(statement).Error; err != nil {
				return err
			}
		}
		return nil
	}
	return d.AutoMigrate(&db.AccountWalletBalance{}, &db.AccountWalletEntry{}, &db.AccountWalletMigration{})
}
func accountWalletBalance(tx *gorm.DB, id int64, lock bool) (*db.AccountWalletBalance, error) {
	balance := &db.AccountWalletBalance{AccountID: id}
	if lock {
		if err := db.AccountsTable(tx, "account_wallet_balances").Clauses(clause.OnConflict{DoNothing: true}).Create(balance).Error; err != nil {
			return nil, err
		}
	}
	q := db.AccountsTable(tx, "account_wallet_balances")
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := q.First(balance, "account_id=?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) && !lock {
		return balance, nil
	}
	if err != nil {
		return nil, err
	}
	if balance.BalanceCents < 0 || balance.Revision < 0 {
		return nil, ErrLocalWalletInvalid
	}
	return balance, nil
}
func accountWalletEntryFromLegacy(owner db.Profile, entry db.LocalWalletEntry) db.AccountWalletEntry {
	return db.AccountWalletEntry{AccountID: owner.AccountID, Key: entry.Key, ServerID: AdminProfileServerID(owner), PlayerID: owner.Id, Kind: entry.Kind, DeltaCents: entry.DeltaCents, BalanceAfter: entry.BalanceAfter, Revision: entry.Revision, OrderID: entry.OrderID, At: entry.At, Source: entry.Source, ReasonID: entry.ReasonID, Label: entry.Label}
}
func accountWalletEntryLegacy(entry db.AccountWalletEntry) db.LocalWalletEntry {
	key := entry.Key
	if entry.LegacyKey != "" {
		key = entry.LegacyKey
	}
	return db.LocalWalletEntry{PlayerID: entry.PlayerID, Key: key, Kind: entry.Kind, DeltaCents: entry.DeltaCents, BalanceAfter: entry.BalanceAfter, Revision: entry.Revision, OrderID: entry.OrderID, At: entry.At, Source: entry.Source, ReasonID: entry.ReasonID, Label: entry.Label}
}
func walletStorageOwner(tx *gorm.DB, id int64) (*db.Profile, error) {
	var owner db.Profile
	if err := tx.Select("id", "server_id", "account_id").First(&owner, id).Error; err != nil {
		return nil, err
	}
	return &owner, nil
}
func walletReadBalance(tx *gorm.DB, id int64, lock bool) (*db.LocalWalletBalance, error) {
	owner, err := walletStorageOwner(tx, id)
	if err != nil {
		return nil, err
	}
	if owner.AccountID > 0 {
		balance, err := accountWalletBalance(tx, owner.AccountID, lock)
		if err != nil {
			return nil, err
		}
		return &db.LocalWalletBalance{PlayerID: id, BalanceCents: balance.BalanceCents, Revision: balance.Revision}, nil
	}
	balance := &db.LocalWalletBalance{PlayerID: id}
	if lock {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(balance).Error; err != nil {
			return nil, err
		}
	}
	q := tx
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err = q.First(balance, "player_id=?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) && !lock {
		return balance, nil
	}
	if err == nil && (balance.BalanceCents < 0 || balance.Revision < 0) {
		return nil, ErrLocalWalletInvalid
	}
	return balance, err
}
func walletSaveBalance(tx *gorm.DB, balance *db.LocalWalletBalance) error {
	owner, err := walletStorageOwner(tx, balance.PlayerID)
	if err != nil {
		return err
	}
	if owner.AccountID > 0 {
		return db.AccountsTable(tx, "account_wallet_balances").Save(&db.AccountWalletBalance{AccountID: owner.AccountID, BalanceCents: balance.BalanceCents, Revision: balance.Revision}).Error
	}
	return tx.Save(balance).Error
}
func walletCreateEntry(tx *gorm.DB, entry *db.LocalWalletEntry) error {
	owner, err := walletStorageOwner(tx, entry.PlayerID)
	if err != nil {
		return err
	}
	if owner.AccountID > 0 {
		row := accountWalletEntryFromLegacy(*owner, *entry)
		return db.AccountsTable(tx, "account_wallet_entries").Create(&row).Error
	}
	return tx.Create(entry).Error
}
func walletFindEntry(tx *gorm.DB, id int64, key, orderID string, entry *db.LocalWalletEntry) error {
	owner, err := walletStorageOwner(tx, id)
	if err != nil {
		return err
	}
	if owner.AccountID > 0 {
		var row db.AccountWalletEntry
		q := db.AccountsTable(tx, "account_wallet_entries").Where("account_id=?", owner.AccountID)
		if orderID != "" {
			q = q.Where("order_id=?", orderID)
		} else {
			q = q.Where("`key`=? OR (server_id=? AND player_id=? AND legacy_key=?)", key, AdminProfileServerID(*owner), id, key)
		}
		if err := q.First(&row).Error; err != nil {
			return err
		}
		*entry = accountWalletEntryLegacy(row)
		return nil
	}
	if orderID != "" {
		return tx.First(entry, "order_id=?", orderID).Error
	}
	return tx.First(entry, "player_id=? AND `key`=?", id, key).Error
}
func walletEntryQuery(tx *gorm.DB, id int64) (*gorm.DB, error) {
	owner, err := walletStorageOwner(tx, id)
	if err != nil {
		return nil, err
	}
	if owner.AccountID > 0 {
		return db.AccountsTable(tx, "account_wallet_entries").Where("account_id=?", owner.AccountID), nil
	}
	return tx.Model(&db.LocalWalletEntry{}).Where("player_id=?", id), nil
}

// AdminAccountWalletLedger reads the one global account ledger without needing a role in this runtime.
func AdminAccountWalletLedger(ctx context.Context, actor *model.AdminPrincipal, id, start, end int64, kind string, page, size int64) (*model.LocalWalletView, error) {
	tx := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(tx, actor, false); err != nil {
		return nil, err
	}
	var account db.GameAccount
	if err := db.AccountsTable(tx, "game_accounts").First(&account, id).Error; err != nil {
		return nil, err
	}
	if account.Protected {
		return nil, constUtil.ErrAdminPermission
	}
	return accountWalletLedger(tx, id, start, end, kind, page, size)
}
