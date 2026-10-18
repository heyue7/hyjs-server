package db

// P11FriendDuel 保存原好友切磋真实双方冻结，内部准备不形成公共成功战斗。
type P11FriendDuel struct {
	ID                string `gorm:"primaryKey;type:varchar(64)"`
	ViewerID          int64  `gorm:"not null;index"`
	OpponentID        int64  `gorm:"not null;index"`
	Attacker          []byte `gorm:"type:longblob;not null"`
	Defender          []byte `gorm:"type:longblob;not null"`
	SnapshotHash      string `gorm:"type:varchar(64);not null"`
	ConfigurationHash string `gorm:"type:varchar(64);not null"`
	CapabilityHash    string `gorm:"type:varchar(64);not null"`
	Seed              int64  `gorm:"not null"`
	CreatedAt         int64  `gorm:"not null"`
	ExpiresAt         int64  `gorm:"not null;index"`
}

func (P11FriendDuel) TableName() string { return "p11_friend_duels" }
