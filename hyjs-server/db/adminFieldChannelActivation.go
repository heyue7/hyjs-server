package db

// AdminFieldChannelActivation records the one-time original line activation, preserving later operator policy changes.
type AdminFieldChannelActivation struct {
	ServerID int64  `gorm:"primaryKey;autoIncrement:false" json:"serverId"`
	Version  int64  `gorm:"not null" json:"version"`
	Source   string `gorm:"type:text;not null" json:"source"`
}

func (AdminFieldChannelActivation) TableName() string { return "admin_field_channel_activations" }
