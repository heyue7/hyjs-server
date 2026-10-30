package constUtil

const (
	// GGOFieldBossStorageLine is the home of pre-line events and in-flight fights.
	GGOFieldBossStorageLine    int64 = 1
	GGOFieldBossPosition             = "ggo:B1"
	GGOFieldBossID                   = "boss_003"
	GGOFieldBossFightOperation       = "boss-fight"
	GGOFieldBossSource               = "ggo-v4"
	GGOFieldBossAwardOperation       = "boss-award"
	GGOFieldBossLateOperation        = "boss-late-settle"
	// A live boss stays available until defeated; its next spawn is one hour later.
	GGOFieldBossRespawnMS   int64 = 60 * 60 * 1000
	GGOFieldBossOpenUntilMS int64 = 253402300799000
)
