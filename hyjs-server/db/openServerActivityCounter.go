package db

// OpenServerActivityCounter 只由成功业务事务记录，日统计绑定UTC+8日期键。
type OpenServerActivityCounter struct {
	PlayerID   int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	CounterKey string `gorm:"primaryKey;size:64" json:"counter_key"`
	Value      int32  `gorm:"not null" json:"value"`
}

func (OpenServerActivityCounter) TableName() string { return "open_server_activity_counters" }
