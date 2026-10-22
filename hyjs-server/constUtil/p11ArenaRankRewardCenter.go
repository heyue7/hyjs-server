package constUtil

// The user-confirmed best-rank diamond now enters the original reward centre
// instead of the settlement response balance. Frozen arenaRankReward inputs keep
// P11ArenaRankRewardSource so accepted P6 sources are never rewritten; awards
// settled under the split carry the explicit centre marker below.
const (
	P11ArenaRankRewardCenterSource     = "user-confirmed-all-ranks-one-diamond-reward-center-v2"
	P11ArenaRankRewardCenterSourceKey  = "arena-best:"
	P11ArenaRankRewardCenterTitle      = "竞技场新纪录"
	P11ArenaRankRewardCenterBodyFormat = "历史最高排名提升至第%d名。"
)
