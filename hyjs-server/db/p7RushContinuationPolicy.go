package db

// P7RushContinuationPolicy 描述同一具名本地目录的发布边界，后继扩层复用该策略。
type P7RushContinuationPolicy struct {
	ID         int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled    bool   `gorm:"not null" json:"enabled"`
	Version    string `gorm:"size:64;not null" json:"version"`
	Source     string `gorm:"type:text;not null" json:"source"`
	FirstFloor int32  `gorm:"not null" json:"firstFloor"`
	LastFloor  int32  `gorm:"not null" json:"lastFloor"`
}

func (P7RushContinuationPolicy) TableName() string { return "p7_rush_continuation_policies" }
