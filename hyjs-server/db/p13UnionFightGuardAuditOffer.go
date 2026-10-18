package db

// P13UnionFightGuardAuditOffer 冻结本场原守卫模型/阵营/绑定及具名本地单实体Monster模板。
type P13UnionFightGuardAuditOffer struct {
	MatchID         string  `gorm:"primaryKey;type:varchar(64)" json:"match_id"`
	GuardGUID       int64   `gorm:"primaryKey;autoIncrement:false" json:"guard_guid"`
	GuardID         string  `gorm:"type:varchar(64);not null" json:"guard_id"`
	HeroID          string  `gorm:"type:varchar(64);not null" json:"hero_id"`
	Camp            int64   `gorm:"not null" json:"camp"`
	BindGUID        int64   `gorm:"not null" json:"bind_guid"`
	InitialHP       float64 `gorm:"not null" json:"initial_hp"`
	ArriveRange     float64 `gorm:"not null" json:"arrive_range"`
	Monster         []byte  `gorm:"type:longblob;not null" json:"monster"`
	BattleOfferHash string  `gorm:"type:char(64);not null" json:"battle_offer_hash"`
	MonsterHash     string  `gorm:"type:char(64);not null" json:"monster_hash"`
	Source          string  `gorm:"type:text;not null" json:"source"`
	SourceHash      string  `gorm:"type:char(64);not null" json:"source_hash"`
}

func (P13UnionFightGuardAuditOffer) TableName() string { return "p13_union_fight_guard_audit_offers" }
