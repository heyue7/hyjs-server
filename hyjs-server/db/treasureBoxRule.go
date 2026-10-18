package db

// TreasureBoxRule 保存本地服箱子抽取解释，原 APK 权重作为输入。
type TreasureBoxRule struct {
	ItemID   string `gorm:"column:item_id;primaryKey;size:64" json:"itemId"`
	Enabled  bool   `gorm:"not null" json:"enabled"`
	Mode     string `gorm:"size:64;not null" json:"mode"`
	MaxBatch int32  `gorm:"not null" json:"maxBatch"`
	Source   string `gorm:"size:255;not null" json:"source"`
}

func (TreasureBoxRule) TableName() string { return "treasure_box_rules" }

// TreasureBoxReward 保存每个箱子可调整的奖励相对权重。
type TreasureBoxReward struct {
	ItemID     string `gorm:"column:item_id;primaryKey;size:64" json:"itemId"`
	ID         int64  `gorm:"column:id;primaryKey;autoIncrement:false" json:"id"`
	ResourceID string `gorm:"column:resource_id;size:64;not null" json:"resourceId"`
	Amount     int32  `gorm:"not null" json:"amount"`
	Weight     int64  `gorm:"not null" json:"weight"`
	Source     string `gorm:"size:255;not null" json:"source"`
}

func (TreasureBoxReward) TableName() string { return "treasure_box_rewards" }
