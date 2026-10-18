package constUtil

const (
	StoryQuestReceiptVersion          = "local-story-quest-exact-http-receipt-v1"
	StoryQuestReceiptSource           = "原24403 CSQuestReward仅questId，原SCQuestReward.get回调入账并推进本地StoryManager；本地恢复策略：正HTTP时间戳与精确任务绑定，24小时内同请求只恢复已提交原奖励帧，附带玩家/副本返回当前快照；不同时间戳/过期仍拒绝已领任务，装备实例须仍为当前完整状态。"
	StoryQuestReceiptLifetimeMS int64 = 24 * 60 * 60 * 1000
	StoryQuestReceiptExpired          = "主线奖励回执恢复窗口已结束，请重新登录读取当前任务"
	StoryQuestReceiptConflict         = "主线任务请求时间戳冲突"
)
