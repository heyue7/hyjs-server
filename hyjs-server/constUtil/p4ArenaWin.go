package constUtil

const (
	P4ArenaWinKind                = "pvp_win"
	P4ArenaWinFactPrefix          = "arena-win:"
	P4ArenaWinPeriodPrefix        = "pvpWinTimes_p4_arena_"
	P4ArenaWinSource              = "local-prepublished-arena-verified-challenger-win-activity-v1; originalType9-pvpWinTimes-BattleRace-WinTimes; unknown-private-goals-awards-window; defaultoff-one-current-month-window-no-grace; original14victory-resource-item027-default; same-tx-accepted-modern23101-only; no-friend-initial-rank-defense-loss-or-retry-facts; open-unsettled-window-reconcile-existing-actual-facts-only; no-closed-history-backfill"
	P4ArenaWinDefaultGoal   int64 = 5
	P4ArenaWinDefaultReward       = "item_027,1"
	P4ArenaWinConflict            = "竞技胜场活动来源不完整"
)
