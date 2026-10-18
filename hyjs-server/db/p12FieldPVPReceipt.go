package db

// P12FieldPVPReceipt is immutable. A nullable stamp preserves original TCP0;
// only same-owner/same-target requests inside its six-second lease replay TCP0.
type P12FieldPVPReceipt struct {
	FightID      string `gorm:"primaryKey;size:64" json:"fightId"`
	PlayerID     int64  `gorm:"not null;index:p12_pk_owner,priority:1;uniqueIndex:p12_pk_stamp,priority:1" json:"playerId"`
	OpponentID   int64  `gorm:"not null;index:p12_pk_peer,priority:1" json:"opponentId"`
	Stamp        *int64 `gorm:"uniqueIndex:p12_pk_stamp,priority:2" json:"stamp"`
	FightType    int32  `gorm:"not null" json:"fightType"`
	RequestHash  string `gorm:"size:64;not null" json:"requestHash"`
	PolicyHash   string `gorm:"size:64;not null" json:"policyHash"`
	ScopeProof   string `gorm:"size:64;not null" json:"scopeProof"`
	P6RecordID   string `gorm:"size:64;not null;uniqueIndex" json:"p6RecordId"`
	WinnerID     int64  `gorm:"not null" json:"winnerId"`
	LootCoin     int64  `gorm:"not null" json:"lootCoin"`
	Response     []byte `gorm:"type:longblob;not null" json:"-"`
	ResponseHash string `gorm:"size:64;not null" json:"responseHash"`
	Terminal     []byte `gorm:"type:longblob;not null" json:"-"`
	TerminalHash string `gorm:"size:64;not null" json:"terminalHash"`
	Replay       []byte `gorm:"type:longblob;not null" json:"-"`
	ReplayHash   string `gorm:"size:64;not null" json:"replayHash"`
	MV           []byte `gorm:"type:longblob;not null" json:"-"`
	MVHash       string `gorm:"size:64;not null" json:"mvHash"`
	CreatedAt    int64  `gorm:"autoCreateTime:false;not null;index:p12_pk_owner,priority:2;index:p12_pk_peer,priority:2" json:"createdAt"`
	BusyUntil    int64  `gorm:"not null" json:"busyUntil"`
}

func (P12FieldPVPReceipt) TableName() string { return "p12_field_pvp_receipts" }
