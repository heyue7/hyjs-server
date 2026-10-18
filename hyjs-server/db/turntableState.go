package db

// TurntableState 保存每名真实玩家的冷却、积分以及最后一次已提交抽奖回执。
type TurntableState struct {
	PoolID       string `gorm:"column:pool_id;primaryKey;size:64" json:"poolId"`
	PlayerID     int64  `gorm:"column:player_id;primaryKey;autoIncrement:false" json:"playerId"`
	PlayerName   string `gorm:"size:128;not null" json:"playerName"`
	Score        int32  `gorm:"not null" json:"score"`
	BoughtCoins  int64  `gorm:"not null" json:"boughtCoins"`
	NextFree     int64  `gorm:"not null" json:"nextFree"`
	FreeDay      string `gorm:"size:10;not null" json:"freeDay"`
	FreeUsed     int32  `gorm:"not null" json:"freeUsed"`
	Timestamp    int64  `gorm:"not null" json:"timestamp"`
	RequestCount int32  `gorm:"not null" json:"requestCount"`
	Response     []byte `gorm:"type:longblob" json:"response"`
}

func (TurntableState) TableName() string { return "turntable_states" }
