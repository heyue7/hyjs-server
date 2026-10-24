package model

// AdminTrialAnchor 保存指数插值的楼层属性锚点。
type AdminTrialAnchor struct {
	// Floor 楼层，0仅用于预览全部楼层。
	Floor int32 `json:"floor"`
	// HP 生命属性。
	HP float64 `json:"hp"`
	// Att 攻击属性。
	Att float64 `json:"att"`
	// Def 物防与法防属性。
	Def float64 `json:"def"`
}

// AdminTrialGeneration 管理既有102层算法参数，奖励和局内加成保持Re规则。
type AdminTrialGeneration struct {
	// Floors 总层数，本版固定102。
	Floors int32 `json:"floors"`
	// Anchors 按楼层递增的属性曲线锚点。
	Anchors []AdminTrialAnchor `json:"anchors"`
	// DifficultyFactors 一至三星属性倍率。
	DifficultyFactors []float64 `json:"difficultyFactors"`
	// Roster 按加入顺序排列的原角色资源ID。
	Roster []string `json:"roster"`
	// Positions 与阵容顺序对应的站位，1至15且不重复。
	Positions []int32 `json:"positions"`
	// InitialCount 第一层敌人数。
	InitialCount int32 `json:"initialCount"`
	// CountEveryFloors 每隔指定层数增加一名敌人。
	CountEveryFloors int32 `json:"countEveryFloors"`
	// MaxCount 敌人数上限。
	MaxCount int32 `json:"maxCount"`
}

// AdminTrialConfigResponse 返回当前来源、实际发布目录及可编辑生成参数。
type AdminTrialConfigResponse struct {
	// Enabled false暂停新开战，true启用Re普通试炼。
	Enabled bool `json:"enabled"`
	// Source 当前生效目录来源。
	Source string `json:"source"`
	// Version 乐观并发版本。
	Version int64 `json:"version"`
	// PublishedAt 最后发布时间，毫秒时间戳。
	PublishedAt int64 `json:"publishedAt"`
	// PublishedFloorCount 实际启用普通试炼楼层数。
	PublishedFloorCount int64 `json:"publishedFloorCount"`
	// Config 完整生成参数。
	Config AdminTrialGeneration `json:"config"`
}

// AdminTrialPreviewRequest 在保存前预览全部三难度，floor为0时返回全部102层。
type AdminTrialPreviewRequest struct {
	// Config 完整生成参数。
	Config AdminTrialGeneration `json:"config" binding:"required"`
	// Floor 楼层，0仅用于预览全部楼层。
	Floor int32 `json:"floor"`
}

// AdminTrialSaveRequest 按版本原子发布；只影响下一场战斗。
type AdminTrialSaveRequest struct {
	// ExpectedVersion 读取时获得的版本。
	ExpectedVersion int64 `json:"expectedVersion"`
	// Enabled false暂停新开战，true启用Re普通试炼。
	Enabled bool `json:"enabled"`
	// Config 完整生成参数。
	Config AdminTrialGeneration `json:"config" binding:"required"`
	// Reason 本次修改原因。
	Reason string `json:"reason" binding:"required"`
}

// AdminTrialEnemy 描述实际生成的敌人及原资源名称。
type AdminTrialEnemy struct {
	// ID 原角色资源ID。
	ID string `json:"id"`
	// Name 原角色名称。
	Name string `json:"name"`
	// Pose 阵容站位。
	Pose int32 `json:"pose"`
	// HP 生命属性。
	HP float64 `json:"hp"`
	// Att 攻击属性。
	Att float64 `json:"att"`
	// Def 物防与法防属性。
	Def float64 `json:"def"`
}

// AdminTrialPreviewItem 为一个楼层难度的真实生成预览。
type AdminTrialPreviewItem struct {
	// Floor 楼层，0仅用于预览全部楼层。
	Floor int32 `json:"floor"`
	// Star 难度，1至3星。
	Star int32 `json:"star"`
	// RecommendedPower 由实际生成敌人属性计算的推荐战力。
	RecommendedPower float64 `json:"recommendedPower"`
	// Enemies 实际生成的完整敌方阵容。
	Enemies []AdminTrialEnemy `json:"enemies"`
}

// AdminTrialPreviewResponse 返回生成结果，不修改运行目录。
type AdminTrialPreviewResponse struct {
	// Items 楼层难度预览列表。
	Items []AdminTrialPreviewItem `json:"items"`
}
