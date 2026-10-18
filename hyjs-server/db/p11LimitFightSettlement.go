package db

// P11LimitFightSettlement stores only current real mode20 publication, frozen input or accepted facts.
type P11LimitFightSettlement struct {
	PlayerID     int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	Stamp        int64  `gorm:"primaryKey;autoIncrement:false" json:"stamp"`
	OpponentID   int64  `gorm:"not null" json:"opponent_id"`
	PeriodID     string `gorm:"type:varchar(64);not null;index" json:"period_id"`
	RequestHash  string `gorm:"type:varchar(64);not null" json:"request__hash"`
	MatchID      string `gorm:"type:varchar(64);not null;uniqueIndex" json:"match_id"`
	MatchHash    string `gorm:"type:varchar(64);not null" json:"match__hash"`
	FightID      string `gorm:"type:varchar(64);not null;uniqueIndex" json:"fight_id"`
	P6RecordID   string `gorm:"type:varchar(64);not null;uniqueIndex" json:"p6__record_id"`
	ScopeProof   string `gorm:"type:varchar(64);not null" json:"scope__proof"`
	EnergySpent  int32  `gorm:"not null" json:"energy__spent"`
	ScoreDelta   int32  `gorm:"not null" json:"score__delta"`
	BeforeScore  int64  `gorm:"not null" json:"before__score"`
	AfterScore   int64  `gorm:"not null" json:"after__score"`
	Day          string `gorm:"type:varchar(10);not null" json:"day"`
	BeforeUsed   int32  `gorm:"not null" json:"before__used"`
	AfterUsed    int32  `gorm:"not null" json:"after__used"`
	Response     []byte `gorm:"type:longblob;not null" json:"response"`
	ResponseHash string `gorm:"type:varchar(64);not null" json:"response__hash"`
	Terminal     []byte `gorm:"type:longblob;not null" json:"terminal"`
	TerminalHash string `gorm:"type:varchar(64);not null" json:"terminal__hash"`
	CreatedAt    int64  `gorm:"not null" json:"created__at"`
}

func (P11LimitFightSettlement) TableName() string { return "p11_limit_fight_settlements" }
