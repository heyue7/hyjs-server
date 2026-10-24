package db

// GameAccountSession stores only a digest of a pre-realm account credential.
type GameAccountSession struct {
	TokenHash    string `gorm:"size:64;primaryKey" json:"-"`
	AccountID    int64  `gorm:"not null;index:game_account_sessions_account" json:"-"`
	AuthRevision int64  `gorm:"not null" json:"-"`
	CreatedAt    int64  `gorm:"not null;autoCreateTime:false" json:"-"`
	ExpiresAt    int64  `gorm:"not null;index:game_account_sessions_expiry" json:"-"`
}

func (GameAccountSession) TableName() string { return "game_account_sessions" }
