package db

// P14PromotionFightOffer freezes the first accepted local season's score rules.
type P14PromotionFightOffer struct {
	SeasonIndex int32  `gorm:"primaryKey;autoIncrement:false" json:"season_index"`
	ServerID    int64  `gorm:"primaryKey;autoIncrement:false" json:"server_id"`
	Version     string `gorm:"type:varchar(128);not null" json:"version"`
	Source      string `gorm:"type:varchar(1024);not null" json:"source"`
	RulesJSON   string `gorm:"type:longtext;not null" json:"rules_json"`
	RulesHash   string `gorm:"type:char(64);not null" json:"rules_hash"`
	PublishedAt int64  `gorm:"not null" json:"published_at"`
	CapturedAt  int64  `gorm:"not null" json:"captured_at"`
	SourceHash  string `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P14PromotionFightOffer) TableName() string { return "p14_promotion_fight_offers" }
