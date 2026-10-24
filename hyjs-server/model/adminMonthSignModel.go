package model

// AdminMonthSignDay 是原每日签到的一档固定奖励。
type AdminMonthSignDay struct {
	Day       int64                 `json:"day"`       // Day 是1至31的签到序号。
	Reward    []AdminResourceAmount `json:"reward"`    // Reward 是原界面支持的一项资源。
	DoubleVIP int64                 `json:"doubleVip"` // DoubleVIP 是双倍所需VIP，0不启用双倍，1至12为原VIP等级。
}

// AdminMonthSignRulesResponse 是所选自然月签到奖历及并发编辑版本。
type AdminMonthSignRulesResponse struct {
	Month       string `json:"month"`       // Month 是YYYY-MM自然月。
	Configured  bool   `json:"configured"`  // Configured 表示该月已独立配置。
	DaysInMonth int64  `json:"daysInMonth"` // DaysInMonth 是该月实际天数。

	Version string              `json:"version"` // Version 是当前31档配置摘要。
	Enabled bool                `json:"enabled"` // Enabled 为所选月签到启停。
	Source  string              `json:"source"`  // Source 标明当前配置来源。
	Rows    []AdminMonthSignDay `json:"rows"`    // Rows 始终包含31档。
}

// AdminMonthSignRulesSaveRequest 保存完整奖历，不修改角色领取历史。
type AdminMonthSignRulesSaveRequest struct {
	Month string `json:"month"` // Month 缺省为北京时间当前月，过去月份只读。

	Version string              `json:"version" binding:"required"` // Version 须与最近读取相同。
	Enabled bool                `json:"enabled"`                    // Enabled 为所选月启停。
	Rows    []AdminMonthSignDay `json:"rows" binding:"required"`    // Rows 必须完整包含1至31档。
}
