package db

// P11ArenaPrizePolicy is an explicitly published local source. Its private-server
// pool and caps are unknown; schema creation preserves operator values and stays off.
type P11ArenaPrizePolicy struct {
	ID          int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled     bool   `gorm:"not null" json:"enabled"`
	Version     string `gorm:"type:varchar(64);not null" json:"version"`
	Source      string `gorm:"type:text;not null" json:"source"`
	PoolJSON    string `gorm:"column:pool_json;type:longtext;not null" json:"poolJSON"`
	PublishedAt int64  `gorm:"not null" json:"publishedAt"`
	StartAt     int64  `gorm:"not null" json:"startAt"`
	EndAt       int64  `gorm:"not null" json:"endAt"`
	MinLevel    int64  `gorm:"not null" json:"minLevel"`
	DailyWins   int64  `gorm:"not null" json:"dailyWins"`
	WindowWins  int64  `gorm:"not null" json:"windowWins"`
}

func (P11ArenaPrizePolicy) TableName() string { return "p11_arena_prize_policies" }
