package constUtil

const (
	P11ArenaVirtualSlots int64 = 32767
	P11ArenaNPCIDBase    int64 = 1000000000000
	P11ArenaNPCMaxRank   int64 = 2147483647
	P11ArenaNPCHeroID          = "hero_047"
	P11ArenaNPCHeroGUID        = "arena-training-hero"
	P11ArenaNPCSource          = "local-arena-virtual-slots-v1:32767 implicit empty ranks; original TableHero hero_047 level1 starter formation; first new player32768; legacy ranks retained; outside500 best=max500 floor(rank*0.75), 101..500 10pct,11..100 5pct,top10 single; original9 candidates and authoritative P6/cost/receipt/rewards retained"
)

// User-confirmed rule: one diamond per new all-time rank, capped at start32768.
const P11ArenaRankRewardSource = "user-confirmed-all-ranks-one-diamond-v1"
