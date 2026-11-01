package db

// RechargeDirectoryMember permanently maps one combined-page index to its original reward.
type RechargeDirectoryMember struct {
	IndexID    int64  `gorm:"primaryKey;autoIncrement;column:index_id" json:"indexId"`
	Domain     string `gorm:"type:varbinary(8);size:8;not null;uniqueIndex:idx_recharge_directory_binding,priority:1" json:"domain"`
	InstanceID string `gorm:"type:varbinary(512);size:512;not null;uniqueIndex:idx_recharge_directory_binding,priority:2" json:"instanceId"`
	ChildIndex int32  `gorm:"not null;uniqueIndex:idx_recharge_directory_binding,priority:3" json:"childIndex"`
}

func (RechargeDirectoryMember) TableName() string { return "recharge_directory_members" }
