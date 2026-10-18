package db

// P14AuditionMatch 保留实际本服海选双方与P6输入来源，只有权威结果可以应用淘汰计数。
type P14AuditionMatch struct {
	ID           string `gorm:"primaryKey;size:64" json:"id"`
	SeasonIndex  int32  `gorm:"not null;uniqueIndex:p14_audition_pair,priority:1" json:"season_index"`
	ServerID     int64  `gorm:"not null;uniqueIndex:p14_audition_pair,priority:2" json:"server_id"`
	RoundIndex   int64  `gorm:"not null;uniqueIndex:p14_audition_pair,priority:3" json:"round_index"`
	PairIndex    int64  `gorm:"not null;uniqueIndex:p14_audition_pair,priority:4" json:"pair_index"`
	AttackerID   int64  `gorm:"not null" json:"attacker_id"`
	DefenderID   int64  `gorm:"not null" json:"defender_id"`
	AttackerHash string `gorm:"size:64;not null" json:"attacker_hash"`
	DefenderHash string `gorm:"size:64;not null" json:"defender_hash"`
	Seed         int64  `gorm:"not null" json:"seed"`
	ReplayID     string `gorm:"size:64;not null" json:"replay_id"`
	InputHash    string `gorm:"size:64;not null" json:"input_hash"`
	CreatedAt    int64  `gorm:"autoCreateTime:false;not null" json:"created_at"`
	SourceHash   string `gorm:"size:64;not null" json:"source_hash"`
	Applied      bool   `gorm:"not null;default:0" json:"applied"`
	WinnerID     int64  `gorm:"not null;default:0" json:"winner_id"`
	OutcomeHash  string `gorm:"size:64;not null;default:''" json:"outcome_hash"`
}

func (P14AuditionMatch) TableName() string { return "p14_audition_matches" }
