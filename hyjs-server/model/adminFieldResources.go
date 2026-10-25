package model

// AdminFieldResourcesResponse 为普通野外首领跨地图共享的每日资源设置。
type AdminFieldResourcesResponse struct {
	// BossDailyRewards 每角色每日可获得普通野外首领奖励的场数，-1不限，0不发奖。
	BossDailyRewards int64 `json:"bossDailyRewards"`
	// GatherRefreshHours 北京时间每日刷新采集点的整点，1至24个唯一0..23整数。
	GatherRefreshHours []int64 `json:"gatherRefreshHours"`
	// Version 配置并发版本。
	Version string `json:"version"`
}

// AdminFieldResourcesSaveRequest 修改后续新开战的野外资源规则。
type AdminFieldResourcesSaveRequest struct {
	AdminFieldResourcesResponse
	// Reason 调整原因，保存审计。
	Reason string `json:"reason" binding:"required"`
}
