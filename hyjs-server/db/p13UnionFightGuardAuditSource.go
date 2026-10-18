package db

// P13UnionFightGuardAuditSource 提前发布具名守卫Monster模板；原等级/属性/血条/战斗站位未知，默认无来源。
type P13UnionFightGuardAuditSource struct {
	Day         string `gorm:"primaryKey;type:char(10)" json:"day"`
	GuardID     string `gorm:"primaryKey;type:varchar(64)" json:"guard_id"`
	Enabled     bool   `gorm:"not null;default:false" json:"enabled"`
	Monster     []byte `gorm:"type:longblob;not null" json:"monster"`
	PublishedAt int64  `gorm:"not null" json:"published_at"`
	Source      string `gorm:"type:text;not null" json:"source"`
}

func (P13UnionFightGuardAuditSource) TableName() string { return "p13_union_fight_guard_audit_sources" }
