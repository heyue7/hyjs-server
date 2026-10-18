package constUtil

const (
	P14PairGuessSource           = "local-original-pair-guess-v1; original-state7-prefab; real-final32-pair; original10000coin; unique-joint-owner-per-pair; result-and-payback-pending"
	P14PairGuessClosed           = "本场普通竞猜尚未提前发布或已结束"
	P14PairGuessCorrupt          = "普通竞猜与真实对阵或冻结来源不符"
	P14PairGuessSettlement       = "local-pair-payback-v1; certified-selected-winner=successCoin+extraDiamond; other=guaranteedRevertCoin; certificate-required-no-early-award"
	P14PairGuessType       int32 = 2
)
