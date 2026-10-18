package db

// P11ArenaDailyChallenge records the actual serialized admission of a newly
// accepted challenge. Request timestamps do not determine daily eligibility.
type P11ArenaDailyChallenge struct {
	ID           int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	FightID      string `gorm:"type:varchar(64);not null;uniqueIndex" json:"fightId"`
	PlayerID     int64  `gorm:"not null;index" json:"playerId"`
	OpponentID   int64  `gorm:"not null;index" json:"opponentId"`
	AcceptedAt   int64  `gorm:"not null;index" json:"acceptedAt"`
	ResponseHash string `gorm:"type:varchar(64);not null" json:"responseHash"`
	TerminalHash string `gorm:"type:varchar(64);not null" json:"terminalHash"`
	P6RecordID   string `gorm:"type:varchar(64);not null" json:"p6RecordId"`
	ScopeProof   string `gorm:"type:varchar(64);not null" json:"scopeProof"`
	SourceHash   string `gorm:"type:varchar(64);not null" json:"sourceHash"`
}

func (P11ArenaDailyChallenge) TableName() string { return "p11_arena_daily_challenges" }
