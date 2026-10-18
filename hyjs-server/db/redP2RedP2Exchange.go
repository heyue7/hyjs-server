package db

// RedP2Exchange 保存 P2 原协议业务状态；运营未知规则在 Source 明示本地兼容。
type RedP2Exchange struct {
	ID         int32  `gorm:"primaryKey;autoIncrement:false" json:"iD"`
	ResourceID string `gorm:"size:64" json:"resourceID"`
	Amount     int32  `json:"amount"`
	Cost       string `gorm:"size:128" json:"cost"`
	DailyLimit int32  `json:"dailyLimit"`
	Stock      int32  `json:"stock"`
	Used       int32  `json:"used"`
	MinLevel   int32  `json:"minLevel"`
	Enabled    bool
	Source     string `gorm:"size:255" json:"source"`
}

func (RedP2Exchange) TableName() string { return "p2_red_exchanges" }
