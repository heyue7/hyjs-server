package model

// InstanceLockStateRequest 查询当前登录角色的实例保护状态。
type InstanceLockStateRequest struct {
	PlayerID   int64  `json:"playerId" binding:"required"`   // 当前角色ID
	SessionHex string `json:"sessionHex" binding:"required"` // 原游戏会话的十六进制值
}

// InstanceLockSetRequest 设置拥有的角色或装备实例保护状态。
type InstanceLockSetRequest struct {
	PlayerID   int64  `json:"playerId" binding:"required"`   // 当前角色ID
	SessionHex string `json:"sessionHex" binding:"required"` // 原游戏会话的十六进制值
	GUID       string `json:"guid" binding:"required"`       // 当前拥有实例的GUID
	Locked     *bool  `json:"locked" binding:"required"`     // true锁定，false解锁
	Revision   int64  `json:"revision"`                      // 最近读取的锁定状态版本
}

// InstanceLockResponse 返回已提交的保护集合和版本。
type InstanceLockResponse struct {
	LockedGUIDs []string `json:"lockedGuids"` // 当前角色锁定的实例GUID集合
	Revision    int64    `json:"revision"`    // 锁定状态版本
}
