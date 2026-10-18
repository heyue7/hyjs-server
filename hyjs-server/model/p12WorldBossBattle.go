package model

// P12WorldBossFinishResponse 返回服务端实际入账结果和最新完整原战斗回包。
type P12WorldBossFinishResponse struct {
	// StartBase64 是完整SCWorldBossFight的Protobuf标准Base64，包含实际伤害和排行。
	StartBase64 string `json:"startBase64"`
	// AppliedDamage 为本次回执实际入账伤害，重试保持首笔结果。
	AppliedDamage int64 `json:"appliedDamage"`
	// CurrentHP 为本次提交完成后的共享HP快照。
	CurrentHP int64 `json:"currentHp"`
	// TotalHP 为该实例冻结的共享总HP。
	TotalHP int64 `json:"totalHp"`
	// AlreadyCommitted 为true表示本次调用重放已经提交的回执。
	AlreadyCommitted bool `json:"alreadyCommitted"`
}
