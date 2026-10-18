package constUtil

const (
	// P6OperationTimeEpsilon tolerates only floating accumulation within an original 0.1s frame.
	P6OperationTimeEpsilon    = 1e-8
	P6EngineVersion           = "apk-10056262-p6-v117"
	P6LocalPVENoTargetFinish  = "local-pve-no-target-finish-v1"
	P6OriginalAINoTargetStall = "原敌方自动AI无目标技能未完成，不能生成战斗胜负"
	P6OriginalClientHash      = "ee14d5df4077573abf94be309d6ebdbe7f4ba84c31b09743ed9a9008b65d200f"
	P6BattleCaptured          = "captured"
	P6BattleObserved          = "observed"
	P6BattleComputed          = "computed"
	P6BattleCancelled         = "cancelled"
	P6BattleExpired           = "expired"
	P6ReplayUnavailable       = "战斗录像尚未完成或不属于当前角色"
	P6ReplayConflict          = "战斗实例或录像内容与已冻结记录冲突"
	P6MaxEvents               = 100000
	P6MaxActors               = 15
	P6RoundTimeLimit          = 120.0
)

// P6DiagnosticUF14CopyAI identifies the preserved shape-selected native probe.
// Ordinary UF14 uses the original BattleManager's PlayerAI for both camps.
const P6DiagnosticUF14CopyAI = "diagnostic-uf14-monster-copy-ai-v1"
