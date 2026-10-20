package db

// AdminMailRecipient retains the exact preview audience and committed original mailbox message.
type AdminMailRecipient struct {
	BatchID    string `gorm:"primaryKey;size:36" json:"batchId"`
	PlayerID   int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	PlayerName string `gorm:"size:191;not null" json:"playerName"`
	MailID     int64  `gorm:"not null" json:"mailId"`
}

func (AdminMailRecipient) TableName() string { return "admin_mail_recipients" }
