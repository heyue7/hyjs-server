package db

type P7ExpeditionPolicy struct {
	ID      int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled bool   `gorm:"not null" json:"enabled"`
	Reward  string `gorm:"type:text;not null" json:"reward"`
	Source  string `gorm:"type:text;not null" json:"source"`
}

func (P7ExpeditionPolicy) TableName() string { return "p7_expedition_policies" }
