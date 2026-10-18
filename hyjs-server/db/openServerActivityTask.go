package db

// OpenServerActivityTask 保留原249个任务的条件与奖励，并保存全服抢购已用库存。
type OpenServerActivityTask struct {
	ID        int32  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	TitleID   int32  `gorm:"column:title_id;not null" json:"title_id"`
	TaskType  int32  `gorm:"not null" json:"task_type"`
	Tap       int32  `gorm:"not null" json:"tap"`
	OpenDay   int32  `gorm:"not null" json:"open_day"`
	Target    int32  `gorm:"not null" json:"target"`
	Visible   bool   `gorm:"not null" json:"visible"`
	NewPrice  int64  `gorm:"not null" json:"new_price"`
	Reward    string `gorm:"type:text;not null" json:"reward"`
	UsedStock int32  `gorm:"not null" json:"used_stock"`
	Source    string `gorm:"type:text;not null" json:"source"`
}

func (OpenServerActivityTask) TableName() string { return "open_server_activity_tasks" }
