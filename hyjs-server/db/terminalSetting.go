package db

// TerminalSetting persists the terminal contract without rewriting historical wallet rows.
type TerminalSetting struct {
	ServerID int64  `gorm:"primaryKey;autoIncrement:false" json:"serverID"`
	Payload  string `gorm:"type:longtext;not null" json:"payload"`
}

func (TerminalSetting) TableName() string { return "terminal_settings" }
