package db

// P13UnionFightMonthlyTitle is a durable cutoff recipient, independent of later
// guild changes. Effects and expiry are frozen from the original title rule.
type P13UnionFightMonthlyTitle struct {
	Month      string `gorm:"primaryKey;type:char(7)" json:"month"`
	PlayerID   int64  `gorm:"primaryKey;autoIncrement:false;index" json:"player_id"`
	TitleID    string `gorm:"primaryKey;type:varchar(32)" json:"title_id"`
	GuildID    string `gorm:"type:varchar(64);not null" json:"guild_id"`
	Rank       int32  `gorm:"not null" json:"rank"`
	Role       int32  `gorm:"not null" json:"role"`
	GrantedAt  int64  `gorm:"not null" json:"granted_at"`
	ExpiresAt  int64  `gorm:"not null" json:"expires_at"`
	Effects    string `gorm:"type:varchar(1024);not null" json:"effects"`
	ResultHash string `gorm:"type:char(64);not null" json:"result_hash"`
	Source     string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightMonthlyTitle) TableName() string { return "p13_union_fight_monthly_titles" }
