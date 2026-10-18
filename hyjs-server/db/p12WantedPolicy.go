package db

// P12WantedPolicy 冻结完整原表和具名本地触发/可玩内容范围；变更不改历史实例。
type P12WantedPolicy struct {
	ID             int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled        bool   `gorm:"not null" json:"enabled"`
	DefinitionJSON string `gorm:"type:longtext;not null" json:"definition_json"`
	DefinitionHash string `gorm:"type:varchar(64);not null" json:"definition_hash"`
	Source         string `gorm:"type:text;not null" json:"source"`
}

func (P12WantedPolicy) TableName() string { return "p12_wanted_policies" }
