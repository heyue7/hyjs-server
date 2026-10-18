package db

// P10ExpeditionHelpAction 一条实际P7风险只接受一人消费，保存来源/关系/日奖励回执，不重复发奖。
type P10ExpeditionHelpAction struct {
	OwnerID       int64  `gorm:"primaryKey;autoIncrement:false" json:"ownerId"`
	MissionSerial string `gorm:"primaryKey;type:varbinary(128)" json:"missionSerial"`
	EventSerial   string `gorm:"primaryKey;type:varbinary(128)" json:"eventSerial"`
	HelperID      int64  `gorm:"not null;index:p10_exp_help_day,priority:1" json:"helperId"`
	Day           string `gorm:"size:10;not null;index:p10_exp_help_day,priority:2" json:"day"`
	StoryID       string `gorm:"size:128;not null" json:"storyId"`
	EventHash     string `gorm:"type:char(64);not null" json:"eventHash"`
	FriendSince   int64  `gorm:"not null" json:"friendSince"`
	Reward        string `gorm:"type:text;not null" json:"reward"`
	DailyHash     string `gorm:"type:char(64);not null" json:"dailyHash"`
	CreatedAt     int64  `gorm:"autoCreateTime:false;not null" json:"createdAt"`
	SourceHash    string `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (P10ExpeditionHelpAction) TableName() string { return "p10_expedition_help_actions" }
