package db

// P13UnionFightSetting 持久保存原自动宣战选择，关闭报名门禁不会丢失用户选择。
type P13UnionFightSetting struct {
	GuildID   string `gorm:"primaryKey;type:varchar(64)" json:"guildId"`
	AutoApply bool   `gorm:"not null;default:false" json:"autoApply"`
}

func (P13UnionFightSetting) TableName() string { return "p13_union_fight_settings" }
