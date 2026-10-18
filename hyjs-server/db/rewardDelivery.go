package db

// RewardDelivery 冻结发奖来源和内容；玩家与来源唯一，领取与角色存档在同一事务提交。
type RewardDelivery struct {
	ID        int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	PlayerID  int64  `gorm:"not null;uniqueIndex:p1_reward_source,priority:1;index" json:"playerId"`
	SourceKey string `gorm:"type:varbinary(191);size:191;not null;uniqueIndex:p1_reward_source,priority:2" json:"sourceKey"`
	Title     string `gorm:"size:191;not null" json:"title"`
	Content   string `gorm:"type:text;not null" json:"content"`
	Rewards   string `gorm:"type:text;not null" json:"rewards"`
	CreatedAt int64  `gorm:"not null" json:"createdAt"`
	ExpiresAt int64  `gorm:"not null" json:"expiresAt"`
	ClaimedAt int64  `gorm:"not null" json:"claimedAt"`
	Result    []byte `gorm:"type:longblob" json:"-"`
}

func (RewardDelivery) TableName() string { return "p1_reward_deliveries" }
