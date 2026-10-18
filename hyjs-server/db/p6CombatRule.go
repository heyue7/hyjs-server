package db

// P6CombatRule 默认关闭权威先算后播；完整机制与原客户端对照后才有开放依据。
type P6CombatRule struct {
	ID                   int64  `gorm:"primaryKey;autoIncrement:false" json:"id"`
	AuthoritativeEnabled bool   `json:"authoritativeEnabled"`
	NativeCapabilityHash string `gorm:"size:64" json:"nativeCapabilityHash"`
	ClientHash           string `gorm:"size:64" json:"clientHash"`
	MaxRounds            int32  `json:"maxRounds"`
	TimeoutMillis        int64  `json:"timeoutMillis"`
	Source               string `gorm:"size:255" json:"source"`
}

func (P6CombatRule) TableName() string { return "p6_combat_rules" }
