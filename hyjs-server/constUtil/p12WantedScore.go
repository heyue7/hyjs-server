package constUtil

const (
	// P12WantedScoreRuleV2 freezes a difficulty pool in each newly paid fight.
	P12WantedScoreRuleV2 = "local-wanted-difficulty-hp-share-v2"
	// P12WantedScoreHPScale quantizes remaining HP before computing per-fight loss.
	P12WantedScoreHPScale int64 = 1000000
	P12WantedScorePool1   int64 = 1000
	P12WantedScorePool2   int64 = 5000
	P12WantedScorePool3   int64 = 10000
)
