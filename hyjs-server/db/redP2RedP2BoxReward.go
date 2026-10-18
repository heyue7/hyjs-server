package db

// RedP2BoxReward 保存 P2 原协议业务状态；运营未知规则在 Source 明示本地兼容。
type RedP2BoxReward struct {
	BoxID      string `gorm:"primaryKey;size:64" json:"boxID"`
	ResourceID string `gorm:"primaryKey;size:64" json:"resourceID"`
	Amount     int32  `json:"amount"`
	Weight     int64  `json:"weight"`
	Source     string `gorm:"size:255" json:"source"`
}

func (RedP2BoxReward) TableName() string { return "p2_red_box_rewards" }
