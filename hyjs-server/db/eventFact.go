package db

// EventFact 保存已提交业务事实；同源同种类唯一，不把无操作ID的合法重复消费去重。
type EventFact struct {
	ID         int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	PlayerID   int64  `gorm:"not null;uniqueIndex:p1_event_source,priority:1;index" json:"playerId"`
	SourceKey  string `gorm:"type:varbinary(191);size:191;not null;uniqueIndex:p1_event_source,priority:2" json:"sourceKey"`
	Kind       string `gorm:"type:varbinary(64);size:64;not null;uniqueIndex:p1_event_source,priority:3;index" json:"kind"`
	Amount     int64  `gorm:"not null" json:"amount"`
	OccurredAt int64  `gorm:"not null;index" json:"occurredAt"`
	Day        string `gorm:"size:10;not null" json:"day"`
}

func (EventFact) TableName() string { return "p1_event_facts" }
