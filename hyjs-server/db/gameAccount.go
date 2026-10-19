package db

// GameAccount stores global login credentials independently from realm role saves.
type GameAccount struct {
	ID           int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Username     string `gorm:"type:varbinary(1024);not null;uniqueIndex" json:"username"`
	Nickname     string `gorm:"size:96;not null" json:"nickname"`
	PasswordSalt []byte `gorm:"type:longblob;not null" json:"-"`
	PasswordHash []byte `gorm:"type:longblob;not null" json:"-"`
	Enabled      bool   `gorm:"not null" json:"enabled"`
	Protected    bool   `gorm:"not null" json:"protected"`
	AuthRevision int64  `gorm:"not null" json:"-"`
	Revision     int64  `gorm:"not null" json:"-"`
	CreatedAt    int64  `gorm:"not null;autoCreateTime:false" json:"createdAt"`
}

func (GameAccount) TableName() string { return "game_accounts" }
