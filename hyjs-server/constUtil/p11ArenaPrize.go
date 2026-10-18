package constUtil

const (
	P11ArenaPrizePolicyID      int64 = 1
	P11ArenaPrizeSource              = "local-prepublished-arena-challenger-win-three-card-prize-v1; original14victoryitems-and-coin-popularity; unknown-private-pool-amounts-weights-and-caps; first-open-month-freeze; actual-accepted-challenger-win-only; first-card-granted-same-transaction-two-preview-only; local-preview-without-replacement-until-pool-exhausted; zero-coin-cards-local-no-prize-compat; no-periodic-rank-reward"
	P11ArenaPrizeOriginalItems       = "item_001,item_002,item_005,item_006,item_007,item_008,item_009,item_010,item_011,item_012,item_013,item_016,item_024,item_027"
	P11ArenaPrizeDefaultPool         = `[{"id":"item_027","num":1,"weight":1},{"id":"item_024","num":1,"weight":1}]`
	P11ArenaPrizeAwarded             = "awarded"
	P11ArenaPrizeDisabled            = "policy-disabled"
	P11ArenaPrizeClosed              = "window-closed"
	P11ArenaPrizeLevel               = "minimum-level"
	P11ArenaPrizeDaily               = "daily-cap"
	P11ArenaPrizeTotal               = "window-cap"
	P11ArenaPrizeLost                = "challenger-lost"
	P11ArenaPrizeConflict            = "竞技场翻牌奖励来源无效"
	P11ArenaPrizeConfigError         = "竞技场翻牌本地运营配置无效"
)
