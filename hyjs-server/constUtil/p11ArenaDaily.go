package constUtil

import "time"

const (
	P11ArenaDailyPolicyID        int64 = 1
	P11ArenaDailySource                = "local-prepublished-arena-daily-rank-reward-v1; original7TablePvpRankReward-preserved; unknown-private-cutoff-participation-policy; defaultoff-local-cutoff-second-and-min-actual-challenger-count; first-current-window-freeze-no-historical-offers; actual-accepted-modern23101-only; before-rank-mutation-cutoff-snapshot; missing-cutoff-evidence-pending; initial-rank-only-no-reward; originalDeleteRewardDays14; P1-once-offline-delivery"
	P11ArenaDailyOriginalRewards       = "1|1|popularity,5000;coin,120000;item_007,50\n2|2-5|popularity,4000;coin,100000;item_007,40\n3|6-20|popularity,3500;coin,80000;item_007,35\n4|21-50|popularity,3000;coin,50000;item_007,30\n5|51-100|popularity,2500;coin,40000;item_007,25\n6|101-500|popularity,2000;coin,20000;item_007,15\n7|501-1000|popularity,1000;coin,10000;item_007,10"
	P11ArenaDailyComplete              = "complete"
	P11ArenaDailyPending               = "pending"
	P11ArenaDailyConflict              = "竞技每日榜奖来源无效"
	P11ArenaDailyConfigError           = "竞技每日榜奖本地发布配置无效"
	P11ArenaDailyWindowLimit     int64 = 32
	P11ArenaDailyWorkerInterval        = time.Minute
)

// P11ArenaDailyClockContextKey permits bounded isolated cutoff tests; normal
// production contexts supply no clock override and retain actual wall time.
type P11ArenaDailyClockContextKey struct{}
