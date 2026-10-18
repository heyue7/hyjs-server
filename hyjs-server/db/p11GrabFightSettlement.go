package db

// P11GrabFightSettlement binds the original response to the real previous session and independent mode19 result.
type P11GrabFightSettlement struct {
	PlayerID          int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	Stamp             int64  `gorm:"primaryKey;autoIncrement:false" json:"stamp"`
	DefenderID        int64  `gorm:"not null" json:"defender_id"`
	Position          int64  `gorm:"not null" json:"position"`
	Day               string `gorm:"type:varchar(10);not null" json:"day"`
	RequestHash       string `gorm:"type:varchar(64);not null" json:"request_hash"`
	PreviousSessionID string `gorm:"type:varchar(64);not null" json:"previous_session_id"`
	PreviousStartedAt int64  `gorm:"not null" json:"previous_started_at"`
	PreviousExpiresAt int64  `gorm:"not null" json:"previous_expires_at"`
	NewSessionID      string `gorm:"type:varchar(64);not null" json:"new_session_id"`
	OfferVersion      string `gorm:"type:varchar(64);not null" json:"offer_version"`
	OfferHash         string `gorm:"type:varchar(64);not null" json:"offer_hash"`
	FightID           string `gorm:"type:varchar(64);uniqueIndex;not null" json:"fight_id"`
	P6RecordID        string `gorm:"type:varchar(64);uniqueIndex;not null" json:"p6_record_id"`
	ScopeProof        string `gorm:"type:varchar(64);not null" json:"scope_proof"`
	BeforeEnergy      int32  `gorm:"not null" json:"before_energy"`
	AfterEnergy       int32  `gorm:"not null" json:"after_energy"`
	BeforeRemaining   int32  `gorm:"not null" json:"before_remaining"`
	AfterRemaining    int32  `gorm:"not null" json:"after_remaining"`
	BeforeUsed        int32  `gorm:"not null" json:"before_used"`
	AfterUsed         int32  `gorm:"not null" json:"after_used"`
	Won               bool   `gorm:"not null" json:"won"`
	StartPB           []byte `gorm:"type:longblob;not null" json:"start_pb"`
	StartHash         string `gorm:"type:varchar(64);not null" json:"start_hash"`
	Terminal          []byte `gorm:"type:longblob;not null" json:"terminal"`
	TerminalHash      string `gorm:"type:varchar(64);not null" json:"terminal_hash"`
	Response          []byte `gorm:"type:longblob;not null" json:"response"`
	ResponseHash      string `gorm:"type:varchar(64);not null" json:"response_hash"`
	CreatedAt         int64  `gorm:"not null" json:"created_at"`
}

func (P11GrabFightSettlement) TableName() string { return "p11_grab_fight_settlements" }
