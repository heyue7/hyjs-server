package db

import "embed"

type RoleWalletMigration struct {
	AccountID    int64 `gorm:"primaryKey;autoIncrement:false" json:"accountId"`
	ServerID     int64 `gorm:"not null" json:"serverId"`
	PlayerID     int64 `gorm:"not null" json:"playerId"`
	BalanceCents int64 `gorm:"not null" json:"balanceCents"`
	Revision     int64 `gorm:"not null" json:"revision"`
	EntryCount   int64 `gorm:"not null" json:"entryCount"`
}

func (RoleWalletMigration) TableName() string { return "role_wallet_migrations" }

//go:embed sql/role_wallet_*.sql sql/terminal_role_*.sql
var roleWalletSQL embed.FS

func RoleWalletSchemaSQL(name string) ([]byte, error) {
	return roleWalletSQL.ReadFile("sql/" + name + ".sql")
}
