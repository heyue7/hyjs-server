package db

// P13UnionFightFieldPolicy 保存具名本地场景策略，运营显式填入后才开放，不授予权威胜负。
type P13UnionFightFieldPolicy struct {
	ID                 int64   `gorm:"primaryKey;autoIncrement:false" json:"id"`
	Enabled            bool    `gorm:"not null;default:false" json:"enabled"`
	PreparationMillis  int64   `gorm:"not null;default:0" json:"preparationMillis"`
	LeftRevivePointID  int64   `gorm:"not null;default:0" json:"leftRevivePointId"`
	RightRevivePointID int64   `gorm:"not null;default:0" json:"rightRevivePointId"`
	InitialGuardHP     float64 `gorm:"not null;default:0" json:"initialGuardHp"`
	InitialGateHP      float64 `gorm:"not null;default:0" json:"initialGateHp"`
	MoveTolerance      float64 `gorm:"not null;default:0" json:"moveTolerance"`
	Source             string  `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightFieldPolicy) TableName() string { return "p13_union_fight_field_policies" }
