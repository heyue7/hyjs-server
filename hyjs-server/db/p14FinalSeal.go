package db

// P14FinalSeal retains a source realm's immutable post-lock member captures.
type P14FinalSeal struct {
	SeasonIndex int32  `gorm:"primaryKey;autoIncrement:false" json:"seasonIndex"`
	RoundStage  int32  `gorm:"primaryKey;autoIncrement:false" json:"roundStage"`
	ServerID    int64  `gorm:"primaryKey;autoIncrement:false" json:"serverId"`
	Content     []byte `gorm:"type:longblob;not null" json:"-"`
	ContentHash string `gorm:"type:char(64);not null" json:"contentHash"`
	ReceivedAt  int64  `gorm:"not null" json:"receivedAt"`
}

func (P14FinalSeal) TableName() string { return "p14_final_seals" }
