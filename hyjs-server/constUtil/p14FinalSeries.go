package constUtil

const (
	P14FinalSeriesSource               = "local-original-first-round-series-v1; original-five-games-best-of-five; hourly-offsets-from-frozen-fightstart; per-game-five-minute-lock; independent-KF-certificate-required"
	P14FinalSeriesRules                = "local-final-series-v1; maxGames5; winsRequired3; interval3600000ms; lock300000ms; game1=actual157input; canonicalViewer=ordered-left-joint; laterInput=each-game-separate-real-formation-seal; no-assumed-HP-or-anger-carry; no-certificate-no-advance; no-timeout-forfeit; no-bye"
	P14FinalSeriesPending              = "系列赛仍等待真实阵容源或独立KF终局证书"
	P14FinalSeriesCorrupt              = "系列赛与已冻结首局或赛程来源不符"
	P14FinalSeriesWaitingResult  int32 = 1
	P14FinalSeriesWaitingInput   int32 = 2
	P14FinalSeriesComplete       int32 = 3
	P14FinalSeriesSlotPlanned    int32 = 1
	P14FinalSeriesSlotInputReady int32 = 2
	P14FinalSeriesSlotCertified  int32 = 3
	P14FinalSeriesSlotUnneeded   int32 = 4
	P14FinalSeriesMaxGames       int32 = 5
	P14FinalSeriesWinsRequired   int32 = 3
	P14FinalSeriesIntervalMillis int64 = 3600000
)
