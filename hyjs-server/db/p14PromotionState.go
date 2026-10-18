package db

// P14PromotionState 只保存真实本服晋级资格对应的匹配点和原初始分，不写诊断胜负积分。
type P14PromotionState struct {
	SeasonIndex       int32  `gorm:"primaryKey;autoIncrement:false" json:"season_index"`
	ServerID          int64  `gorm:"primaryKey;autoIncrement:false" json:"server_id"`
	PlayerID          int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	QualificationHash string `gorm:"type:char(64);not null" json:"qualification_hash"`
	CalendarHash      string `gorm:"type:char(64);not null" json:"calendar_hash"`
	StageStart        int64  `gorm:"not null" json:"stage_start"`
	StageEnd          int64  `gorm:"not null" json:"stage_end"`
	EligibleAt        int64  `gorm:"not null" json:"eligible_at"`
	RulesJSON         string `gorm:"type:longtext;not null" json:"rules_json"`
	RulesHash         string `gorm:"type:char(64);not null" json:"rules_hash"`
	Points            int64  `gorm:"not null" json:"points"`
	Score             int64  `gorm:"not null" json:"score"`
	LastGrantAt       int64  `gorm:"not null" json:"last_grant_at"`
	CreatedAt         int64  `gorm:"not null;autoCreateTime:false" json:"created_at"`
	UpdatedAt         int64  `gorm:"not null;autoUpdateTime:false" json:"updated_at"`
	SourceHash        string `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P14PromotionState) TableName() string { return "p14_promotion_states" }
