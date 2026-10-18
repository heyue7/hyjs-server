package db

// AdminActivityPattern stores reusable relative-day plans, without dates or a realm binding.
type AdminActivityPattern struct {
	ID              string `gorm:"primaryKey;size:64" json:"id"`
	Name            string `gorm:"size:160;not null" json:"name"`
	CycleDays       int64  `gorm:"not null" json:"cycleDays"`
	EntriesJSON     []byte `gorm:"type:longblob;not null" json:"-"`
	Revision        int64  `gorm:"not null" json:"-"`
	UpdatedAtMillis int64  `gorm:"not null" json:"updatedAt"`
	Archived        bool   `gorm:"not null" json:"archived"`
}

func (AdminActivityPattern) TableName() string { return "admin_activity_patterns" }
