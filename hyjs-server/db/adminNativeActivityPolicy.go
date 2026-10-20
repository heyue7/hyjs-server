package db

// AdminNativeActivityPolicy 保存没有独立原配置表的运营策略，例如会员福利档位。
type AdminNativeActivityPolicy struct {
	// ID 是稳定策略ID，例如vip-benefits。
	ID string `gorm:"primaryKey;size:64" json:"id"`
	// Form 是已校验的业务JSON，不保存浏览器原始表单或凭据。
	Form []byte `gorm:"type:longblob;not null" json:"form"`
	// Enabled 是true策略生效，false显示不可领并拒绝领取。
	Enabled bool `gorm:"not null" json:"enabled"`
	// Version 是保存时的业务摘要，便于审计；读取仍以Form重新计算乐观锁。
	Version string `gorm:"size:64;not null" json:"version"`
	// UpdatedAt 是最近保存毫秒。
	UpdatedAt int64 `gorm:"not null" json:"updatedAt"`
}

func (AdminNativeActivityPolicy) TableName() string { return "admin_native_activity_policies" }
