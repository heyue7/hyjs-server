package db

// P11PlunderFightSettlement owns one accepted typed3 result and resource transfer.
type P11PlunderFightSettlement struct {
	PlayerID            int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	Stamp               int64  `gorm:"primaryKey;autoIncrement:false" json:"stamp"`
	OpponentID          int64  `gorm:"not null" json:"opponentId"`
	FragmentID          string `gorm:"type:varchar(64);not null" json:"fragmentId"`
	RequestHash         string `gorm:"type:varchar(64);not null" json:"requestHash"`
	FightID             string `gorm:"type:varchar(64);not null;uniqueIndex" json:"fightId"`
	MatchID             string `gorm:"type:varchar(64);not null;uniqueIndex" json:"matchId"`
	MatchHash           string `gorm:"type:varchar(64);not null" json:"matchHash"`
	P6RecordID          string `gorm:"type:varchar(64);not null;uniqueIndex" json:"p6RecordId"`
	ScopeProof          string `gorm:"type:varchar(64);not null" json:"scopeProof"`
	OfferHash           string `gorm:"type:varchar(64);not null" json:"offerHash"`
	AttackerWon         bool   `gorm:"not null" json:"attackerWon"`
	Draw                int64  `gorm:"not null" json:"draw"`
	DropPPM             int64  `gorm:"not null" json:"dropPPM"`
	FragmentTransferred bool   `gorm:"not null" json:"fragmentTransferred"`
	EnergySpent         int32  `gorm:"not null" json:"energySpent"`
	RewardCoin          int32  `gorm:"not null" json:"rewardCoin"`
	RewardExp           int32  `gorm:"not null" json:"rewardExp"`
	Response            []byte `gorm:"type:longblob;not null" json:"response"`
	ResponseHash        string `gorm:"type:varchar(64);not null" json:"responseHash"`
	Terminal            []byte `gorm:"type:longblob;not null" json:"terminal"`
	TerminalHash        string `gorm:"type:varchar(64);not null" json:"terminalHash"`
	CreatedAt           int64  `gorm:"not null" json:"createdAt"`
}

func (P11PlunderFightSettlement) TableName() string { return "p11_plunder_fight_settlements" }
