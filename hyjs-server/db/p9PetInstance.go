package db

// P9PetInstance retains the native Pet protobuf, owned GUID and rule source.
type P9PetInstance struct {
	GUID      string `gorm:"primaryKey;size:64"`
	PlayerID  int64  `gorm:"not null;index"`
	PetID     string `gorm:"size:64;not null"`
	Data      []byte `gorm:"type:longblob;not null"`
	CreatedAt int64  `gorm:"not null"`
	Source    string `gorm:"type:longtext;not null"`
}

func (P9PetInstance) TableName() string { return "p9_pet_instances" }
