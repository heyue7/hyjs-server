package model

// P7ElitePageRequest 只读取当前可见窗口，或一个已选择的精英首领。
type P7ElitePageRequest struct {
	// PlayerID 当前角色编号。
	PlayerID int64 `json:"playerId" binding:"required"`
	// SessionHex 当前原游戏会话，16字节的小写十六进制。
	SessionHex string `json:"sessionHex" binding:"required"`
	// Offset 首屏为0；与bossId互斥。
	Offset *int64 `json:"offset,omitempty"`
	// Limit 当前窗口条数，取值1至5。
	Limit int64 `json:"limit"`
	// BossID 只复核一个已选择的首领；与offset互斥。
	BossID string `json:"bossId,omitempty"`
	// SelectedBossID 保持已有选择；滑动分页不准备其他首领的战斗数据。
	SelectedBossID string `json:"selectedBossId,omitempty"`
}

// P7ElitePageResponse 返回有限条原协议数据及明确的待查看状态。
type P7ElitePageResponse struct {
	// Packet 原SCGetRushEliteBossInfo的Protobuf Base64，最多5条。
	Packet string `json:"packet"`
	// Offset 本次窗口的起始位置；单项复核为-1。
	Offset int64 `json:"offset"`
	// NextOffset 下一窗口起点；-1表示没有后续。
	NextOffset int64 `json:"nextOffset"`
	// Total 当前精英目录总条数。
	Total int64 `json:"total"`
	// SelectedBossID 当前选中首领；不在本窗口时保留此前选择。
	SelectedBossID string `json:"selectedBossId"`
	// PendingIDs 仅有摘要、尚未完整检查的首领，不能当作已开放。
	PendingIDs []string `json:"pendingIds"`
}
