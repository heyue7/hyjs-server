package db

// AdminGameServer stores operator metadata separately from runtime process identity.
type AdminGameServer struct {
	ID           int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Name         string `gorm:"size:80;not null" json:"name"`
	Enabled      bool   `gorm:"not null" json:"enabled"`
	OpenAt       int64  `gorm:"not null" json:"openAt"`
	RuntimeBound bool   `gorm:"not null" json:"runtimeBound"`
	Revision     int64  `gorm:"not null" json:"-"`
}

func (AdminGameServer) TableName() string { return "admin_game_servers" }
