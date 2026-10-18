package db

// AdminUser 独立运营账号；密码只保存bcrypt摘要，不使用游戏账号凭据。
type AdminUser struct {
	ID            int64  `gorm:"primaryKey" json:"id"`
	Username      string `gorm:"type:varchar(64);not null;uniqueIndex" json:"username"`
	PasswordHash  string `gorm:"type:varchar(128);not null" json:"-"`
	Role          string `gorm:"type:varchar(16);not null" json:"role"`
	Enabled       bool   `gorm:"not null" json:"enabled"`
	CreatedAtUnix int64  `gorm:"not null" json:"createdAtUnix"`
}

func (AdminUser) TableName() string { return "admin_users" }

// AdminSession 不落库可重用的明文会话口令，CSRF摘要绑定同一会话。
type AdminSession struct {
	TokenHash     string `gorm:"type:char(64);primaryKey" json:"-"`
	UserID        int64  `gorm:"not null;index" json:"userId"`
	CSRFHash      string `gorm:"type:char(64);not null" json:"-"`
	ExpiresAtUnix int64  `gorm:"not null;index" json:"expiresAtUnix"`
}

func (AdminSession) TableName() string { return "admin_sessions" }

// AdminLoginThrottle 按规范化账号持久化失败窗口，重启继续执行冷却。
type AdminLoginThrottle struct {
	Username            string `gorm:"type:varchar(64);primaryKey" json:"username"`
	Failures            int64  `gorm:"not null" json:"failures"`
	WindowStartedAtUnix int64  `gorm:"not null" json:"windowStartedAtUnix"`
	LockedUntilUnix     int64  `gorm:"not null" json:"lockedUntilUnix"`
}

func (AdminLoginThrottle) TableName() string { return "admin_login_throttles" }
