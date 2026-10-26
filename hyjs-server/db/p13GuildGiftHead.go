package db

// P13GuildGiftHead belongs only to authenticated ordinary guild monthly gifts.
type P13GuildGiftHead struct {
	GuildID        string `gorm:"primaryKey;type:varchar(64);index:idx_guild_gift_due,priority:2" json:"guild_id"`
	Month          string `gorm:"type:char(7);not null" json:"month"`
	DueAt          int64  `gorm:"not null;index:idx_guild_gift_due,priority:1" json:"due_at"`
	Score          int64  `gorm:"not null" json:"score"`
	CreditCount    int64  `gorm:"not null" json:"credit_count"`
	LastCreditID   string `gorm:"type:char(64);not null" json:"last_credit_id"`
	LastCreditHash string `gorm:"type:char(64);not null" json:"last_credit_hash"`
	LastCreditAt   int64  `gorm:"not null" json:"last_credit_at,omitempty"`
	RuleHash       string `gorm:"type:char(64);not null" json:"rule_hash"`
	ArchiveHash    string `gorm:"type:char(64);not null" json:"archive_hash"`
	RetiredAt      int64  `gorm:"not null;default:0" json:"retired_at"`
	SourceHash     string `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P13GuildGiftHead) TableName() string { return "p13_guild_gift_heads" }
