package db

// P7ReRushSection 保存每一轮每个三层区段的领奖和加成，星数由真实回执重核。
type P7ReRushSection struct {
	PlayerID       int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	RunID          int64  `gorm:"primaryKey;autoIncrement:false" json:"runId"`
	Section        int32  `gorm:"primaryKey;autoIncrement:false" json:"section"`
	Claimed        bool   `gorm:"not null" json:"claimed"`
	Selected       string `gorm:"size:64;not null" json:"selected"`
	SpentStars     int32  `gorm:"not null" json:"spentStars"`
	AcceptanceHash string `gorm:"size:64;not null" json:"acceptanceHash"`
}

func (P7ReRushSection) TableName() string { return "p7_re_rush_sections" }
