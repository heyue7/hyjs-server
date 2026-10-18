package db

// P13Team 保存实际队长与授权代数；转让会撤销旧队长全部邀请。
type P13Team struct {
	ID               string `gorm:"primaryKey;type:varchar(64)" json:"id"`
	LeaderID         int64  `gorm:"not null" json:"leaderId"`
	MaxSize          int32  `gorm:"not null" json:"maxSize"`
	AuthorityVersion int64  `gorm:"not null" json:"authorityVersion"`
	Revision         int64  `gorm:"not null" json:"revision"`
	CreatedAt        int64  `gorm:"not null" json:"createdAt"`
}

func (P13Team) TableName() string { return "p13_teams" }
