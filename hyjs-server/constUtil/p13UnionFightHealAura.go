package constUtil

const P13UnionFightHealAuraSource = "local-guild-recovery-matrix-v1: original013 self-emitter buff009 five seconds/team heal; explicit prepublished radius/tick/HP-percent/authoritymask; follows caster current position; catchup uses current eligible targets; cancel on exit/death/Hook/feature shutdown; living poses only; no battle damage or resurrection"

const (
	P13UFHealAuraPendingRule  = "local-heal-aura-pending-v1"
	P13UFHealAuraPending      = "pending"
	P13UFHealAuraApplied      = "applied"
	P13UFHealAuraCanceled     = "canceled"
	P13UFHealAuraBackpressure = "战场效果正在结算，请稍后再施放"
)
