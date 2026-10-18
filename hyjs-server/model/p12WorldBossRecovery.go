package model

// P12WorldBossResumeRequest 不接收客户端战斗状态，恢复当前角色唯一未提交战斗。
type P12WorldBossResumeRequest struct{}

// P12WorldBossResumeResponse 返回服务器冻结的原始开战响应。
type P12WorldBossResumeResponse struct {
	// Pending 表示仍有本周期内可恢复的战斗；过期战斗仅解除占用。
	Pending bool `json:"pending"`
	// StartBase64 是原SCWorldBossFight的Protobuf，仅Pending为true时返回。
	StartBase64 string `json:"startBase64,omitempty"`
	// Scene 是该战斗冻结的野外场景，恢复方必须与当前原场景精确匹配。
	Scene string `json:"scene,omitempty"`
}

// P12WorldBossAbandonRequest 标识主动放弃的一场服务器战斗。
type P12WorldBossAbandonRequest struct {
	// FightID 必须属于当前会话角色且尚未提交。
	FightID string `json:"fightId" binding:"required,max=64"`
}

// P12WorldBossAbandonResponse 表示该战斗已解除角色占用。
type P12WorldBossAbandonResponse struct {
	// Abandoned 为true表示本次或此前已放弃，不扣首领HP、不退挑战次数。
	Abandoned bool `json:"abandoned"`
}
