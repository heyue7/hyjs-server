package db

// CookingCraftState 将料理熟练度保存在独立行中，旧存档从一级开始。
type CookingCraftState struct {
	PlayerID int64 `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Level    int32 `gorm:"not null" json:"level"`
	Exp      int32 `gorm:"not null" json:"exp"`
}

func (CookingCraftState) TableName() string { return "cooking_craft_states" }
