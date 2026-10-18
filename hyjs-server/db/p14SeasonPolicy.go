package db

// P14SeasonPolicy 使用运营配置的真实赛季与报名窗口，未配置默认关闭。
type P14SeasonPolicy struct {
	ID            int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled       bool   `gorm:"not null;default:false" json:"enabled"`
	SeasonIndex   int32  `gorm:"not null;default:0" json:"seasonIndex"`
	RegisterStart int64  `gorm:"not null;default:0" json:"registerStart"`
	RegisterEnd   int64  `gorm:"not null;default:0" json:"registerEnd"`
	Source        string `gorm:"type:text;not null" json:"source"`
}

func (P14SeasonPolicy) TableName() string { return "p14_season_policies" }
