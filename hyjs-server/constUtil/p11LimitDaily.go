package constUtil

import "time"

const (
	P11LimitDailySource         = "local-limit-daily-score-v1: original seven TableLimitPvpDailyScore amounts; private cutoff/participation calendar unknown; prepublished/defaultoff; current period real accepted mode20 challengers only; original scoreDESC/playerID tie order"
	P11LimitDailyOriginalScores = "1|1|1|50\n2|2|3|40\n3|4|10|30\n4|11|50|25\n5|51|100|20\n6|101|500|15\n7|501|9999|10"
	P11LimitDailyConflict       = "限时竞技每日榜积分冻结或来源不一致"
	P11LimitDailyConfigError    = "限时竞技每日榜预发布配置未完整准备"
	P11LimitDailyComplete       = "complete"
	P11LimitDailyPending        = "pending"
	P11LimitDailyWorkerInterval = 5 * time.Second
)

type P11LimitDailyClockContextKey struct{}
