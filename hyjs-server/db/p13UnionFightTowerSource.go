package db

// P13UnionFightTowerSource publishes only the previously unknown tower damage.
// Missing, disabled or late publications never enable ambient combat.
type P13UnionFightTowerSource struct {
	PlayerAttack     int64  `gorm:"not null;default:0" json:"player_attack,omitempty"`
	PlayerDamageRule string `gorm:"type:varchar(64);not null;default:''" json:"player_damage_rule,omitempty"`
	Day              string `gorm:"primaryKey;type:char(10)" json:"day"`
	TowerID          int64  `gorm:"primaryKey;autoIncrement:false" json:"tower_id"`
	Enabled          bool   `gorm:"not null;default:false" json:"enabled"`
	Version          string `gorm:"type:varchar(64);not null" json:"version"`
	HPPercent        int64  `gorm:"not null" json:"hp_percent"`
	PublishedAt      int64  `gorm:"not null;autoCreateTime:false" json:"published_at"`
	Source           string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightTowerSource) TableName() string { return "p13_union_fight_tower_sources" }
