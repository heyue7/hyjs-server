package constUtil

const (
	// The original wire field records elimination stage, not a boolean admission.
	P14ParticipationEligible int32 = 0
	P14EliminatedAudition    int32 = 4
	P14EliminatedPromotion   int32 = 6
	P14EliminatedFinal16     int32 = 8
	P14EliminatedFinal8      int32 = 10
	P14EliminatedFinal4      int32 = 12
	P14EliminatedFinalHalf   int32 = 14
	P14EliminatedFinalThird  int32 = 16
	P14EliminatedFinal       int32 = 18
	P14ParticipationPending        = "跨服当前阶段的真实参赛资格尚未确认"
	P14ParticipationCorrupt        = "跨服淘汰阶段缺少一致的真实结果"
)
