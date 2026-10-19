package db

// AdminActivityOccurrence binds one immutable consumer identity to a realm calendar slot.
// Archived rows remain so guessed foreign IDs and historical claims retain their ownership.
type AdminActivityOccurrence struct {
	Timing          string `gorm:"size:24;not null;default:calendar" json:"timing"`
	StartDay        int64  `gorm:"not null;default:1" json:"startDay"`
	DurationDays    int64  `gorm:"not null;default:14" json:"durationDays"`
	ClaimDays       int64  `gorm:"not null;default:0" json:"claimDays"`
	ID              string `gorm:"primaryKey;size:64" json:"id"`
	ServerID        int64  `gorm:"not null;index:idx_activity_realm_dates,priority:1" json:"serverId"`
	TemplateID      string `gorm:"size:64;not null;index" json:"templateId"`
	TemplateVersion string `gorm:"size:64;not null" json:"-"`
	Name            string `gorm:"size:160;not null" json:"name"`
	Mode            string `gorm:"size:64;not null" json:"mode"`
	ConsumerKey     string `gorm:"size:128;not null;uniqueIndex" json:"-"`
	ConsumerID      string `gorm:"size:160;not null;uniqueIndex" json:"consumerId"`
	StartAt         int64  `gorm:"not null;index:idx_activity_realm_dates,priority:2" json:"startAt"`
	EndAt           int64  `gorm:"not null" json:"endAt"`
	ClaimEndAt      int64  `gorm:"not null" json:"claimEndAt"`
	Lane            int64  `gorm:"not null" json:"lane"`
	Enabled         bool   `gorm:"not null" json:"enabled"`
	Archived        bool   `gorm:"not null" json:"archived"`
	FormJSON        []byte `gorm:"type:longblob;not null" json:"-"`
	Revision        int64  `gorm:"not null" json:"-"`
}

func (AdminActivityOccurrence) TableName() string { return "admin_activity_occurrences" }
