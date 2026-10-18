package db

// StoryQuestReceipt freezes one committed original quest reward for exact HTTP retries.
// The quest progress remains permanent after the bounded response-recovery window expires.
type StoryQuestReceipt struct {
	PlayerID   int64  `gorm:"primaryKey;autoIncrement:false;uniqueIndex:idx_story_quest_claim,priority:1" json:"playerId"`
	Stamp      int64  `gorm:"primaryKey;autoIncrement:false" json:"stamp"`
	QuestID    string `gorm:"type:varchar(64);not null;uniqueIndex:idx_story_quest_claim,priority:2" json:"questId"`
	Request    []byte `gorm:"type:blob;not null" json:"-"`
	Response   []byte `gorm:"type:longblob;not null" json:"-"`
	CreatedAt  int64  `gorm:"not null" json:"createdAt"`
	ExpiresAt  int64  `gorm:"not null" json:"expiresAt"`
	Version    string `gorm:"type:varchar(128);not null" json:"version"`
	Source     string `gorm:"type:varchar(1024);not null" json:"source"`
	SourceHash string `gorm:"type:char(64);not null" json:"sourceHash"`
}

func (StoryQuestReceipt) TableName() string { return "story_quest_receipts" }
