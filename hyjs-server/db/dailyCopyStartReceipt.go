package db

// DailyCopyStartReceipt 记录每个开始包；重新进入复用战斗时也保留包与战斗的映射。
type DailyCopyStartReceipt struct {
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	Stamp     int64  `gorm:"primaryKey;autoIncrement:false" json:"stamp"`
	ChapterID string `gorm:"size:64;not null" json:"chapter_id"`
	FightID   string `gorm:"size:64;not null;index" json:"fight_id"`
}

func (DailyCopyStartReceipt) TableName() string { return "daily_copy_start_receipts" }
