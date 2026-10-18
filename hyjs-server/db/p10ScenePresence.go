package db

// P10ScenePresence 保留分线和真实角色原帧；旧会话hash没有在线资格。
type P10ScenePresence struct {
	PlayerID    int64  `gorm:"primaryKey;autoIncrement:false"`
	Scene       string `gorm:"type:varchar(64);not null;index"`
	Line        int32  `gorm:"not null;index"`
	SessionHash string `gorm:"type:char(64);not null"`
	Frame       []byte `gorm:"type:longblob;not null"`
	UpdatedAt   int64  `gorm:"not null;autoUpdateTime:false"`
}

func (P10ScenePresence) TableName() string { return "p10_scene_presence" }
