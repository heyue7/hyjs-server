package db

// P10ScenePolicy 透明本地有限线路运营值与默认门禁。
type P10ScenePolicy struct {
	ID         int64  `gorm:"primaryKey;autoIncrement:false"`
	Enabled    bool   `gorm:"not null"`
	LineCount  int32  `gorm:"not null"`
	MaxPlayers int32  `gorm:"not null"`
	Source     string `gorm:"type:text;not null"`
}

func (P10ScenePolicy) TableName() string { return "p10_scene_policies" }
