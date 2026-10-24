package constUtil

// Every actual accepted arena victory grants these fixed values in addition to
// the operator flip card. Experience uses the pre-settlement character level and
// the original UserExp level-up thresholds, so it is real progression rather
// than a display-only number.
const (
	P11ArenaVictoryRewardSource         = "user-confirmed-arena-real-victory-popularity50-per-level2exp-v1"
	P11ArenaVictoryPopularity     int64 = 50
	P11ArenaVictoryExpPerLevel    int64 = 2
	P11ArenaVictoryRewardConflict       = "竞技场胜场声望经验事实无效"
)
