package constUtil

const (
	P11ArenaGuideCompleted = int32(13)
	P11ArenaGuideLevel     = int32(10)
	P11ArenaGuideDenied    = "竞技引导等级、角色或完成进度不满足"
	P11ArenaGuideShape     = "竞技引导需要完整原先手阵容与40回合独立范围"
	P11ArenaGuidePeers     = "竞技引导尚缺两个实际可挑战对手"
	P11ArenaGuideSource    = "original23105 CSPvpFightResultGuide; actual query-frozen local roster; original Guide typed2 forced attacker-first at explicit Inst1/0 intersection"
	P11ArenaModernSource   = "original23101, actual query-frozen local roster; rank-only prepublished policy"
)

type P11ArenaGuideContextKey struct{}
