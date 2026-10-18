package db

// P12WantedInstance 保存一个自然发现的完整原任务及每部位当前血量。
type P12WantedInstance struct {
	ID             string  `gorm:"primaryKey;type:varchar(64)" json:"id"`
	ServerID       int64   `gorm:"index:idx_p12_wanted_visible;not null" json:"server_id"`
	FinderID       int64   `gorm:"index:idx_p12_wanted_owner;not null" json:"finder_id"`
	FinderName     string  `gorm:"type:varchar(128);not null" json:"finder_name"`
	WantedID       string  `gorm:"type:varchar(32);not null" json:"wanted_id"`
	Level          int64   `gorm:"not null" json:"level"`
	Difficulty     int64   `gorm:"not null" json:"difficulty"`
	State          string  `gorm:"type:varchar(16);index:idx_p12_wanted_visible;index:idx_p12_wanted_owner;not null" json:"state"`
	CreatedAt      int64   `gorm:"not null" json:"created_at"`
	ExpiresAt      int64   `gorm:"index:idx_p12_wanted_visible;not null" json:"expires_at"`
	SharedAt       int64   `gorm:"not null" json:"shared_at"`
	KilledAt       int64   `gorm:"not null" json:"killed_at"`
	KillerID       int64   `gorm:"not null" json:"killer_id"`
	CurrentHP      float64 `gorm:"not null" json:"current_hp"`
	MaxHP          float64 `gorm:"not null" json:"max_hp"`
	MonsterHPJSON  string  `gorm:"type:longtext;not null" json:"monster_hp_json"`
	DefinitionJSON string  `gorm:"type:longtext;not null" json:"definition_json"`
	DefinitionHash string  `gorm:"type:varchar(64);not null" json:"definition_hash"`
}

func (P12WantedInstance) TableName() string { return "p12_wanted_instances" }
