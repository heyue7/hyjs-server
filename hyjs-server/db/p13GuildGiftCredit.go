package db

// P13GuildGiftCredit belongs only to authenticated ordinary guild monthly gifts.
type P13GuildGiftCredit struct {
	ID           string `gorm:"primaryKey;type:char(64)" json:"id"`
	PlayerID     int64  `gorm:"not null;uniqueIndex:idx_guild_gift_player_day,priority:1" json:"player_id"`
	Day          string `gorm:"type:char(10);not null;uniqueIndex:idx_guild_gift_player_day,priority:2" json:"day"`
	GuildID      string `gorm:"type:varchar(64);not null;index:idx_guild_gift_credits,priority:1" json:"guild_id"`
	Month        string `gorm:"type:char(7);not null;index:idx_guild_gift_credits,priority:2" json:"month"`
	JoinedAt     int64  `gorm:"not null" json:"joined_at"`
	Amount       int32  `gorm:"not null" json:"amount"`
	BeforeScore  int64  `gorm:"not null" json:"before_score"`
	AfterScore   int64  `gorm:"not null" json:"after_score"`
	PreviousID   string `gorm:"type:char(64);not null" json:"previous_id"`
	PreviousHash string `gorm:"type:char(64);not null" json:"previous_hash"`
	RuleHash     string `gorm:"type:char(64);not null" json:"rule_hash"`
	OccurredAt   int64  `gorm:"not null" json:"occurred_at"`
	SourceHash   string `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P13GuildGiftCredit) TableName() string { return "p13_guild_gift_credits" }
