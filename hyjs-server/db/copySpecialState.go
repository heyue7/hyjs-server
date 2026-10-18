package db

// CopySpecialState conserves successes = credit + spent across frozen cadence versions.
type CopySpecialState struct {
	PlayerID      int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	TaskID        string `gorm:"primaryKey;type:varbinary(32)" json:"taskId"`
	RewardID      string `gorm:"primaryKey;type:varbinary(64)" json:"rewardId"`
	Credit        int64  `gorm:"not null" json:"credit"`
	Successes     int64  `gorm:"not null" json:"successes"`
	Spent         int64  `gorm:"not null" json:"spent"`
	Granted       int64  `gorm:"not null" json:"granted"`
	LastInputHash string `gorm:"type:char(64);not null" json:"lastInputHash"`
}

func (CopySpecialState) TableName() string { return "copy_special_states" }
