package db

// P9PetRecruitRule 记录原服奖池缺失时可审计的独立兼容配置。
type P9PetRecruitRule struct {
	ID      string `gorm:"primaryKey;size:32" json:"id"`
	Enabled bool   `gorm:"not null" json:"enabled"`
	Pool    []byte `gorm:"type:longblob;not null" json:"pool"`
	Source  string `gorm:"type:text;not null" json:"source"`
}

func (P9PetRecruitRule) TableName() string { return "p9_pet_recruit_rules" }
