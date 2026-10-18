package db

// P11ArenaFightSettlement commits one real scoped calculation, original response,
// exact Energy2 payment and ranking transition under the same player/stamp key.
type P11ArenaFightSettlement struct {
	PlayerID        int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	Stamp           int64  `gorm:"primaryKey;autoIncrement:false" json:"stamp"`
	OpponentID      int64  `gorm:"not null;index" json:"opponent_id"`
	FightID         string `gorm:"type:varchar(64);not null;uniqueIndex" json:"fight_id"`
	MatchID         string `gorm:"type:varchar(64);not null;index" json:"match_id"`
	MatchHash       string `gorm:"type:varchar(64);not null" json:"match_hash"`
	P6RecordID      string `gorm:"type:varchar(64);not null;uniqueIndex" json:"p6_record_id"`
	RequestHash     string `gorm:"type:varchar(64);not null" json:"request_hash"`
	ScopeProof      string `gorm:"type:varchar(64);not null" json:"scope_proof"`
	PolicyHash      string `gorm:"type:varchar(64);not null" json:"policy_hash"`
	WinnerID        int64  `gorm:"not null" json:"winner_id"`
	AttackerOldRank int64  `gorm:"not null" json:"attacker_old_rank"`
	DefenderOldRank int64  `gorm:"not null" json:"defender_old_rank"`
	AttackerNewRank int64  `gorm:"not null" json:"attacker_new_rank"`
	DefenderNewRank int64  `gorm:"not null" json:"defender_new_rank"`
	EnergySpent     int32  `gorm:"not null" json:"energy_spent"`
	Response        []byte `gorm:"type:longblob;not null" json:"response"`
	ResponseHash    string `gorm:"type:varchar(64);not null" json:"response_hash"`
	Terminal        []byte `gorm:"type:longblob;not null" json:"terminal"`
	TerminalHash    string `gorm:"type:varchar(64);not null" json:"terminal_hash"`
	CreatedAt       int64  `gorm:"not null" json:"created_at"`
}

func (P11ArenaFightSettlement) TableName() string { return "p11_arena_fight_settlements" }
