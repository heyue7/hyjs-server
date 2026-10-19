package db

// AdminFieldChannel keeps each realm map's original line capacity separate from resource snapshots.
type AdminFieldChannel struct {
	ServerID   int64  `gorm:"primaryKey;autoIncrement:false" json:"serverId"`
	Scene      string `gorm:"primaryKey;size:64" json:"scene"`
	LineCount  int64  `gorm:"not null" json:"lineCount"`
	MaxPlayers int64  `gorm:"not null" json:"maxPlayers"`
	Revision   int64  `gorm:"not null" json:"revision"`
}

func (AdminFieldChannel) TableName() string { return "admin_field_channels" }
