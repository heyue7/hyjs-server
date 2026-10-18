package db

// MembershipClaim 同一角色、福利与周期仅领取一次；奖励串保留审计来源。
type MembershipClaim struct {
	PlayerID int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	Kind     string `gorm:"primaryKey;size:128" json:"kind"`
	Period   string `gorm:"primaryKey;size:32" json:"period"`
	Reward   string `gorm:"type:text;not null" json:"reward"`
	At       int64  `gorm:"not null" json:"at"`
}

func (MembershipClaim) TableName() string { return "p3_membership_claims" }
