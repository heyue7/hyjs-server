package db

// P8LifeState 保存野外回血冷却、安全区恢复时钟、死亡计时及原地复活保护时刻。
type P8LifeState struct {
	PlayerID        int64  `gorm:"primaryKey;autoIncrement:false" json:"playerId"`
	FoodReadyAt     int64  `gorm:"not null" json:"foodReadyAt"`
	SafeRecoveryAt  int64  `gorm:"not null" json:"safeRecoveryAt"`
	InSafe          bool   `gorm:"not null" json:"inSafe"`
	DeathAt         int64  `gorm:"not null" json:"deathAt"`
	InvincibleUntil int64  `gorm:"not null" json:"invincibleUntil"`
	LastFightID     string `gorm:"size:64;not null" json:"lastFightId"`
	Shortcut1       string `gorm:"size:64;not null;default:''" json:"shortcut1"`
	Shortcut2       string `gorm:"size:64;not null;default:''" json:"shortcut2"`
	Source          string `gorm:"type:longtext;not null" json:"-"`
}

func (P8LifeState) TableName() string { return "p8_life_states" }
