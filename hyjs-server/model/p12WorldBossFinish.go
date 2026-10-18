package model

// P12WorldBossFinishRequest 只标识当前已预计算战斗，客户端不能提交伤害或胜负。
type P12WorldBossFinishRequest struct {
	// FightID 是开战响应中服务器生成的唯一战斗编号。
	FightID string `json:"fightId" binding:"required,max=64"`
}

// P12WorldBossFinishHeader 在本地兼容接口沿用当前游戏会话认证。
type P12WorldBossFinishHeader struct {
	// PlayerID 是当前已登录角色。
	PlayerID int64 `header:"X-Hyjs-Player-ID" binding:"required,gt=0"`
	// SessionKey 是当前16字节原会话的标准Base64表示。
	SessionKey string `header:"X-Hyjs-Session-Key" binding:"required"`
}
