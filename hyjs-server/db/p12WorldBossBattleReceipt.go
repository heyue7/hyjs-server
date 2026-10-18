package db

// P12WorldBossBattleReceipt 冻结服务端预计算；Committed仅由认证结束调用设置。
type P12WorldBossBattleReceipt struct {
	FightID         string `gorm:"primaryKey;type:varchar(64)" json:"fight_id"`
	PlayerID        int64  `gorm:"not null;uniqueIndex:p12_boss_stamp;index:p12_boss_pending" json:"player_id"`
	Stamp           *int64 `gorm:"uniqueIndex:p12_boss_stamp" json:"stamp,omitempty"`
	InstanceID      string `gorm:"type:varchar(64);not null;index;index:p12_boss_committed,priority:1" json:"instance_id"`
	PolicyName      string `gorm:"type:varchar(96);not null" json:"policy_name"`
	InputJSON       string `gorm:"type:longtext;not null" json:"input_json"`
	InputHash       string `gorm:"type:varchar(64);not null" json:"input_hash"`
	TerminalJSON    string `gorm:"type:longtext;not null" json:"terminal_json"`
	TerminalHash    string `gorm:"type:varchar(64);not null" json:"terminal_hash"`
	StartProto      []byte `gorm:"type:longblob;not null" json:"start_proto"`
	StartHash       string `gorm:"type:varchar(64);not null" json:"start_hash"`
	FinishProto     []byte `gorm:"type:longblob" json:"finish_proto,omitempty"`
	FinishHash      string `gorm:"type:varchar(64);not null" json:"finish_hash"`
	PredictedDamage int64  `gorm:"not null" json:"predicted_damage"`
	AppliedDamage   int64  `gorm:"not null" json:"applied_damage"`
	Committed       bool   `gorm:"not null;index:p12_boss_pending;index:p12_boss_committed,priority:2" json:"committed"`
	FinishHP        int64  `gorm:"not null" json:"finish_hp"`
	CreatedAt       int64  `gorm:"not null" json:"created_at"`
	FinishedAt      int64  `gorm:"not null" json:"finished_at"`
	AbandonedAt     int64  `gorm:"not null;default:0" json:"abandoned_at"`
	AbandonReason   string `gorm:"type:varchar(64);not null;default:''" json:"abandon_reason"`
}

func (P12WorldBossBattleReceipt) TableName() string { return "p12_world_boss_battle_receipts" }
