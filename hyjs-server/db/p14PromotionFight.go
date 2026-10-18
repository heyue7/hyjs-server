package db

// P14PromotionFight records one genuinely certified challenge and its point debit.
// The existing noon grant chain stays immutable; its points are projected with
// accepted spends since the latest grant, which restores the original cap20.
type P14PromotionFight struct {
	ID                    string `gorm:"primaryKey;type:char(64)" json:"id"`
	SeasonIndex           int32  `gorm:"not null;uniqueIndex:p14_fight_order,priority:1;uniqueIndex:p14_fight_stamp,priority:1" json:"season_index"`
	ServerID              int64  `gorm:"not null;uniqueIndex:p14_fight_order,priority:2;uniqueIndex:p14_fight_stamp,priority:2" json:"server_id"`
	PlayerID              int64  `gorm:"not null;uniqueIndex:p14_fight_order,priority:3;uniqueIndex:p14_fight_stamp,priority:3" json:"player_id"`
	MatchIndex            int64  `gorm:"not null;uniqueIndex:p14_fight_order,priority:4" json:"match_index"`
	Stamp                 int64  `gorm:"not null;uniqueIndex:p14_fight_stamp,priority:4" json:"stamp"`
	OpponentServerID      int64  `gorm:"not null" json:"opponent_server_id"`
	OpponentID            int64  `gorm:"not null" json:"opponent_id"`
	AttackerFormationHash string `gorm:"type:char(64);not null" json:"attacker_formation_hash"`
	DefenderFormationHash string `gorm:"type:char(64);not null" json:"defender_formation_hash"`
	QualificationHash     string `gorm:"type:char(64);not null" json:"qualification_hash"`
	OfferHash             string `gorm:"type:char(64);not null" json:"offer_hash"`
	GrantAt               int64  `gorm:"not null" json:"grant_at"`
	GrantHash             string `gorm:"type:char(64);not null" json:"grant_hash"`
	PointsBefore          int64  `gorm:"not null" json:"points_before"`
	PointsAfter           int64  `gorm:"not null" json:"points_after"`
	ScoreBefore           int64  `gorm:"not null" json:"score_before"`
	ScoreAfter            int64  `gorm:"not null" json:"score_after"`
	ScoreDelta            int64  `gorm:"not null" json:"score_delta"`
	WinnerID              int64  `gorm:"not null" json:"winner_id"`
	P6RecordID            string `gorm:"type:char(64);not null" json:"p6_record_id"`
	ScopeProof            string `gorm:"type:char(64);not null" json:"scope_proof"`
	Response              []byte `gorm:"type:longblob;not null" json:"-"`
	ResponseHash          string `gorm:"type:char(64);not null" json:"response_hash"`
	Terminal              []byte `gorm:"type:longblob;not null" json:"-"`
	TerminalHash          string `gorm:"type:char(64);not null" json:"terminal_hash"`
	CreatedAt             int64  `gorm:"not null;autoCreateTime:false" json:"created_at"`
	SourceHash            string `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P14PromotionFight) TableName() string { return "p14_promotion_fights" }
