package model

// P1OperationsSnapshot 输出有限运行记录，不返回会话、密码、完整存档或私人正文。
type P1OperationsSnapshot struct {
	// BinarySHA256 是进程启动时读取的实际可执行文件SHA，后续磁盘覆盖不会改写本进程版本。
	BinarySHA256 string `json:"binarySha256"`
	// StartedAt 为公共能力初始化的Unix毫秒时间。
	StartedAt int64 `json:"startedAt"`
	// Status 为本进程已初始化且数据库只读查询成功的状态。
	Status string `json:"status"`
	// Database 为实际数据库驱动。
	Database string `json:"database"`
	// ServerTime 为当前服务器Unix毫秒时间。
	ServerTime int64 `json:"serverTime"`
	// PlayerID 为筛选角色；0表示所有角色汇总。
	PlayerID int64 `json:"playerId"`
	// InstanceID 为活动实例筛选，空表示全部实例。
	InstanceID string `json:"instanceId"`
	// PendingRewards 为当前仍可领取附件数。
	PendingRewards int64 `json:"pendingRewards"`
	// ClaimedRewards 为已领取附件数。
	ClaimedRewards int64 `json:"claimedRewards"`
	// ExpiredRewards 为过期未领附件数。
	ExpiredRewards int64 `json:"expiredRewards"`
	// UnreadMail 为未读且未过期邮件数。
	UnreadMail int64 `json:"unreadMail"`
	// EventFacts 为已提交业务事实数。
	EventFacts int64 `json:"eventFacts"`
	// SettledPeriods 为已冻结截止快照数。
	SettledPeriods int64 `json:"settledPeriods"`
	// Recent 为最多20项可追溯奖励来源与领取状态。
	Recent []P1RewardRecord `json:"recent"`
}

// P1RewardRecord 是不含正文与凭据的奖励查询结果。
type P1RewardRecord struct {
	// ID 为原领奖协议奖励ID。
	ID int64 `json:"id"`
	// PlayerID 为奖励所属角色。
	PlayerID int64 `json:"playerId"`
	// SourceKey 为业务来源键。
	SourceKey string `json:"sourceKey"`
	// CreatedAt 为创建Unix毫秒时间。
	CreatedAt int64 `json:"createdAt"`
	// ExpiresAt 为领取截止Unix毫秒时间，0表示无期限。
	ExpiresAt int64 `json:"expiresAt"`
	// ClaimedAt 为领取Unix毫秒时间，0表示未领取。
	ClaimedAt int64 `json:"claimedAt"`
}
