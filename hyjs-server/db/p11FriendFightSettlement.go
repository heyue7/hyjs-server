package db

// P11FriendFightSettlement preserves a completed public original friend duel.
// It is separate from diagnostic preparation and never changes ranks/resources.
type P11FriendFightSettlement struct {
	PlayerID     int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	Stamp        int64  `gorm:"primaryKey;autoIncrement:false" json:"stamp"`
	OpponentID   int64  `gorm:"not null;index" json:"opponent_id"`
	FightID      string `gorm:"type:varchar(64);not null;uniqueIndex" json:"fight_id"`
	P6RecordID   string `gorm:"type:varchar(64);not null;uniqueIndex" json:"p6_record_id"`
	RequestHash  string `gorm:"type:varchar(64);not null" json:"request_hash"`
	ScopeProof   string `gorm:"type:varchar(64);not null" json:"scope_proof"`
	Response     []byte `gorm:"type:longblob;not null" json:"response"`
	ResponseHash string `gorm:"type:varchar(64);not null" json:"response_hash"`
	Terminal     []byte `gorm:"type:longblob;not null" json:"terminal"`
	TerminalHash string `gorm:"type:varchar(64);not null" json:"terminal_hash"`
	CreatedAt    int64  `gorm:"not null" json:"created_at"`
}

func (P11FriendFightSettlement) TableName() string { return "p11_friend_fight_settlements" }
