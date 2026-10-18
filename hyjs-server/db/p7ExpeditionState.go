package db

type P7ExpeditionState struct {
	PlayerID    int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Level       string `gorm:"type:varchar(8);not null" json:"level"`
	Exp         int32  `gorm:"not null" json:"exp"`
	Day         string `gorm:"type:char(10);not null" json:"day"`
	RefreshUsed int32  `gorm:"not null" json:"refreshUsed"`
}

func (P7ExpeditionState) TableName() string { return "p7_expedition_states" }
