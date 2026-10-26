package db

// AdminSproutSetting 保存全局小芽资格配置，按角色创建时间判定。
type AdminSproutSetting struct {
	ID                  int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled             bool   `gorm:"not null;default:false" json:"enabled"`
	Days                *int64 `json:"days"`
	NewCharacterEnabled *bool  `json:"new_character_enabled"`
	ReturnEnabled       *bool  `json:"return_enabled"`
	ReturnOfflineDays   *int64 `json:"return_offline_days"`
	ReturnDays          *int64 `json:"return_days"`
	Revision            int64  `gorm:"not null;default:1" json:"revision"`
	UpdatedAt           int64  `gorm:"not null;default:0;autoUpdateTime:false" json:"updated_at"`
	UpdatedBy           string `gorm:"type:varchar(64);not null;default:''" json:"updated_by"`
}

func (AdminSproutSetting) TableName() string { return "admin_sprout_settings" }
