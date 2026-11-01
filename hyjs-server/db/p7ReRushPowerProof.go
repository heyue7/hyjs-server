package db

// P7ReRushPowerProof 冻结一次按阵容战力取得的三星扫荡资格。
type P7ReRushPowerProof struct {
	PlayerID     int64  `gorm:"primaryKey;autoIncrement:false;index:idx_p7_power_floor,priority:1" json:"playerId"`
	RunID        int64  `gorm:"primaryKey;autoIncrement:false;index:idx_p7_power_floor,priority:3" json:"runId"`
	Floor        int32  `gorm:"primaryKey;autoIncrement:false;index:idx_p7_power_floor,priority:2" json:"floor"`
	Snapshot     []byte `gorm:"type:longblob;not null" json:"-"`
	SnapshotHash string `gorm:"type:char(64);not null" json:"snapshotHash"`
}

func (P7ReRushPowerProof) TableName() string { return "p7_re_rush_power_proofs" }
