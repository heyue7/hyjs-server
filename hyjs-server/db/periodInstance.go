package db

// PeriodInstance 为独立活动窗口，发布后使用方须新建ID，已有玩家规则快照保持。
type PeriodInstance struct {
	ID          string `gorm:"type:varbinary(64);primaryKey;size:64" json:"id"`
	Enabled     bool   `gorm:"not null" json:"enabled"`
	Title       string `gorm:"size:191;not null" json:"title"`
	Kind        string `gorm:"size:64;not null" json:"kind"`
	Mode        string `gorm:"size:16;not null" json:"mode"`
	Goal        int64  `gorm:"not null" json:"goal"`
	Milestones  []byte `gorm:"type:longblob" json:"milestones,omitempty"` // Milestones freezes optional ordered goals and rewards; empty keeps the legacy single goal.
	Rewards     string `gorm:"type:text;not null" json:"rewards"`
	StartAt     int64  `gorm:"not null" json:"startAt"`
	EndAt       int64  `gorm:"not null" json:"endAt"`
	RewardEndAt int64  `gorm:"not null" json:"rewardEndAt"`
	RankPlaces  int32  `gorm:"not null" json:"rankPlaces"`
	RankRewards string `gorm:"type:text;not null" json:"rankRewards"`
	Frozen      []byte `gorm:"type:longblob" json:"-"`
	Source      string `gorm:"type:text;not null" json:"source"`
}

func (PeriodInstance) TableName() string { return "p1_period_instances" }
