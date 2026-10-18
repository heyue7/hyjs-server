package db

// KingRecruitState 保存 P2 原协议业务状态；运营未知规则在 Source 明示本地兼容。
type KingRecruitState struct {
	PlayerID int64  `gorm:"primaryKey;autoIncrement:false" json:"playerID"`
	Day      string `gorm:"size:10" json:"day"`
	Draws    int32  `json:"draws"`
	Luck     int32  `json:"luck"`
}

func (KingRecruitState) TableName() string { return "p2_king_states" }
