package db

import "time"

type ExampleModel struct {
	Id         int64     `gorm:"primaryKey;autoIncrement" json:"id"` // ID
	FirstName  string    `gorm:"size:50;not null" json:"first_name"`
	LastName   string    `gorm:"size:50;not null" json:"last_name"`
	CreateTime time.Time `gorm:"column:create_time;autoCreateTime" json:"create_time"` // 创建时间
	UpdateTime time.Time `gorm:"column:update_time;autoUpdateTime" json:"update_time"` // 更新时间
}

func (ExampleModel) TableName() string {
	return "t_example"
}
