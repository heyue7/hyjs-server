package db

// P13UnionFightReviveReceipt permanently binds one natural revival to a settled
// death ordinal. Small before/after PBs are business proof, not replay payload.
type P13UnionFightReviveReceipt struct {
	MatchID            string `gorm:"primaryKey;type:char(64)" json:"match_id"`
	PlayerID           int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	DeadOrdinal        int64  `gorm:"primaryKey;autoIncrement:false" json:"dead_ordinal"`
	Status             string `gorm:"type:varchar(16);not null" json:"status"`
	Rule               string `gorm:"type:varchar(64);not null" json:"rule"`
	DeathKind          string `gorm:"type:varchar(16);not null" json:"death_kind"`
	DeathID            string `gorm:"type:varchar(128);not null" json:"death_id"`
	DeathSourceHash    string `gorm:"type:char(64);not null" json:"death_source_hash"`
	DeathAt            int64  `gorm:"not null" json:"death_at"`
	DueAt              int64  `gorm:"not null" json:"due_at"`
	AppliedAt          int64  `gorm:"not null" json:"applied_at"`
	Before             []byte `gorm:"type:blob;not null" json:"-"`
	After              []byte `gorm:"type:blob;not null" json:"-"`
	BeforeHash         string `gorm:"type:char(64);not null" json:"before_hash"`
	AfterHash          string `gorm:"type:char(64);not null" json:"after_hash"`
	BeforePosition     []byte `gorm:"type:blob;not null" json:"-"`
	AfterPosition      []byte `gorm:"type:blob;not null" json:"-"`
	BeforePositionHash string `gorm:"type:char(64);not null" json:"before_position_hash"`
	AfterPositionHash  string `gorm:"type:char(64);not null" json:"after_position_hash"`
	MatchHash          string `gorm:"type:char(64);not null" json:"match_hash"`
	FieldHash          string `gorm:"type:char(64);not null" json:"field_hash"`
	BattleHash         string `gorm:"type:char(64);not null" json:"battle_hash"`
	SourceHash         string `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P13UnionFightReviveReceipt) TableName() string { return "p13_union_fight_revive_receipts" }
