package constUtil

const (
	P14FinalFormationProtocolVersion int32 = 3
	P14FinalFormationLockMillis      int64 = 5 * 60 * 1000
	P14FinalFormationSource                = "local-original-final16-formation-v1; original81030-type3; true-final32-common-source; five-minutes-before-original-stage8; no-series-result"
	P14FinalFormationClosed                = "当前决赛阵容未开放或已到原开战前五分钟锁定时刻"
	P14FinalFormationCorrupt               = "决赛阵容与真实轮次资格或冻结来源不符"
)
