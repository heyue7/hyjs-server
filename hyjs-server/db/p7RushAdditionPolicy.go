package db

// P7RushAdditionPolicy 保存具名本地试炼三选一加成与发布窗口。
type P7RushAdditionPolicy struct {
	ID          int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled     bool   `gorm:"not null" json:"enabled"`
	Version     string `gorm:"size:64;not null" json:"version"`
	Source      string `gorm:"type:text;not null" json:"source"`
	PublishedAt int64  `gorm:"not null" json:"publishedAt"`
	StartAt     int64  `gorm:"not null" json:"startAt"`
	EndAt       int64  `gorm:"not null" json:"endAt"`
	ThreeOption string `gorm:"size:64;not null" json:"threeOption"`
	SixOption   string `gorm:"size:64;not null" json:"sixOption"`
	NineOption  string `gorm:"size:64;not null" json:"nineOption"`
}

func (P7RushAdditionPolicy) TableName() string { return "p7_rush_addition_policies" }
