package db

// P11LimitDailyChallenge stores only published daily terms or actual accepted period point facts.
type P11LimitDailyChallenge struct {
	ID           int64  `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	FightID      string `gorm:"column:fight_id;type:varchar(64);not null;uniqueIndex" json:"fightId"`
	PeriodID     string `gorm:"column:period_id;type:varchar(64);not null;index" json:"periodId"`
	PlayerID     int64  `gorm:"column:player_id;not null;index" json:"playerId"`
	OpponentID   int64  `gorm:"column:opponent_id;not null" json:"opponentId"`
	AcceptedAt   int64  `gorm:"column:accepted_at;not null;index" json:"acceptedAt"`
	ResponseHash string `gorm:"column:response_hash;type:varchar(64);not null" json:"responseHash"`
	TerminalHash string `gorm:"column:terminal_hash;type:varchar(64);not null" json:"terminalHash"`
	P6RecordID   string `gorm:"column:p6_record_id;type:varchar(64);not null" json:"p6RecordId"`
	ScopeProof   string `gorm:"column:scope_proof;type:varchar(64);not null" json:"scopeProof"`
	SourceHash   string `gorm:"column:source_hash;type:varchar(64);not null" json:"sourceHash"`
}

func (P11LimitDailyChallenge) TableName() string { return "p11_limit_daily_challenges" }
