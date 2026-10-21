package db

// ClientUpdateSession 保存客户端更新的独立运行事实。
type ClientUpdateSession struct {
	PlayerID         int64  `gorm:"column:player_id;primaryKey;autoIncrement:false" json:"playerId"`
	SessionHash      string `gorm:"column:session_hash;size:64" json:"sessionHash"`
	BuildCode        int64  `gorm:"column:build_code;not null" json:"buildCode"`
	StartupID        string `gorm:"column:startup_id;size:64" json:"startupId"`
	LoginAt          int64  `gorm:"column:login_at;not null" json:"loginAt"`
	LastSeenAt       int64  `gorm:"column:last_seen_at;not null" json:"lastSeenAt"`
	DisconnectedAt   int64  `gorm:"column:disconnected_at;not null" json:"disconnectedAt"`
	MaintenanceEpoch int64  `gorm:"column:maintenance_epoch;not null" json:"maintenanceEpoch"`
	Connected        bool   `gorm:"column:connected;not null" json:"connected"`
}

func (ClientUpdateSession) TableName() string { return "client_update_sessions" }
