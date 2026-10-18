package db

// P13UnionFightPlayerBattleOffer retains the pre-entry match-bound policy and original rule values; no old audit offer is upgraded.
type P13UnionFightPlayerBattleOffer struct {
	MatchID    string `gorm:"primaryKey;type:char(64)" json:"match_id"`
	Snapshot   string `gorm:"type:longtext;not null" json:"snapshot"`
	SourceHash string `gorm:"type:char(64);not null" json:"source_hash"`
	CapturedAt int64  `gorm:"not null" json:"captured_at"`
}

func (P13UnionFightPlayerBattleOffer) TableName() string {
	return "p13_union_fight_player_battle_offers"
}
