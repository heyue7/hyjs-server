package db

type P5Achievement struct {
	Name     string `gorm:"type:varchar(320);not null;default:''"`
	Type     int32  `gorm:"not null;default:1"`
	Disabled bool   `gorm:"not null;default:false"`
	ID       string `gorm:"primaryKey;size:64"`
	Kind     string `gorm:"size:64;not null"`
	Target   int32  `gorm:"not null"`
	Reward   string `gorm:"type:text;not null"`
	Source   string `gorm:"type:text;not null"`
}

func (P5Achievement) TableName() string { return "p5_achievement_rules" }
