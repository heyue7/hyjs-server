package constUtil

// 转盘递增概率与保底错误文案供同事务抽取路径共用。
const (
	TurntablePityScheduleInvalid      = "转盘保底进度无法兑现，请检查活动规则与存档"
	TurntablePityRulesInvalid         = "转盘奖位递增规则无效"
	TurntablePityFallbackInsufficient = "转盘未开启递增的普通奖位库存不足"
	TurntablePityProgressInvalid      = "转盘保底进度无效"
	TurntablePityStateInvalid         = "转盘保底存档无效"
)
