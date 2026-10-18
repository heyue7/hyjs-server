package db

type P5Collection struct {
	PlayerID   int64  `gorm:"primaryKey"`
	HeroID     string `gorm:"primaryKey;size:64"`
	AcquiredAt int64  `gorm:"not null;autoCreateTime:false"`
}

func (P5Collection) TableName() string { return "p5_collections" }
