package db

type LocalPlayActivityBinding struct {
	PlayerID   int64  `gorm:"primaryKey;autoIncrement:false"`
	ActivityID string `gorm:"primaryKey;size:64"`
	CycleID    string `gorm:"size:64;not null"`
	Revision   int64  `gorm:"not null"`
}

func (LocalPlayActivityBinding) TableName() string { return "local_play_activity_bindings" }

type LocalPlayActivityCycle struct {
	ID            string `gorm:"primaryKey;size:64"`
	PlayerID      int64  `gorm:"not null;index"`
	ActivityID    string `gorm:"size:64;not null"`
	CreatedAt     int64  `gorm:"not null"`
	RealCreatedAt int64  `gorm:"not null"`
	EventAfterID  int64  `gorm:"not null"`
	StartAt       int64  `gorm:"not null"`
	EndAt         int64  `gorm:"not null"`
	RewardEndAt   int64  `gorm:"not null"`
	Snapshot      []byte `gorm:"type:longblob;not null"`
	Source        string `gorm:"size:64;not null"`
}

func (LocalPlayActivityCycle) TableName() string { return "local_play_activity_cycles" }

type LocalPlayActivityOperation struct {
	PlayerID   int64  `gorm:"primaryKey;autoIncrement:false"`
	RequestID  string `gorm:"primaryKey;size:128"`
	Action     string `gorm:"size:16;not null"`
	ActivityID string `gorm:"size:64;not null"`
	Response   []byte `gorm:"type:longblob;not null"`
	CreatedAt  int64  `gorm:"not null"`
}

func (LocalPlayActivityOperation) TableName() string { return "local_play_activity_operations" }

type LocalPlayOpenServerState struct {
	CycleID   string `gorm:"primaryKey;size:64"`
	PlayerID  int64  `gorm:"not null;index"`
	TaskID    int32  `gorm:"primaryKey;autoIncrement:false"`
	Achieved  bool   `gorm:"not null"`
	Progress  int32  `gorm:"not null"`
	Claims    int32  `gorm:"not null"`
	UsedStock int32  `gorm:"not null"`
}

func (LocalPlayOpenServerState) TableName() string { return "local_play_open_server_states" }

type LocalPlayOpenServerCounter struct {
	CycleID    string `gorm:"primaryKey;size:64"`
	PlayerID   int64  `gorm:"not null;index"`
	CounterKey string `gorm:"primaryKey;size:64"`
	Value      int32  `gorm:"not null"`
}

func (LocalPlayOpenServerCounter) TableName() string { return "local_play_open_server_counters" }

// A stamp identifies one original operation across all cycles, including retired cycles.
type LocalPlayOpenServerReceipt struct {
	PlayerID  int64  `gorm:"primaryKey;autoIncrement:false"`
	Timestamp int64  `gorm:"primaryKey;autoIncrement:false"`
	CycleID   string `gorm:"size:64;not null;index"`
	Request   []byte `gorm:"type:longblob;not null"`
	Response  []byte `gorm:"type:longblob;not null"`
}

func (LocalPlayOpenServerReceipt) TableName() string { return "local_play_open_server_receipts" }
