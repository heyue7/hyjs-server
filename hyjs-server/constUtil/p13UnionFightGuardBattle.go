package constUtil

import "time"

const (
	P13UFGuardBattleSource       = "p13_uf_guard_battle"
	P13UFGuardPending            = "pending"
	P13UFGuardSettled            = "settled"
	P13UFGuardCaptured           = "captured"
	P13UFGuardComputed           = "computed"
	P13UFGuardWorkerBatch        = 16
	P13UFGuardWorkerConcurrency  = 2
	P13UFGuardWorkerInterval     = time.Second
	P13UFGuardWorkerWarnInterval = time.Minute
	P13UFGuardRetryMaxSeconds    = 30
	P13UFGuardConflict           = "公会战守卫战冻结来源或状态冲突"
	P13UFGuardProofMissing       = "公会战守卫1/1独立原机能力尚未批准"
	P13UFGuardBuildingPending    = "守卫建筑事件待主代理集成，不能视为场次已结算"
	P13UFGuardLocalRule          = "local-guard-v1:defeat-score0-token0;guard-player-death-ordinal-no-enemy-player-kill;five-second-due"
)
