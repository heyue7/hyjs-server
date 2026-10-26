package db

// P13UnionFightItemTeamUse is one match/camp/item counter, never a generic event table.
type P13UnionFightItemTeamUse struct {
	MatchID    string `gorm:"primaryKey;type:char(64)" json:"matchId"`
	Camp       int64  `gorm:"primaryKey;autoIncrement:false" json:"camp"`
	ItemID     string `gorm:"primaryKey;type:varchar(64)" json:"itemId"`
	Used       int64  `gorm:"not null" json:"used"`
	Limit      int64  `gorm:"column:use_limit;not null" json:"limit"`
	Rule       string `gorm:"type:varchar(64);not null" json:"rule"`
	OfferHash  string `gorm:"type:char(64);not null" json:"offerHash"`
	SourceHash string `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (P13UnionFightItemTeamUse) TableName() string { return "p13_union_fight_item_team_uses" }
