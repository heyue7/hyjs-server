package constUtil

const (
	// GGOBattleResumeMonster is the original field monster battle type.
	GGOBattleResumeMonster int64 = 6
	// GGOBattleResumeBoss is the original field boss battle type.
	GGOBattleResumeBoss int64 = 12
	// GGOBattleResumeActive returns the already accepted original start.
	GGOBattleResumeActive = "active"
	// GGOBattleResumeSettled returns an immutable original end receipt.
	GGOBattleResumeSettled = "settled"
	// GGOBattleResumeConflict deliberately reveals no other fight identity.
	GGOBattleResumeConflict = "原战斗身份或状态无法确认"
)
