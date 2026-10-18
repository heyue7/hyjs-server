package db

// P11LimitDailyAward stores only published daily terms or actual accepted period point facts.
type P11LimitDailyAward struct {
	OfferID      string `gorm:"column:offer_id;primaryKey;type:varchar(64)" json:"offerId"`
	PlayerID     int64  `gorm:"column:player_id;primaryKey;autoIncrement:false" json:"playerId"`
	PeriodID     string `gorm:"column:period_id;type:varchar(64);not null;index" json:"periodId"`
	Day          string `gorm:"column:day;type:varchar(8);not null" json:"day"`
	Rank         int64  `gorm:"column:rank;not null" json:"rank"`
	Bonus        int64  `gorm:"column:bonus;not null" json:"bonus"`
	BeforeScore  int64  `gorm:"column:before_score;not null" json:"beforeScore"`
	AfterScore   int64  `gorm:"column:after_score;not null" json:"afterScore"`
	SnapshotHash string `gorm:"column:snapshot_hash;type:varchar(64);not null" json:"snapshotHash"`
	SourceKey    string `gorm:"column:source_key;type:varchar(191);not null;uniqueIndex" json:"sourceKey"`
	SourceHash   string `gorm:"column:source_hash;type:varchar(64);not null" json:"sourceHash"`
	CreatedAt    int64  `gorm:"column:created_at;not null" json:"createdAt"`
}

func (P11LimitDailyAward) TableName() string { return "p11_limit_daily_awards" }
