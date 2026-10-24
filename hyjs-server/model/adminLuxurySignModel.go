package model

// AdminLuxurySignRulesResponse 是原豪华签到奖励与只读充值门槛。
type AdminLuxurySignRulesResponse struct {
	Version       string                `json:"version"`       // Version 是固定活动及奖励摘要。
	Enabled       bool                  `json:"enabled"`       // Enabled 是运营启停设置。
	Rewards       []AdminResourceAmount `json:"rewards"`       // Rewards 是每日原协议资源。
	RechargeCents int64                 `json:"rechargeCents"` // RechargeCents 是原当日确认支付门槛，单位分。
	StartAt       int64                 `json:"startAt"`       // StartAt 是现有开始毫秒，缺配置为0。
	EndAt         int64                 `json:"endAt"`         // EndAt 是现有结束毫秒，缺配置为0。
}

// AdminLuxurySignRulesSaveRequest 只修改豪华签到启停及每日奖励。
type AdminLuxurySignRulesSaveRequest struct {
	Version string                `json:"version" binding:"required"` // Version 须匹配读取版本。
	Enabled bool                  `json:"enabled"`                    // Enabled 为true时要求非空奖励。
	Rewards []AdminResourceAmount `json:"rewards"`                    // Rewards 是真实支持的奖励。
}
