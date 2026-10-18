package db

// P13UnionFightTransportScope marks new fields whose entire deposit lifecycle is observed.
// Old fields have no row and cannot receive a retrospective transport MVP.
type P13UnionFightTransportScope struct {
	MatchID   string `gorm:"primaryKey;type:char(64)" json:"matchId"`
	StartedAt int64  `gorm:"not null" json:"startedAt"`
	Source    string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightTransportScope) TableName() string { return "p13_union_fight_transport_scopes" }
