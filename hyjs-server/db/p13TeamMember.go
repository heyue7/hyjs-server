package db

// P13TeamMember 单玩家主键阻止重复入队；退队保留代数使旧邀请无法复活。
type P13TeamMember struct {
	PlayerID   int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	TeamID     string `gorm:"type:varchar(64);not null;index" json:"teamId"`
	Generation int64  `gorm:"not null" json:"generation"`
	JoinedAt   int64  `gorm:"not null" json:"joinedAt"`
}

func (P13TeamMember) TableName() string { return "p13_team_members" }
