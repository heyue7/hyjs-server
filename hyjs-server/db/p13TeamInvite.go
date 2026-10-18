package db

// P13TeamInvite 绑定队长授权代数和邀请双方实际成员代数；跨队/转让/退出撤权。
type P13TeamInvite struct {
	TeamID            string `gorm:"primaryKey;type:varchar(64)" json:"teamId"`
	TargetID          int64  `gorm:"primaryKey;autoIncrement:false" json:"targetId"`
	InviterID         int64  `gorm:"not null" json:"inviterId"`
	AuthorityVersion  int64  `gorm:"not null" json:"authorityVersion"`
	InviterGeneration int64  `gorm:"not null" json:"inviterGeneration"`
	TargetGeneration  int64  `gorm:"not null" json:"targetGeneration"`
	CreatedAt         int64  `gorm:"not null" json:"createdAt"`
}

func (P13TeamInvite) TableName() string { return "p13_team_invites" }
