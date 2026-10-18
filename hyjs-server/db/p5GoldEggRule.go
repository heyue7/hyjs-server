package db

// P5GoldEggRule freezes the declared local interpretation of all original egg rows.
type P5GoldEggRule struct {
	ID        string `gorm:"primaryKey;type:varbinary(64)" json:"id"`
	Enabled   bool   `gorm:"not null" json:"enabled"`
	Rules     []byte `gorm:"type:longblob;not null" json:"rules"`
	Frozen    []byte `gorm:"type:longblob" json:"frozen"`
	InputHash string `gorm:"type:char(64);not null" json:"inputHash"`
	Source    string `gorm:"type:text;not null" json:"source"`
}

func (P5GoldEggRule) TableName() string { return "p5_gold_egg_rules" }
