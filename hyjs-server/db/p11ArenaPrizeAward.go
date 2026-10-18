package db

// P11ArenaPrizeAward links one accepted arena/P6 terminal to its exact card
// projection and actual simple-resource grant. Zero cards never grant inventory.
type P11ArenaPrizeAward struct {
	FightID           string `gorm:"column:fight_id;type:varchar(128);primaryKey" json:"fightId"`
	PlayerID          int64  `gorm:"index:idx_p11_arena_prize_player_month,priority:1;not null" json:"playerId"`
	PlayerLevel       int64  `gorm:"not null" json:"playerLevel"`
	Stamp             int64  `gorm:"not null" json:"stamp"`
	WinnerID          int64  `gorm:"not null" json:"winnerId"`
	P6RecordID        string `gorm:"column:p6_record_id;type:varchar(128);not null" json:"p6RecordId"`
	TerminalHash      string `gorm:"type:varchar(64);not null" json:"terminalHash"`
	ScopeProof        string `gorm:"type:varchar(64);not null" json:"scopeProof"`
	BaseResponseHash  string `gorm:"type:varchar(64);not null" json:"baseResponseHash"`
	FinalResponseHash string `gorm:"type:varchar(64);not null" json:"finalResponseHash"`
	Month             string `gorm:"type:varchar(6);index:idx_p11_arena_prize_player_month,priority:2;not null" json:"month"`
	Day               string `gorm:"type:varchar(8);not null" json:"day"`
	Status            string `gorm:"type:varchar(32);not null" json:"status"`
	OfferHash         string `gorm:"type:varchar(64);not null" json:"offerHash"`
	DayOrdinal        int64  `gorm:"not null" json:"dayOrdinal"`
	WindowOrdinal     int64  `gorm:"not null" json:"windowOrdinal"`
	DrawJSON          string `gorm:"column:draw_json;type:text;not null" json:"drawJSON"`
	BoxesJSON         string `gorm:"column:boxes_json;type:text;not null" json:"boxesJSON"`
	GrantPB           []byte `gorm:"column:grant_pb;type:longblob" json:"grantPB"`
	SourceHash        string `gorm:"type:varchar(64);not null" json:"sourceHash"`
	CreatedAt         int64  `gorm:"autoCreateTime:false;not null" json:"createdAt"`
	RankBestBefore    int64  `gorm:"not null;default:0" json:"rankBestBefore,omitempty"`
	RankBestAfter     int64  `gorm:"not null;default:0" json:"rankBestAfter,omitempty"`
	RankDiamond       int64  `gorm:"not null;default:0" json:"rankDiamond,omitempty"`
	RankSource        string `gorm:"type:varchar(128);not null;default:''" json:"rankSource,omitempty"`
}

func (P11ArenaPrizeAward) TableName() string { return "p11_arena_prize_awards" }
