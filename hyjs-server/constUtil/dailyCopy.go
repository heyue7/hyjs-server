package constUtil

const (
	DailyCopyPolicyID            = "local_v1"
	DailyCopyConfigError         = "资源副本配置或战斗存档无效"
	DailyCopyUnavailable         = "资源副本未开放、等级不足或今日次数已用完"
	DailyCopyStaminaError        = "体力不足，无法挑战资源副本"
	DailyCopyBattleError         = "资源副本战斗会话或战斗记录无效"
	DailyCopyReplayError         = "资源副本请求已过期或重试内容不一致"
	DailyCopyActiveError         = "请先完成当前资源副本战斗"
	DailyCopyDefaultRoom         = "map_forest_002"
	DailyCopyDefaultRounds int32 = 40
	DailyCopyDefaultStars  int32 = 5
	DailyCopyPolicySource        = "本地资源副本策略：UTC+8按原表星期开放；开始挑战扣除原表体力及一次日限，失败不返还；同日单场未结算战斗可恢复，跨日新场独立计次；胜利5星；Drop1每资源按原表数量权重独立抽取，否则使用Drop；开始冻结战斗和奖励；客户端模拟，仅校验会话、回合和剩余HP边界"
)
