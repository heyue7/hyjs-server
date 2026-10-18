package constUtil

const (
	P14GuessSource         = "local-original-champion-guess-v1; original405-410-417-419-config; stage7-real-complete-final32; unique-season-joint-target; result-and-payback-pending"
	P14GuessClosed         = "冠军竞猜尚未提前发布或已结束"
	P14GuessCorrupt        = "冠军竞猜与真实候选或冻结来源不符"
	P14GuessChampion int32 = 1
	P14GuessCurrent  int32 = 0
	P14GuessSuccess  int32 = 0
	P14GuessFailed   int32 = 1
	P14GuessAlready  int32 = 2
	P14GuessCoinLack int32 = 3
	P14GuessEnded    int32 = 4
)
