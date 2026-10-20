package db

// AdminGuildSetting 保存后台已提交的公会运营覆盖，静态原表保持只读。
// Settings 只记录白名单参数、分级配置和策略字段，Version 由规范 JSON 摘要生成。
type AdminGuildSetting struct {
	ID        int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Settings  string `gorm:"type:longtext;not null" json:"settings"`
	Version   string `gorm:"type:char(64);not null;default:''" json:"version"`
	UpdatedAt int64  `gorm:"not null;default:0;autoUpdateTime:false" json:"updated_at"`
	UpdatedBy string `gorm:"type:varchar(64);not null;default:''" json:"updated_by"`
}

func (AdminGuildSetting) TableName() string { return "admin_guild_settings" }
