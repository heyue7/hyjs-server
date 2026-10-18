package db

// AdminActivityScheduleRevision serializes whole-calendar saves for a realm.
type AdminActivityScheduleRevision struct {
	ServerID int64 `gorm:"primaryKey;autoIncrement:false" json:"serverId"`
	Revision int64 `gorm:"not null" json:"revision"`
}

func (AdminActivityScheduleRevision) TableName() string { return "admin_activity_schedule_revisions" }
