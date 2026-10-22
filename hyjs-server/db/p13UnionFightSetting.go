package db

// P13UnionFightSetting 仅保留旧表结构供一次性迁移读取，新代码不再创建或写入该表。
// 自动宣战选择已并入 P13Guild.UnionFightAutoApply；旧表暂存作上线核对与回退，不参与正式读写。
type P13UnionFightSetting struct {
	GuildID   string `gorm:"primaryKey;type:varchar(64)" json:"guildId"`
	AutoApply bool   `gorm:"not null;default:false" json:"autoApply"`
}

func (P13UnionFightSetting) TableName() string { return "p13_union_fight_settings" }
