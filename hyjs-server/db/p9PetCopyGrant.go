package db

// P9PetCopyGrant binds original battle/sweep intimacy to one committed settlement.
// Response preserves native PetAddIntimacy protobufs; facts also retain earned
// growth deferred at the unverified random-attribute unlock boundary.
type P9PetCopyGrant struct {
	PlayerID       int64  `gorm:"primaryKey;not null"`
	SourceKey      string `gorm:"primaryKey;size:191;not null"`
	CompletedCount int64  `gorm:"not null;default:1"`
	Response       []byte `gorm:"type:longblob;not null"`
	Facts          []byte `gorm:"type:longblob;not null"`
	CreatedAt      int64  `gorm:"not null"`
	Source         string `gorm:"type:longtext;not null"`
}

func (P9PetCopyGrant) TableName() string { return "p9_pet_copy_grants" }
