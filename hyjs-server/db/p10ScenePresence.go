package db

// P10ScenePresence 保留分线和真实角色原帧；旧会话hash没有在线资格。
type P10ScenePresence struct {
	PlayerID      int64  `gorm:"primaryKey;autoIncrement:false"`
	Scene         string `gorm:"type:varchar(64);not null;index;index:idx_p10_scene_visibility,priority:1"`
	Line          int32  `gorm:"not null;index;index:idx_p10_scene_visibility,priority:2"`
	SessionHash   string `gorm:"type:char(64);not null;index:idx_p10_scene_visibility,priority:3"`
	Frame         []byte `gorm:"type:longblob;not null"`
	UpdatedAt     int64  `gorm:"not null;autoUpdateTime:false"`
	OfflineUntil  int64  `gorm:"not null;default:0;index:idx_p10_scene_visibility,priority:4"`
	OfflinePKUsed bool   `gorm:"not null;default:false"`
}

func (P10ScenePresence) TableName() string { return "p10_scene_presence" }
