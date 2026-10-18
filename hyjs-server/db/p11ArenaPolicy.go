package db

// P11ArenaPolicy 原参数与明确本地真实名册策略，新竞争默认关闭。
type P11ArenaPolicy struct {
	ID          int64  `gorm:"primaryKey;autoIncrement:false"`
	Enabled     bool   `gorm:"not null"`
	DuelEnabled bool   `gorm:"not null"`
	NextRank    int64  `gorm:"not null"`
	MatchCount  int32  `gorm:"not null"`
	RankShow    int32  `gorm:"not null"`
	MatchMillis int64  `gorm:"not null"`
	Source      string `gorm:"type:text;not null"`
}

func (P11ArenaPolicy) TableName() string { return "p11_arena_policies" }
