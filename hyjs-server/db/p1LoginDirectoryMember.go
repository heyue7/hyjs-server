package db

// P1LoginDirectoryMember permanently binds an original Type4 index to one period.
type P1LoginDirectoryMember struct {
	IndexID    int64  `gorm:"primaryKey;autoIncrement;column:index_id" json:"indexId"`
	InstanceID string `gorm:"type:varbinary(64);size:64;not null;uniqueIndex:idx_p1_login_directory_binding" json:"instanceId"`
	ChildIndex int32  `gorm:"not null;uniqueIndex:idx_p1_login_directory_binding" json:"childIndex"`
}

func (P1LoginDirectoryMember) TableName() string { return "p1_login_directory_members" }
