package constUtil

const (
	P13UnionFightTowerCancelUnavailable = "target-unavailable-at-impact"
	P13UnionFightTowerCancelOccupancy   = "launch-occupancy-changed"

	P13UnionFightTowerPlayerDamageRule        = "local-tower-physical-v1"
	P13UnionFightTowerPlayerAttack      int64 = 65
	P13UnionFightTowerHitPending              = "pending"
	P13UnionFightTowerHitApplied              = "applied"
	P13UnionFightTowerHitCanceled             = "canceled"
	P13UnionFightTowerHitLegacyApplied        = "legacy-applied"
	P13UnionFightTowerDrainLimit              = 6
	P13UnionFightTowerOccupancyOrdinary       = "ordinary"
	P13UnionFightTowerOccupancyGuard          = "guard"
	P13UnionFightTowerCancelLife              = "life-ordinal-changed"
	P13UnionFightTowerCancelActor             = "bound-actor-dead-or-missing"
	P13UnionFightTowerCancelLeft              = "actor-left-field"
	P13UnionFightTowerCancelCoalesced         = "occupancy-shot-already-frozen"
	P13UnionFightTowerPending                 = "公会战环境命中尚未完成，不能开始下一战"

	P13UnionFightTowerSource                = "original-six-towers-v2;local-tower-physical-v1;fixed-att65-car5;deferred-one-per-tower-occupancy;idle-no-catchup"
	P13UnionFightTowerConflict              = "公会战塔冻结来源/环境伤害账本损坏"
	P13UnionFightTowerBatch                 = 16
	P13UnionFightTowerPlayerLimit           = 128
	P13UnionFightTowerCarLimit              = 512
	P13UnionFightTowerCooldownMillis  int64 = 5000
	P13UnionFightTowerReleaseMillis   int64 = 600
	P13UnionFightTowerAnimationMillis int64 = 1000
	P13UnionFightTowerReviveSeconds         = "5,10,15,20,25,30,35,40"
)
