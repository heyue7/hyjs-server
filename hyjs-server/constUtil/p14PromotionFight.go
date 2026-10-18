package constUtil

const (
	P14PromotionFightSource       = "local-prepublished-promotion-fight-v1; originalpoint1/max40/initialscore1; scopedtyped17; foreign-distinct-playerid; localwin15-loss0-no-powerbonus-min1"
	P14PromotionFightBattleSource = "p14-promotion-scoped"
	P14PromotionFightClosed       = "跨服晋级匹配尚未开放"
	P14PromotionFightPending      = "当前跨服阵容尚未取得原战斗结算资格"
	P14PromotionFightCorrupt      = "跨服晋级匹配来源或点数账目不符"
)

// P14CrossClockContextKey is an internal test clock. Public request fields never
// bind it; ordinary server contexts always retain the wall clock.
type P14CrossClockContextKey struct{}
