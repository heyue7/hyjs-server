package db

// AdminActivityDefinition is a reusable global typed activity. Editing it never rewrites issued occurrences.
type AdminActivityDefinition struct {
	ID                string `gorm:"primaryKey;size:64" json:"id"`
	Name              string `gorm:"size:160;not null" json:"name"`
	Mode              string `gorm:"size:64;not null" json:"mode"`
	FormJSON          []byte `gorm:"type:longblob;not null" json:"-"`
	DefaultDays       int64  `gorm:"not null" json:"defaultDays"`
	ClaimDays         int64  `gorm:"not null" json:"claimDays"`
	Usable            bool   `gorm:"not null" json:"usable"`
	UnavailableReason string `gorm:"type:text;not null" json:"unavailableReason"`
	Enabled           bool   `gorm:"not null" json:"enabled"`
	Archived          bool   `gorm:"not null" json:"archived"`
	SourceActivityID  string `gorm:"size:160;not null;index" json:"-"`
	Revision          int64  `gorm:"not null" json:"-"`
	UpdatedAtMillis   int64  `gorm:"not null" json:"updatedAt"`
}

func (AdminActivityDefinition) TableName() string { return "admin_activity_templates" }
