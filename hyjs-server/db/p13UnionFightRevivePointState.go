package db

// P13UnionFightRevivePointState 保存实际原阵营进度/占领点和前次观察身份，不追补离线时段。
type P13UnionFightRevivePointState struct {
	MatchID      string `gorm:"primaryKey;type:varchar(64)" json:"match_id"`
	PointGUID    int64  `gorm:"primaryKey;autoIncrement:false" json:"point_guid"`
	Point        []byte `gorm:"type:longblob;not null" json:"point"`
	Participants []byte `gorm:"type:longblob;not null" json:"participants"`
	LastAt       int64  `gorm:"not null" json:"last_at"`
	OfferHash    string `gorm:"type:char(64);not null" json:"offer_hash"`
	StateHash    string `gorm:"type:char(64);not null" json:"state_hash"`
}

func (P13UnionFightRevivePointState) TableName() string { return "p13_union_fight_revive_point_states" }
