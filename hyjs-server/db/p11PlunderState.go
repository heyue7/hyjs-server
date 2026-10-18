package db

// P11PlunderState 绝对免战截止跨重启/日切保持，协议查询换算剩余毫秒。
type P11PlunderState struct {
	PlayerID   int64  `gorm:"primaryKey;autoIncrement:false" json:"player_id"`
	AvoidUntil int64  `gorm:"not null" json:"avoid_until"`
	LastUseAt  int64  `gorm:"not null" json:"last_use_at"`
	LastItem   string `gorm:"type:varchar(64);not null" json:"last_item"`
	UpdatedAt  int64  `gorm:"autoUpdateTime:false;not null" json:"updated_at"`
}

func (P11PlunderState) TableName() string { return "p11_plunder_states" }
