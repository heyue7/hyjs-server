package db

// P13GuildGiftCutoff belongs only to authenticated ordinary guild monthly gifts.
type P13GuildGiftCutoff struct {
	ID             string `gorm:"primaryKey;type:char(64)" json:"id"`
	GuildID        string `gorm:"type:varchar(64);not null;uniqueIndex:idx_guild_gift_cutoff,priority:1" json:"guild_id"`
	Month          string `gorm:"type:char(7);not null;uniqueIndex:idx_guild_gift_cutoff,priority:2" json:"month"`
	CutoffAt       int64  `gorm:"not null" json:"cutoff_at"`
	CapturedAt     int64  `gorm:"not null" json:"captured_at"`
	Level          int32  `gorm:"not null" json:"level"`
	Score          int64  `gorm:"not null" json:"score"`
	Qualified      bool   `gorm:"not null" json:"qualified"`
	RuleHash       string `gorm:"type:char(64);not null" json:"rule_hash"`
	HeadHash       string `gorm:"type:char(64);not null" json:"head_hash"`
	HeadRaw        []byte `gorm:"type:longblob;not null" json:"head_raw"`
	LastCreditID   string `gorm:"type:char(64);not null" json:"last_credit_id"`
	LastCreditHash string `gorm:"type:char(64);not null" json:"last_credit_hash"`
	Roster         []byte `gorm:"type:longblob;not null" json:"roster"`
	SourceHash     string `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P13GuildGiftCutoff) TableName() string { return "p13_guild_gift_cutoffs" }
