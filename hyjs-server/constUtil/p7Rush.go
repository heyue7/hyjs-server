package constUtil

const (
	P7RushTrial           int32 = 1
	P7RushElite           int32 = 2
	P7RushTrialBattleType int32 = 9
	P7RushEliteBattleType int32 = 10
	P7RushCaptured              = "captured"
	P7RushComputed              = "computed"
	P7RushSettled               = "settled"
	P7RushReplaySource          = "p7-rush"
	P7RushPolicyID        int64 = 1
	P7RushGateClosed            = "试炼权威结算尚未通过原客户端完整对照，挑战与奖励暂未开放"
	P7RushContentMissing        = "暂未开放：该试炼或精英战斗的原配置尚未恢复"
	P7RushStarEvent             = "rush_star_total"
	P7RushPolicySource          = "原VipPerks numPerk2/numPerk23和numPerks24/numPerks25定义免费/付费日次数；本地未开放策略将每次精英购买/试炼付费重置暂定50钻，原价格未知。原楼层/敌人/条件和P6原生对照未齐，不允许实际扣费和发奖。"
)

// P7RejectedEndLogMaxBytes 限制单次拒绝录像日志大小，保留既有文件轮转上限。
const P7RejectedEndLogMaxBytes = 128 * 1024
