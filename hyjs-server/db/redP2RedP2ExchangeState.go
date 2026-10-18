package db

// RedP2ExchangeState 保存 P2 原协议业务状态；运营未知规则在 Source 明示本地兼容。
type RedP2ExchangeState struct {
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false" json:"playerID"`
	ProductID int32  `gorm:"primaryKey;autoIncrement:false" json:"productID"`
	Day       string `gorm:"primaryKey;size:10" json:"day"`
	Bought    int32  `json:"bought"`
}

func (RedP2ExchangeState) TableName() string { return "p2_red_exchange_states" }
