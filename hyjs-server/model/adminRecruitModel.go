package model

// AdminRecruitSetting 是后台展示的一种招募运行规则。
type AdminRecruitSetting struct {
	// Kind 为2=强者招募，3=王者招募。
	Kind int64 `json:"kind"`
	// Name 是固定的玩家可见招募名称。
	Name string `json:"name"`
	// DailyLimit 是每日可抽数；强者招募0表示不限。
	DailyLimit int64 `json:"dailyLimit"`
	// GuaranteeEvery 是距下一次保底的抽数间隔。
	GuaranteeEvery int64 `json:"guaranteeEvery"`
}

// AdminRecruitSettingsResponse 返回当前全局招募运行规则及乐观版本。
type AdminRecruitSettingsResponse struct {
	// Version 是当前两项规则的摘要，用于保存时检测并发修改。
	Version string `json:"version"`
	// Items 固定包含强者招募与王者招募两项。
	Items []AdminRecruitSetting `json:"items"`
}

// AdminRecruitSettingsSaveRequest 保存完整的两项全局招募运行规则。
type AdminRecruitSettingsSaveRequest struct {
	// Version 是读取时返回的当前版本。
	Version string `json:"version"`
	// Reason 是本次运营调整原因，写入后台审计。
	Reason string `json:"reason"`
	// Items 必须完整包含2、3两个不重复的招募类型。
	Items []AdminRecruitSetting `json:"items"`
}
