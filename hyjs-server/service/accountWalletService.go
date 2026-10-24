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
	"strings"
)

// InitializeAccountWallets preserves each legacy balance/entry byte value and consolidates funds once under an account row lock.
func (s *GameService) InitializeAccountWallets(ctx context.Context) error {
	d := db.GetIns().WithContext(ctx)
	if err := initializeAccountWalletSchema(ctx); err != nil {
		return err
	}
	if err := initializeRoleWalletSchema(ctx); err != nil {
		return err
	}
	var cursor int64
	for {
		var owners []db.Profile
		if err := d.Where("id>? AND account_id>0", cursor).Order("id").Limit(200).Find(&owners).Error; err != nil {
			return err
		}
		if len(owners) == 0 {
			return s.InitializeRoleWallets(ctx)
		}
		for _, owner := range owners {
			if err := d.Transaction(func(tx *gorm.DB) error {
				var migrated int64
				if err := db.AccountsTable(tx, "role_wallet_migrations").Where("account_id=?", owner.AccountID).Count(&migrated).Error; err != nil {
					return err
				}
				if migrated > 0 {
					return nil
				}
				var legacy db.LocalWalletBalance
				err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&legacy, "player_id=?", owner.Id).Error
				if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				if errors.Is(err, gorm.ErrRecordNotFound) {
					var entryCount int64
					if err := tx.Model(&db.LocalWalletEntry{}).Where("player_id=?", owner.Id).Count(&entryCount).Error; err != nil {
						return err
					}
					if entryCount == 0 {
						return nil
					}
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
func roleWalletBalance(tx *gorm.DB, serverID, playerID, accountID int64, lock bool) (*db.RoleWalletBalance, error) {
	row := &db.RoleWalletBalance{ServerID: serverID, PlayerID: playerID, AccountID: accountID}
	q := db.AccountsTable(tx, "role_wallet_balances")
	if lock {
		if err := q.Clauses(clause.OnConflict{DoNothing: true}).Create(row).Error; err != nil {
			return nil, err
		}
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := q.Where("server_id=? AND player_id=?", serverID, playerID).First(row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) && !lock {
		return row, nil
	}
	if err != nil {
		return nil, err
	}
	if row.AccountID != accountID || row.BalanceCents < 0 || row.Revision < 0 {
		return nil, ErrLocalWalletInvalid
	}
	return row, nil
}
func walletReadBalance(tx *gorm.DB, id int64, lock bool) (*db.LocalWalletBalance, error) {
	owner, err := walletStorageOwner(tx, id)
	if err != nil {
		return nil, err
	}
	if owner.AccountID < 1 {
		// Explicit legacy fixtures and pre-migration roles retain their original local storage.
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
		err := q.First(balance, "player_id=?", id).Error
		if errors.Is(err, gorm.ErrRecordNotFound) && !lock {
			return balance, nil
		}
		if err == nil && (balance.BalanceCents < 0 || balance.Revision < 0) {
			return nil, ErrLocalWalletInvalid
		}
		return balance, err
	}
	b, err := roleWalletBalance(tx, AdminProfileServerID(*owner), id, owner.AccountID, lock)
	if err != nil {
		return nil, err
	}
	return &db.LocalWalletBalance{PlayerID: id, BalanceCents: b.BalanceCents, Revision: b.Revision}, nil
}
func walletSaveBalance(tx *gorm.DB, b *db.LocalWalletBalance) error {
	owner, err := walletStorageOwner(tx, b.PlayerID)
	if err != nil {
		return err
	}
	if owner.AccountID < 1 {
		return tx.Save(b).Error
	}
	return db.AccountsTable(tx, "role_wallet_balances").Save(&db.RoleWalletBalance{ServerID: AdminProfileServerID(*owner), PlayerID: b.PlayerID, AccountID: owner.AccountID, BalanceCents: b.BalanceCents, Revision: b.Revision}).Error
}
func walletCreateEntry(tx *gorm.DB, e *db.LocalWalletEntry) error {
	owner, err := walletStorageOwner(tx, e.PlayerID)
	if err != nil {
		return err
	}
	if owner.AccountID < 1 {
		return tx.Create(e).Error
	}
	a := accountWalletEntryFromLegacy(*owner, *e)
	return db.AccountsTable(tx, "role_wallet_entries").Create(&db.RoleWalletEntry{AccountID: a.AccountID, ServerID: a.ServerID, PlayerID: a.PlayerID, Key: a.Key, Kind: a.Kind, DeltaCents: a.DeltaCents, BalanceAfter: a.BalanceAfter, Revision: a.Revision, OrderID: a.OrderID, At: a.At, Source: a.Source, ReasonID: a.ReasonID, Label: a.Label}).Error
}
func walletFindEntry(tx *gorm.DB, id int64, key, orderID string, e *db.LocalWalletEntry) error {
	owner, err := walletStorageOwner(tx, id)
	if err != nil {
		return err
	}
	if owner.AccountID < 1 {
		q := tx.Where("player_id=?", id)
		if orderID != "" {
			return q.First(e, "order_id=?", orderID).Error
		}
		return q.First(e, "`key`=?", key).Error
	}
	q, err := walletEntryQuery(tx, id)
	if err != nil {
		return err
	}
	if orderID != "" {
		q = q.Where("order_id=?", orderID)
	} else {
		q = q.Where("(`key`=? OR legacy_key=?)", key, key)
	}
	var row db.RoleWalletEntry
	if err := q.First(&row).Error; err != nil {
		return err
	}
	keyValue := row.Key
	if row.LegacyKey != "" {
		keyValue = row.LegacyKey
	}
	*e = db.LocalWalletEntry{PlayerID: row.PlayerID, Key: keyValue, Kind: row.Kind, DeltaCents: row.DeltaCents, BalanceAfter: row.BalanceAfter, Revision: row.Revision, OrderID: row.OrderID, At: row.At, Source: row.Source, ReasonID: row.ReasonID, Label: row.Label}
	return nil
}
func walletEntryQuery(tx *gorm.DB, id int64) (*gorm.DB, error) {
	owner, err := walletStorageOwner(tx, id)
	if err != nil {
		return nil, err
	}
	if owner.AccountID < 1 {
		return tx.Model(&db.LocalWalletEntry{}).Where("player_id=?", id), nil
	}
	return db.AccountsTable(tx, "role_wallet_entries").Where("server_id=? AND player_id=? AND account_id=?", AdminProfileServerID(*owner), id, owner.AccountID), nil
}

// AdminAccountWalletLedger requires an explicit role now that balances belong to realm characters.
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
	return nil, fmt.Errorf("%w: 请选择区服角色查看钱包账单", constUtil.ErrAdminInput)
}

func initializeRoleWalletSchema(ctx context.Context) error {
	d := db.GetIns().WithContext(ctx)
	for _, item := range []struct {
		name string
		row  any
	}{{"role_wallet_balances", &db.RoleWalletBalance{}}, {"role_wallet_entries", &db.RoleWalletEntry{}}, {"role_wallet_migrations", &db.RoleWalletMigration{}}, {"terminal_role_states", &db.TerminalRoleState{}}, {"terminal_role_incomes", &db.TerminalRoleIncome{}}, {"terminal_role_bursts", &db.TerminalRoleBurst{}}} {
		if d.Dialector.Name() == "mysql" {
			raw, err := db.RoleWalletSchemaSQL(item.name)
			if err != nil {
				return err
			}
			if err = d.Exec(strings.Replace(string(raw), "CREATE TABLE IF NOT EXISTS "+item.name, "CREATE TABLE IF NOT EXISTS "+db.AccountsTableName(item.name), 1)).Error; err != nil {
				return err
			}
		} else if err := d.AutoMigrate(item.row); err != nil {
			return err
		}
	}
	return nil
}

// InitializeRoleWallets atomically assigns immutable account history to its unique existing role.
func (s *GameService) InitializeRoleWallets(ctx context.Context) error {
	if err := initializeRoleWalletSchema(ctx); err != nil {
		return err
	}
	d := db.GetIns().WithContext(ctx)
	var cursor int64
	for {
		var accounts []db.GameAccount
		if err := db.AccountsTable(d, "game_accounts").Select("id").Where("id>?", cursor).Order("id").Limit(200).Find(&accounts).Error; err != nil {
			return err
		}
		if len(accounts) == 0 {
			return nil
		}
		for _, a := range accounts {
			if err := d.Transaction(func(tx *gorm.DB) error {
				var account db.GameAccount
				if err := db.AccountsTable(tx, "game_accounts").Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, a.ID).Error; err != nil {
					return err
				}
				var marker db.RoleWalletMigration
				err := db.AccountsTable(tx, "role_wallet_migrations").First(&marker, "account_id=?", a.ID).Error
				if err == nil {
					return nil
				}
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				b := &db.AccountWalletBalance{AccountID: a.ID}
				err = db.AccountsTable(tx, "account_wallet_balances").Clauses(clause.Locking{Strength: "UPDATE"}).First(b, "account_id=?", a.ID).Error
				if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				if b.BalanceCents < 0 || b.Revision < 0 {
					return ErrLocalWalletInvalid
				}
				var entries []db.AccountWalletEntry
				if err = db.AccountsTable(tx, "account_wallet_entries").Where("account_id=?", a.ID).Order("revision,`key`").Find(&entries).Error; err != nil {
					return err
				}
				var states []db.TerminalAccountState
				var incomes []db.TerminalIncome
				var bursts []db.TerminalBurst
				if err = db.AccountsTable(tx, "terminal_account_states").Where("account_id=?", a.ID).Find(&states).Error; err != nil {
					return err
				}
				if err = db.AccountsTable(tx, "terminal_incomes").Where("account_id=?", a.ID).Find(&incomes).Error; err != nil {
					return err
				}
				if err = db.AccountsTable(tx, "terminal_bursts").Where("account_id=?", a.ID).Find(&bursts).Error; err != nil {
					return err
				}
				if b.BalanceCents == 0 && b.Revision == 0 && len(entries)+len(states)+len(incomes)+len(bursts) == 0 {
					return nil
				}
				var roles []db.GameAccountRole
				if err = db.AccountsTable(tx, "game_account_roles").Where("account_id=?", a.ID).Find(&roles).Error; err != nil {
					return err
				}
				if len(roles) != 1 {
					return fmt.Errorf("账号 %d 旧钱包迁移要求唯一角色，实际 %d 个角色", a.ID, len(roles))
				}
				r := roles[0]
				row := db.RoleWalletBalance{AccountID: a.ID, ServerID: r.ServerID, PlayerID: r.PlayerID, BalanceCents: b.BalanceCents, Revision: b.Revision}
				if err = db.AccountsTable(tx, "role_wallet_balances").Create(&row).Error; err != nil {
					return err
				}
				for _, e := range entries {
					v := db.RoleWalletEntry{AccountID: a.ID, ServerID: r.ServerID, PlayerID: r.PlayerID, Key: e.Key, LegacyKey: e.LegacyKey, Kind: e.Kind, DeltaCents: e.DeltaCents, BalanceAfter: e.BalanceAfter, Revision: e.Revision, OrderID: e.OrderID, At: e.At, Source: e.Source, ReasonID: e.ReasonID, Label: e.Label}
					if err = db.AccountsTable(tx, "role_wallet_entries").Create(&v).Error; err != nil {
						return err
					}
				}
				for _, e := range states {
					if err = db.AccountsTable(tx, "terminal_role_states").Create(&db.TerminalRoleState{AccountID: a.ID, ServerID: r.ServerID, PlayerID: r.PlayerID, SeenRevision: e.SeenRevision, DeliveredRevision: e.DeliveredRevision}).Error; err != nil {
						return err
					}
				}
				for _, e := range incomes {
					if err = db.AccountsTable(tx, "terminal_role_incomes").Create(&db.TerminalRoleIncome{AccountID: a.ID, ServerID: r.ServerID, PlayerID: r.PlayerID, ID: e.ID, Label: e.Label, AmountCents: e.AmountCents, CreatedAt: e.CreatedAt, Revision: e.Revision, Source: e.Source}).Error; err != nil {
						return err
					}
				}
				for _, e := range bursts {
					if err = db.AccountsTable(tx, "terminal_role_bursts").Create(&db.TerminalRoleBurst{AccountID: a.ID, ServerID: r.ServerID, PlayerID: r.PlayerID, RequestID: e.RequestID, Day: e.Day, AwardPoints: e.AwardPoints}).Error; err != nil {
						return err
					}
				}
				var count int64
				if err = db.AccountsTable(tx, "role_wallet_entries").Where("server_id=? AND player_id=?", r.ServerID, r.PlayerID).Count(&count).Error; err != nil {
					return err
				}
				if count != int64(len(entries)) {
					return ErrLocalWalletConflict
				}
				return db.AccountsTable(tx, "role_wallet_migrations").Create(&db.RoleWalletMigration{AccountID: a.ID, ServerID: r.ServerID, PlayerID: r.PlayerID, BalanceCents: b.BalanceCents, Revision: b.Revision, EntryCount: count}).Error
			}); err != nil {
				return err
			}
		}
		cursor = accounts[len(accounts)-1].ID
	}
}
