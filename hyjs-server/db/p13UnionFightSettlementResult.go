package db

// P13UnionFightSettlementResult 保存具名运营零战斗终局及其真实发奖来源。
type P13UnionFightSettlementResult struct {
	WinTaskPrestige  int32  `gorm:"not null;default:0" json:"win_task_prestige,omitempty"`
	LoseTaskPrestige int32  `gorm:"not null;default:0" json:"lose_task_prestige,omitempty"`
	MatchID          string `gorm:"primaryKey;type:char(64)" json:"match_id"`
	WinnerGuildID    string `gorm:"type:varchar(64);not null" json:"winner_guild_id"`
	LoserGuildID     string `gorm:"type:varchar(64);not null" json:"loser_guild_id"`
	LeftPower        int64  `gorm:"not null" json:"left_power"`
	RightPower       int64  `gorm:"not null" json:"right_power"`
	EndAt            int64  `gorm:"not null" json:"end_at"`
	SettledAt        int64  `gorm:"not null" json:"settled_at"`
	AwardAt          int64  `gorm:"not null" json:"award_at"`
	DeliveredAt      int64  `gorm:"not null;default:0" json:"delivered_at"`
	OfferHash        string `gorm:"type:char(64);not null" json:"offer_hash"`
	Awards           []byte `gorm:"type:longblob;not null" json:"awards"`
	MVP              []byte `gorm:"type:blob" json:"-"`
	Source           string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightSettlementResult) TableName() string { return "p13_union_fight_settlement_results" }
