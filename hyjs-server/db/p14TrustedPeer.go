package db

// P14TrustedPeer 是运营明确授权的原服互信与目的地址，不暴露给游戏客户端。
type P14TrustedPeer struct {
	ServerID   int64  `gorm:"primaryKey;autoIncrement:false" json:"serverId"`
	Enabled    bool   `gorm:"not null;default:false" json:"enabled"`
	Endpoint   string `gorm:"type:varchar(512);not null" json:"endpoint"`
	SigningKey []byte `gorm:"type:varbinary(64);not null" json:"-"`
}

func (P14TrustedPeer) TableName() string { return "p14_trusted_peers" }
