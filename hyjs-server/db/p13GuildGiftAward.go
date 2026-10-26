package db

// P13GuildGiftAward belongs only to authenticated ordinary guild monthly gifts.
type P13GuildGiftAward struct {
	ID          string `gorm:"primaryKey;type:char(64);index:idx_guild_gift_outbox,priority:2" json:"id"`
	PlayerID    int64  `gorm:"not null;index:idx_guild_gift_person,priority:1;uniqueIndex:idx_guild_gift_award,priority:1" json:"player_id"`
	GuildID     string `gorm:"type:varchar(64);not null;uniqueIndex:idx_guild_gift_award,priority:2" json:"guild_id"`
	Month       string `gorm:"type:char(7);not null;uniqueIndex:idx_guild_gift_award,priority:3" json:"month"`
	Role        int32  `gorm:"not null" json:"role"`
	CutoffID    string `gorm:"type:char(64);not null" json:"cutoff_id"`
	CutoffHash  string `gorm:"type:char(64);not null" json:"cutoff_hash"`
	Rewards     string `gorm:"type:text;not null" json:"rewards"`
	SourceHash  string `gorm:"type:char(64);not null" json:"source_hash"`
	DeliveredAt int64  `gorm:"not null;default:0;index:idx_guild_gift_outbox,priority:1;index:idx_guild_gift_person,priority:2" json:"delivered_at"`
	DeliveryID  int64  `gorm:"not null;default:0" json:"delivery_id"`
}

func (P13GuildGiftAward) TableName() string { return "p13_guild_gift_awards" }
