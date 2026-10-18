package db

// P12WantedFight 绑定原Start、输入、实际终末、End和消耗；重复提交不二次扣血/发奖。
type P12WantedFight struct {
	ID             string  `gorm:"primaryKey;type:varchar(64)" json:"id"`
	PlayerID       int64   `gorm:"index:idx_p12_wanted_pending;not null" json:"player_id"`
	InstanceID     string  `gorm:"type:varchar(64);index;not null" json:"instance_id"`
	Stamp          int64   `gorm:"index;not null" json:"stamp"`
	AttackType     int64   `gorm:"not null" json:"attack_type"`
	Cost           int64   `gorm:"not null" json:"cost"`
	CreatedAt      int64   `gorm:"not null" json:"created_at"`
	Committed      bool    `gorm:"index:idx_p12_wanted_pending;not null" json:"committed"`
	CommittedAt    int64   `gorm:"index;not null" json:"committed_at"`
	AbandonedAt    int64   `gorm:"not null" json:"abandoned_at"`
	InputJSON      string  `gorm:"type:longtext;not null" json:"input_json"`
	InputHash      string  `gorm:"type:varchar(64);not null" json:"input_hash"`
	StartProto     []byte  `gorm:"type:longblob;not null" json:"start_proto"`
	StartHash      string  `gorm:"type:varchar(64);not null" json:"start_hash"`
	TerminalJSON   string  `gorm:"type:longtext;not null" json:"terminal_json"`
	TerminalHash   string  `gorm:"type:varchar(64);not null" json:"terminal_hash"`
	EndProto       []byte  `gorm:"type:longblob;not null" json:"end_proto"`
	EndHash        string  `gorm:"type:varchar(64);not null" json:"end_hash"`
	EndRequestHash string  `gorm:"type:varchar(64);not null" json:"end_request_hash"`
	AppliedDamage  float64 `gorm:"not null" json:"applied_damage"`
	Performance    int64   `gorm:"not null" json:"performance"`
	Score          int64   `gorm:"not null" json:"score"`
}

func (P12WantedFight) TableName() string { return "p12_wanted_fights" }
