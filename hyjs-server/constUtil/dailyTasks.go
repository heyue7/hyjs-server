package constUtil

// 日常类型沿用 DailyTaskItem.GetTaskItemInfo，状态沿用原客户端按钮枚举。
const (
	DailyTaskCopy               int32 = 1
	DailyTaskRecruit            int32 = 5
	DailyTaskEquipStrengthen    int32 = 8
	DailyTaskTreasureStrengthen int32 = 9
	DailyTaskTraining           int32 = 10
	DailyTaskCombine            int32 = 11
	DailyTaskEquipRefine        int32 = 12
	DailyTaskClaimable          int32 = 0
	DailyTaskDoing              int32 = 1
	DailyTaskReceived           int32 = 2
	DailyTaskConfigError              = "日常任务SQL配置或存档无效"
	DailyTaskUnavailable              = "日常任务尚未完成或已领取"
	DailyTaskBoxUnavailable           = "日常积分不足或宝箱已领取"
)
