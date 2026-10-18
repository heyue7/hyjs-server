package model

// GameGuideSkipRequest 使用当前游戏会话持久化角色的初始教学跳过选择。
type GameGuideSkipRequest struct {
	// PlayerID 当前已创建角色的 ID。
	PlayerID int64 `json:"playerId" binding:"required"`
	// SessionHex 当前登录会话的 16 字节十六进制表示。
	SessionHex string `json:"sessionHex" binding:"required"`
}

// GameGuideSkipResponse 返回已提交的教学跳过状态，不包含奖励或功能解锁。
type GameGuideSkipResponse struct {
	// Skipped true=已持久化跳过初始教学；false=未跳过。
	Skipped bool `json:"skipped"`
	// NoviceGuideSchedule 原协议的教学进度，保留已经超过初始教学的后续指引进度。
	NoviceGuideSchedule int32 `json:"noviceGuideSchedule"`
}
