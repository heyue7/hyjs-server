package db

// CookingEffect 保存单一全局奖励料理，使用服务器时间结算和过期。
type CookingEffect struct {
	PlayerID int64   `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	ItemID   string  `gorm:"not null" json:"itemId"`
	Kind     string  `gorm:"not null" json:"kind"`
	EndAt    int64   `gorm:"not null" json:"endAt"`
	Value    float64 `gorm:"not null" json:"value"`
}

func (CookingEffect) TableName() string { return "cooking_effects" }
