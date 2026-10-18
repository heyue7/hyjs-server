package db

// P4PlayerStats 保存实际属性变化快照，重复登录或只读查询不制造战力事件。
type P4PlayerStats struct {
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Period    string `gorm:"type:varbinary(16);not null" json:"period"`
	LastPower int32  `gorm:"not null" json:"lastPower"`
}

func (P4PlayerStats) TableName() string { return "p4_player_stats" }
