package db

// P8LifeSkill 保存通过原生活技能书实际学习的技能，配方按此资格开放。
type P8LifeSkill struct {
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	SkillID   string `gorm:"primaryKey;size:64" json:"skillId"`
	BookID    string `gorm:"size:64;not null" json:"bookId"`
	LearnedAt int64  `gorm:"not null" json:"learnedAt"`
	Source    string `gorm:"type:longtext;not null" json:"-"`
}

func (P8LifeSkill) TableName() string { return "p8_life_skills" }
