package model

// AdminDailyTask69Item 展示真实任务条件及单条奖励。
type AdminDailyTask69Item struct {
	ID       string              `json:"id"`       // 任务稳定ID。
	Name     string              `json:"name"`     // 任务名称。
	Type     int32               `json:"type"`     // 原任务类型编号。
	Total    int32               `json:"total"`    // 完成目标次数，只读。
	Score    int32               `json:"score"`    // 完成积分，只读。
	MinLevel int32               `json:"minLevel"` // 开放等级，只读。
	Enabled  bool                `json:"enabled"`  // false停用，true启用，只读。
	Reward   AdminResourceAmount `json:"reward"`   // 单条资源奖励。
}

// AdminDailyTask69Response 包含整批内容版本。
type AdminDailyTask69Response struct {
	Boxes   []AdminDailyTaskBox74  `json:"boxes"`   // 原积分宝箱，按等级和所需积分展示。
	Version string                 `json:"version"` // 整批规则内容版本。
	Items   []AdminDailyTask69Item `json:"items"`   // 七项任务或奖励。
}

// AdminDailyTask69Reward 仅允许写入奖励。
type AdminDailyTask69Reward struct {
	ID     string              `json:"id"`     // 任务稳定ID。
	Reward AdminResourceAmount `json:"reward"` // 单条资源奖励。
}

// AdminDailyTask69SaveRequest 原子更新全部七项奖励。
type AdminDailyTask69SaveRequest struct {
	Boxes   []AdminDailyTaskBoxReward74 `json:"boxes,omitempty"`            // 省略时保持旧调用方只修改任务奖励。
	Version string                      `json:"version" binding:"required"` // 整批规则内容版本。
	Reason  string                      `json:"reason" binding:"required"`  // 修改原因。
	Items   []AdminDailyTask69Reward    `json:"items" binding:"required"`   // 七项任务或奖励。
}

// AdminDailyTaskBox74 exposes the existing score chest rules.
type AdminDailyTaskBox74 struct {
	ID      string                `json:"id"`
	Name    string                `json:"name"`
	Score   int32                 `json:"score"`
	Rewards []AdminResourceAmount `json:"rewards"`
}
type AdminDailyTaskBoxReward74 struct {
	ID      string                `json:"id"`
	Rewards []AdminResourceAmount `json:"rewards"`
}
