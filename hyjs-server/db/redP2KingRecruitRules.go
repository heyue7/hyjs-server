package db

// KingRecruitRules 保存 P2 原协议业务状态；运营未知规则在 Source 明示本地兼容。
type KingRecruitRules struct {
	ID             int64  `gorm:"primaryKey;autoIncrement:false" json:"iD"`
	DailyLimit     int32  `json:"dailyLimit"`
	LuckLimit      int32  `json:"luckLimit"`
	LuckPerDraw    int32  `json:"luckPerDraw"`
	RotationMillis int64  `json:"rotationMillis"`
	EpochMillis    int64  `json:"epochMillis"`
	WheelPool      string `gorm:"size:64" json:"wheelPool"`
	Source         string `gorm:"size:255" json:"source"`
}

func (KingRecruitRules) TableName() string { return "p2_king_rules" }
