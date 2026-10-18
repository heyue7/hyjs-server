package db

// P9PetRecruitProgress 随玩家首次开启冻结奖池，记录独立普通招募保底。
type P9PetRecruitProgress struct {
	PlayerID       int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Input          []byte `gorm:"type:longblob;not null" json:"input"`
	InputHash      string `gorm:"size:64;not null" json:"inputHash"`
	SinceGuarantee int32  `gorm:"not null" json:"sinceGuarantee"`
	Draws          int64  `gorm:"not null" json:"draws"`
	InitializedAt  int64  `gorm:"not null" json:"initializedAt"`
}

func (P9PetRecruitProgress) TableName() string { return "p9_pet_recruit_progress" }
