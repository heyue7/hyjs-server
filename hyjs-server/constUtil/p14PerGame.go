package constUtil

const (
	P14PerGameSource                = "local-original-81030-per-game-v1; server-inferred-slot2to5; actual-current-profile; five-minute-lock; complete-origin-seals; no-Game1-reuse-no-result"
	P14PerGameClosed                = "本局阵容尚未提前发布、未到上传窗或已锁定"
	P14PerGameCorrupt               = "本局阵容与真实系列、冻结时窗或签名来源不符"
	P14PerGameImportPath            = "/internal/p14/per-game-formation"
	P14PerGameProtocolVersion int64 = 1
	P14PerGameFormationKind   int32 = 1
	P14PerGameSealKind        int32 = 2
)
