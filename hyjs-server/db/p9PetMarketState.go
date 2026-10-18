package db

// P9PetMarketState 保存冻结周规则、已选商品与独立库存代次。
type P9PetMarketState struct {
	PlayerID     int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	WeekKey      string `gorm:"size:8;not null" json:"weekKey"`
	Input        []byte `gorm:"type:longblob;not null" json:"input"`
	InputHash    string `gorm:"size:64;not null" json:"inputHash"`
	Selected     []byte `gorm:"type:longblob;not null" json:"selected"`
	SelectedHash string `gorm:"size:64;not null" json:"selectedHash"`
	Bought       []byte `gorm:"type:longblob;not null" json:"bought"`
	Generation   int64  `gorm:"not null" json:"generation"`
	RefreshUsed  int32  `gorm:"not null" json:"refreshUsed"`
}

func (P9PetMarketState) TableName() string { return "p9_pet_market_states" }
