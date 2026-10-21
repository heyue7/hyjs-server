package db

// RecruitRuntimeRule 保存招募的全局运行覆盖，不修改原奖池、价格、保底范围或历史进度。
type RecruitRuntimeRule struct {
	// Kind 为2=强者招募，3=王者招募。
	Kind int32 `gorm:"primaryKey;autoIncrement:false" json:"kind"`
	// DailyLimit 为每日抽数上限；强者招募0表示不限，王者招募至少为1。
	DailyLimit int32 `gorm:"not null" json:"dailyLimit"`
	// GuaranteeEvery 为本次保底间隔，1至10000。
	GuaranteeEvery int32 `gorm:"not null" json:"guaranteeEvery"`
}

func (RecruitRuntimeRule) TableName() string { return "recruit_runtime_rules" }
