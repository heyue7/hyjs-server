package db

// P11ArenaDailyAward stores the independently published local daily reward source.
type P11ArenaDailyAward struct {
	Day          string `gorm:"type:varchar(8);primaryKey" json:"day"`
	PlayerID     int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Rank         int64  `gorm:"column:position;not null" json:"rank"`
	Rewards      string `gorm:"type:text;not null" json:"rewards"`
	SnapshotHash string `gorm:"type:varchar(64);not null" json:"snapshotHash"`
	RankProof    string `gorm:"type:varchar(64);not null" json:"rankProof"`
	DeliveryID   int64  `gorm:"not null" json:"deliveryId"`
	SourceKey    string `gorm:"type:varbinary(191);not null;uniqueIndex" json:"sourceKey"`
	SourceHash   string `gorm:"type:varchar(64);not null" json:"sourceHash"`
	CreatedAt    int64  `gorm:"autoCreateTime:false;not null" json:"createdAt"`
}

func (P11ArenaDailyAward) TableName() string { return "p11_arena_daily_awards" }
