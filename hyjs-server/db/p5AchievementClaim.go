package db

type P5AchievementClaim struct {
	PlayerID  int64  `gorm:"primaryKey"`
	ID        string `gorm:"primaryKey;size:64"`
	Progress  int32  `gorm:"not null;default:0"`
	Snapshot  string `gorm:"type:longtext;not null"`
	ClaimedAt int64  `gorm:"not null;autoCreateTime:false"`
}

func (P5AchievementClaim) TableName() string { return "p5_achievement_claims" }
