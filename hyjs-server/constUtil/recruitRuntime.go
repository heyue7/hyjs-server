package constUtil

// 招募运行规则固定作用于强者招募与王者招募，后台修改不改变原静态奖池快照。
const (
	RecruitRuntimeKindStrong int32 = 2
	RecruitRuntimeKindKing   int32 = 3

	RecruitRuntimeStrongName = "强者招募"
	RecruitRuntimeKingName   = "王者招募"

	// RecruitRuntimeDailyUnlimited 只允许强者招募使用；王者招募每日次数至少为1。
	RecruitRuntimeDailyUnlimited int32 = 0
	RecruitRuntimeDailyMaximum   int32 = 100000
	RecruitRuntimeGuaranteeMin   int32 = 1
	RecruitRuntimeGuaranteeMax   int32 = 10000

	RecruitRuntimeAuditAction = "recruit-settings.update"
	RecruitRuntimeAuditTarget = "recruit_runtime_rules"
)

// RecruitRuntimeName 返回后台固定的中文名称，避免运行规则引入可变名称。
func RecruitRuntimeName(kind int32) string {
	switch kind {
	case RecruitRuntimeKindStrong:
		return RecruitRuntimeStrongName
	case RecruitRuntimeKindKing:
		return RecruitRuntimeKingName
	}
	return ""
}
