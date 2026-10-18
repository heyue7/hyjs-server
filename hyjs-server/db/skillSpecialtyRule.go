package db

// SkillSpecialtyRule 记录 APK 缺失的初始状态及双强化解锁规则，并允许独立关闭。
type SkillSpecialtyRule struct {
	ID           string `gorm:"column:id;primaryKey;size:32" json:"id"`
	Enabled      bool   `gorm:"not null" json:"enabled"`
	InitialTypes string `gorm:"size:64;not null" json:"initialTypes"`
	UnlockPolicy string `gorm:"size:64;not null" json:"unlockPolicy"`
	Source       string `gorm:"type:text;not null" json:"source"`
}

func (SkillSpecialtyRule) TableName() string { return "skill_specialty_rules" }
