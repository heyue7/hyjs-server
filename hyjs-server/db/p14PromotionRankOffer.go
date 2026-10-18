package db

// P14PromotionRankOffer fixes the peer cohort and tie rule for a real season.
type P14PromotionRankOffer struct {
	SeasonIndex     int32  `gorm:"primaryKey;autoIncrement:false" json:"season_index"`
	ServerID        int64  `gorm:"primaryKey;autoIncrement:false" json:"server_id"`
	Version         string `gorm:"type:varchar(128);not null" json:"version"`
	Source          string `gorm:"type:varchar(1024);not null" json:"source"`
	RulesJSON       string `gorm:"type:longtext;not null" json:"rules_json"`
	RulesHash       string `gorm:"type:char(64);not null" json:"rules_hash"`
	PeerServersJSON string `gorm:"type:text;not null" json:"peer_servers_json"`
	CohortHash      string `gorm:"type:char(64);not null" json:"cohort_hash"`
	StageStart      int64  `gorm:"not null" json:"stage_start"`
	StageEnd        int64  `gorm:"not null" json:"stage_end"`
	PublishedAt     int64  `gorm:"not null" json:"published_at"`
	FrozenAt        int64  `gorm:"not null" json:"frozen_at"`
	SourceHash      string `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P14PromotionRankOffer) TableName() string { return "p14_promotion_rank_offers" }
