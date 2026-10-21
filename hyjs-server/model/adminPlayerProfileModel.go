package model

// AdminPlayerEditOption 是原表约束下可选择的业务值。
type AdminPlayerEditOption struct {
	Value string `json:"value"` // Value 是原业务ID。
	Label string `json:"label"` // Label 是显示名称。
}

// AdminPlayerChapterOption 是原主线章节及其关卡。
type AdminPlayerChapterOption struct {
	Value    string                  `json:"value"`    // Value 是章节ID。
	Label    string                  `json:"label"`    // Label 是章节名称。
	Missions []AdminPlayerEditOption `json:"missions"` // Missions 是本章原关卡。
}

// AdminGamePlayerEditOptionsResponse 为资料表单提供真实约束和目录。
type AdminGamePlayerEditOptionsResponse struct {
	LevelMax      int64                      `json:"levelMax"`      // LevelMax 是原角色等级上限。
	VIPMax        int64                      `json:"vipMax"`        // VIPMax 是原会员等级上限。
	Chapters      []AdminPlayerChapterOption `json:"chapters"`      // Chapters 是原主线章节与关卡。
	Scenes        []AdminPlayerEditOption    `json:"scenes"`        // Scenes 是已恢复的可转入场景。
	BalanceLimits map[string]int64           `json:"balanceLimits"` // BalanceLimits 是各资源可精确输入的上限。
}

// AdminGamePlayerProfileRequest 保存有限的玩家业务资料，统计与身份字段不接受写入。
type AdminGamePlayerProfileRequest struct {
	Version   string           `json:"version" binding:"required"`   // Version 绑定读取时的角色存档版本。
	Name      string           `json:"name" binding:"required"`      // Name 是沿原规则验证的角色名。
	Level     int64            `json:"level" binding:"required"`     // Level 是原等级表支持的角色等级。
	Exp       int64            `json:"exp"`                          // Exp 是本级经验，非负整数。
	VIP       int64            `json:"vip"`                          // VIP 是原会员等级。
	Chapter   string           `json:"chapter" binding:"required"`   // Chapter 是当前主线章节。
	Mission   string           `json:"mission" binding:"required"`   // Mission 是当前主线关卡。
	Scene     string           `json:"scene" binding:"required"`     // Scene 是已恢复的目标场景。
	CreatedAt int64            `json:"createdAt" binding:"required"` // CreatedAt 是角色创建毫秒，不改领取历史。
	Balances  map[string]int64 `json:"balances" binding:"required"`  // Balances 是表单允许编辑的资源余额。
	Reason    string           `json:"reason" binding:"required"`    // Reason 是记录至审计的操作原因。
}
