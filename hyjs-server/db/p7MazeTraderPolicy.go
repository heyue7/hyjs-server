package db

// P7MazeTraderPolicy 是预先发布的本地有限单商品契约；全部价格和概率保留本地来源。
type P7MazeTraderPolicy struct {
	ID            int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled       bool   `gorm:"not null" json:"enabled"`
	Version       string `gorm:"size:64;not null" json:"version"`
	Source        string `gorm:"type:text;not null" json:"source"`
	PublishedAt   int64  `gorm:"not null" json:"publishedAt"`
	StartAt       int64  `gorm:"not null" json:"startAt"`
	EndAt         int64  `gorm:"not null" json:"endAt"`
	MinFloor      int32  `gorm:"not null" json:"minFloor"`
	AppearPPM     int32  `gorm:"not null" json:"appearPPM"`
	ResourceID    string `gorm:"size:64;not null" json:"resourceId"`
	Quantity      int32  `gorm:"not null" json:"quantity"`
	OriginalPrice int32  `gorm:"not null" json:"originalPrice"`
	DiscountPrice int32  `gorm:"not null" json:"discountPrice"`
}

func (P7MazeTraderPolicy) TableName() string { return "p7_maze_trader_policies" }
