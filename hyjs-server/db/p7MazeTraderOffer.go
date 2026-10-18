package db

// P7MazeTraderOffer 冻结真实失败来源和一次出现抽签，重启或查询不重新抽选。
type P7MazeTraderOffer struct {
	PlayerID    int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	RunID       int64  `gorm:"primaryKey;autoIncrement:false" json:"runId"`
	FightID     string `gorm:"size:64;not null" json:"fightId"`
	StartHash   string `gorm:"size:64;not null" json:"startHash"`
	ResultHash  string `gorm:"size:64;not null" json:"resultHash"`
	Policy      []byte `gorm:"type:longblob;not null" json:"-"`
	PolicyHash  string `gorm:"size:64;not null" json:"policyHash"`
	Draw        int32  `gorm:"not null" json:"draw"`
	Appeared    bool   `gorm:"not null" json:"appeared"`
	FrozenAt    int64  `gorm:"not null" json:"frozenAt"`
	PurchasedAt int64  `gorm:"not null" json:"purchasedAt"`
	BuyStamp    int64  `gorm:"not null" json:"buyStamp"`
}

func (P7MazeTraderOffer) TableName() string { return "p7_maze_trader_offers" }
