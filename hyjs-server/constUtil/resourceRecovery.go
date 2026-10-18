package constUtil

// 原 APK 的 UIPlayerData.ResetPlayer/SetLast*Time 固定恢复上限与周期。
// TableGameConfigure、TableVipPerks 和 TableUserExp 未定义这四项参数。
const (
	EnergyRecoveryLimit       int32 = 30
	StaminaRecoveryLimit      int32 = 100
	EnergyRecoveryIntervalMS  int64 = 1800 * 1000
	StaminaRecoveryIntervalMS int64 = 360 * 1000
	ErrResourceRecoveryState        = "资源余额或恢复时间无效"
)
