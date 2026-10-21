package db

// TerminalSeedMigration records one-time default text migrations without exposing markers as operational content.
type TerminalSeedMigration struct {
	ID string `gorm:"size:64;primaryKey" json:"id"`
}

func (TerminalSeedMigration) TableName() string { return "terminal_seed_migrations" }
