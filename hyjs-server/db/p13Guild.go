package db

// P13Guild 保存公会共享资产，所有成员管理在门禁行锁下同事务执行。
type P13Guild struct {
	ID          string `gorm:"primaryKey;type:varchar(64);index:idx_p13_guild_order,priority:2" json:"id"`
	Name        string `gorm:"type:varchar(64);not null;uniqueIndex" json:"name"`
	Level       int32  `gorm:"not null" json:"level"`
	Notice      string `gorm:"type:text;not null" json:"notice"`
	Declaration string `gorm:"type:text;not null" json:"declaration"`
	Flag        int32  `gorm:"not null" json:"flag"`
	Coin        int64  `gorm:"not null" json:"coin"`
	Badge       int32  `gorm:"not null" json:"badge"`
	Exp         int32  `gorm:"not null" json:"exp"`
	DonateScore int32  `gorm:"not null" json:"donateScore"`
	ActiveScore int32  `gorm:"not null" json:"activeScore"`
	// 建设开关仅由原建设事务授予，升级科技独立核实后保存。
	HeroStatueOpen int32 `gorm:"not null;default:0" json:"heroStatueOpen"`
	GuardTreeOpen  int32 `gorm:"not null;default:0" json:"guardTreeOpen"`
	// UnionFightAutoApply 保存原自动宣战选择；NULL 只作为旧库尚未回填的迁移哨兵，正式读取按 false 处理。
	UnionFightAutoApply *bool `gorm:"index" json:"unionFightAutoApply"`
	CreatedAt           int64 `gorm:"not null;index:idx_p13_guild_order,priority:1" json:"createdAt"`
}

func (P13Guild) TableName() string { return "p13_guilds" }
