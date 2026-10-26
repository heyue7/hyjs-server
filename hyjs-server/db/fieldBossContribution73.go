package db

// FieldBossContribution73 stores immutable spawn membership and one durable reward receipt.
type FieldBossContribution73 struct {
	ID         int64   `gorm:"primaryKey;autoIncrement" json:"id"`
	ServerID   int64   `gorm:"not null;uniqueIndex:uidx_boss73_member,priority:1;index:idx_boss73_pending,priority:1;index:idx_boss73_day,priority:1" json:"serverId"`
	SpawnUUID  string  `gorm:"type:varbinary(128);not null;uniqueIndex:uidx_boss73_member,priority:2" json:"spawnUuid"`
	PlayerID   int64   `gorm:"not null;uniqueIndex:uidx_boss73_member,priority:3;index:idx_boss73_pending,priority:2;index:idx_boss73_day,priority:2" json:"playerId"`
	Damage     float64 `gorm:"not null" json:"damage"`
	Rules      []byte  `gorm:"type:longblob;not null" json:"-"`
	Sprouts    int64   `gorm:"not null" json:"sprouts"`
	DailyLimit int64   `gorm:"not null" json:"dailyLimit"`
	Reward     string  `gorm:"type:text;not null" json:"reward"`
	RewardDay  string  `gorm:"type:char(10);not null;index:idx_boss73_day,priority:3" json:"rewardDay"`
	Rank       int64   `gorm:"column:reward_rank;not null" json:"rank"`
	Frozen     bool    `gorm:"not null;index:idx_boss73_pending,priority:3" json:"frozen"`
	Claimed    bool    `gorm:"not null;index:idx_boss73_pending,priority:4;index:idx_boss73_day,priority:4" json:"claimed"`
}

func (FieldBossContribution73) TableName() string { return "field_boss_contributions73" }
