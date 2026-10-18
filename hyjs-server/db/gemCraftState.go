package db

// GemCraftState 保存冶炼等级和当前等级经验，默认等级 1 与原客户端的生活技能入口一致。
type GemCraftState struct {
	PlayerID int64 `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Level    int32 `gorm:"not null" json:"level"`
	Exp      int32 `gorm:"not null" json:"exp"`
}

func (GemCraftState) TableName() string { return "gem_craft_states" }
