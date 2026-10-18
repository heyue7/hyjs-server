package db

// P10ExpeditionHelpDaily 冻结当日实际奖励和完整VIP次数表；当天真实VIP升级仍选择相应权益。
type P10ExpeditionHelpDaily struct {
	PlayerID          int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Day               string `gorm:"primaryKey;size:10" json:"day"`
	Reward            string `gorm:"type:text;not null" json:"reward"`
	VIPLimits         []byte `gorm:"column:vip_limits;type:blob;not null" json:"-"`
	ConfigurationHash string `gorm:"type:char(64);not null" json:"configurationHash"`
	SourceHash        string `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (P10ExpeditionHelpDaily) TableName() string { return "p10_expedition_help_dailies" }
