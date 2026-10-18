package db

// EquipmentRecycleRule 明确分解时原表之外的进阶返还解释。
type EquipmentRecycleRule struct {
	ID                int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled           bool   `gorm:"not null" json:"enabled"`
	ProgressionRefund string `gorm:"size:32;not null" json:"progressionRefund"`
	Source            string `gorm:"size:255;not null" json:"source"`
}

// TableName 指定运营返还策略表。
func (EquipmentRecycleRule) TableName() string { return "equipment_recycle_rules" }
