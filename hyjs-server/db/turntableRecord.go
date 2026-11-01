package db

// TurntableRecord 只记录真实玩家的已提交奖励，按自增序号向客户端展示最新记录。
type TurntableRecord struct {
	ID              int64  `gorm:"primaryKey;autoIncrement;index:idx_turntable_record_owner,priority:3" json:"id"`
	PoolID          string `gorm:"column:pool_id;size:64;not null;index:idx_turntable_records_pool;index:idx_turntable_record_owner,priority:1;index:idx_turntable_record_reward,priority:1" json:"poolId"`
	PlayerID        int64  `gorm:"column:player_id;not null;index:idx_turntable_record_owner,priority:2;index:idx_turntable_record_reward,priority:2" json:"playerId"`
	PlayerName      string `gorm:"size:128;not null" json:"playerName"`
	ResourceID      string `gorm:"column:resource_id;size:64;not null;index:idx_turntable_record_reward,priority:3" json:"resourceId"`
	Amount          int32  `gorm:"not null" json:"amount"`
	Jackpot         bool   `gorm:"not null;index:idx_turntable_record_reward,priority:4" json:"jackpot"`
	CreatedAtMillis int64  `gorm:"not null" json:"createdAtMillis"`
}

func (TurntableRecord) TableName() string { return "turntable_records" }
