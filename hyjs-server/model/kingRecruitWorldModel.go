package model

// KingRecruitWorldsRequest 使用原游戏会话查询可用世界。
type KingRecruitWorldsRequest struct {
	// PlayerID 是原角色ID。
	PlayerID int64 `json:"playerId" binding:"required"`
	// SessionHex 是原16字节游戏会话的32位十六进制值。
	SessionHex string `json:"sessionHex" binding:"required"`
}

// KingRecruitSelectRequest 绑定当前目录版本选择开放世界。
type KingRecruitSelectRequest struct {
	// PlayerID 是原角色ID。
	PlayerID int64 `json:"playerId" binding:"required"`
	// SessionHex 是原16字节游戏会话的32位十六进制值。
	SessionHex string `json:"sessionHex" binding:"required"`
	// PoolID 是worlds接口返回的开放奖池ID。
	PoolID string `json:"poolId" binding:"required"`
	// Version 是worlds接口返回的当前目录摘要。
	Version string `json:"version" binding:"required"`
}

// KingRecruitWorld 是当前角色可用的一个世界。
type KingRecruitWorld struct {
	// PoolID 是当前开放奖池ID。
	PoolID string `json:"poolId"`
	// World 为0=旧轮换，1=SAO，2=ALO，3=GGO。
	World int64 `json:"world"`
	// Label 是玩家可见的世界名。
	Label string `json:"label"`
	// EndAt 是开放截止毫秒，0为旧常驻。
	EndAt int64 `json:"endAt"`
}

// KingRecruitRuntimeRule 是客户端展示的全局招募运行规则。
type KingRecruitRuntimeRule struct {
	// Kind 为2=强者招募，3=王者招募。
	Kind int64 `json:"kind"`
	// DailyLimit 是每日可抽数；强者招募0表示不限。
	DailyLimit int64 `json:"dailyLimit"`
	// GuaranteeEvery 是距下一次保底的抽数间隔。
	GuaranteeEvery int64 `json:"guaranteeEvery"`
}

// KingRecruitWorldsResponse 返回当前开放世界与选择。
type KingRecruitWorldsResponse struct {
	// Worlds 仅包含当前可访问的开放世界。
	Worlds []KingRecruitWorld `json:"worlds"`
	// SelectedPoolID 是当前选中奖池，没有开放世界时为空。
	SelectedPoolID string `json:"selectedPoolId"`
	// Version 是当前世界目录摘要。
	Version string `json:"version"`
	// RecruitRules 固定返回当前有效的强者、王者全局规则，不含账号凭据。
	RecruitRules []KingRecruitRuntimeRule `json:"recruitRules"`
}

// KingRecruitSelectResponse 返回服务端保存后的选择与当前开放世界。
type KingRecruitSelectResponse KingRecruitWorldsResponse
