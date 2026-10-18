package constUtil

const (
	P11GrabRandomSource      = "grab-occupation-random-local-v1：原82053仅返回SCGetRandReward.add，原按钮依赖红点79；原164表仅有1/2/3h累计奖，没有随机间隔/奖池/数量/权重/日额/过期策略。具名本地预发布版本默认关闭，实际占领分段自首次开放冻结时刻起累计，默认每1800秒一次/北京时间日6次；窗口须同一月且预先发布。奖池默认金币10000、潜能药剂007一份、耐力药剂001一份等权，均为本地运营参数且SQL可配置，不冒称原概率。实际时长在本窗口累积、按北京时间限制每日领取数、窗末未领取到期；每次真实时长资格消耗一个序号，停止/换位/自然3h截止不重计；没有占领或开启前时长不发奖，原占领/1-3h累计奖/对战权威保持。"
	P11GrabRandomRedPoint    = "grabRandomReward"
	P11GrabRandomDefaultPool = `[{"id":"coin","num":10000,"weight":1},{"id":"item_007","num":1,"weight":1},{"id":"item_001","num":1,"weight":1}]`
	P11GrabRandomInvalid     = "奖杯随机时长奖励配置或冻结来源无效"
	P11GrabRandomUnavailable = "当前实际占领时长没有可领取的随机奖励"
)
