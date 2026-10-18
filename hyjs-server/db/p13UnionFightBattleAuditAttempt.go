package db

// P13UnionFightBattleAuditAttempt 存实际Capture/Compute的非权威输入/结果关联和拒绝回执，不回滚审计也不改战争生命/分数。
type P13UnionFightBattleAuditAttempt struct {
	MatchID     string `gorm:"primaryKey;uniqueIndex:p13_battle_audit_receipt,priority:1;type:varchar(64)" json:"match_id"`
	GUID        int64  `gorm:"primaryKey;autoIncrement:false" json:"guid"`
	PlayerID    int64  `gorm:"not null;uniqueIndex:p13_battle_audit_receipt,priority:2" json:"player_id"`
	TargetID    int64  `gorm:"not null" json:"target_id"`
	Stamp       int64  `gorm:"not null;uniqueIndex:p13_battle_audit_receipt,priority:3" json:"stamp"`
	RequestHash string `gorm:"not null;type:char(64)" json:"request_hash"`
	OfferHash   string `gorm:"not null;type:char(64)" json:"offer_hash"`
	P6RecordID  string `gorm:"not null;type:char(64)" json:"p6_record_id"`
	InputHash   string `gorm:"not null;type:char(64)" json:"input_hash"`
	OutcomeHash string `gorm:"not null;type:char(64)" json:"outcome_hash"`
	ReplayHash  string `gorm:"not null;type:char(64)" json:"replay_hash"`
	Refusal     string `gorm:"not null;type:varchar(128)" json:"refusal"`
	SourceHash  string `gorm:"not null;type:char(64)" json:"source_hash"`
	CreatedAt   int64  `gorm:"not null" json:"created_at"`
}

func (P13UnionFightBattleAuditAttempt) TableName() string {
	return "p13_union_fight_battle_audit_attempts"
}
