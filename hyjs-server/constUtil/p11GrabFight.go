package constUtil

const (
	P11GrabFightAcceptedEvent       = "grab_occupied_accepted"
	P11GrabFightWonEvent            = "grab_occupied_won"
	P11GrabFightSource              = "p11-grab-occupied-original19"
	P11GrabFightPolicySource        = "local-grab-occupied-v1: original SCGrab type19, forest, two original automatic camps and attacker first1/0; current real owner/profile; original10 energy; prepublished local one-attempt/cost charge on either accepted result, private-server loss refund unknown; only verified challenger win transfers ownership; random rewards remain separate"
	P11GrabFightClosed              = "奖杯已占位置的独立19战斗范围尚未开放"
	P11GrabFightConflict            = "奖杯对手、占位来源或独立结算回执已变化"
	P11GrabFightMaxRound      int32 = 40
)
