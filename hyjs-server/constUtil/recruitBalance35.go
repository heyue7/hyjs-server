package constUtil

// 招募调整只在版本首次应用时改运营配置，后续后台修改和已累计抽数始终保留。
const (
	RecruitBalance35Source               = "本服招募：红卡限定所属世界；橙紫蓝绿跨世界共享。基础概率按角色权重，最后20%抽数线性提高，保底间隔由后台配置。王者保底为橙卡或红卡，橙卡招募保底为橙卡。幸运兑换单列，不计入普通抽卡红卡目标。"
	RecruitBalance35Version              = "mixed-soft-pity-v1"
	RecruitBalance35BeforeAction         = "recruit.balance.before"
	RecruitBalance35ApplyAction          = "recruit.balance.apply"
	RecruitBalance35RedPPM         int64 = 1500
	RecruitBalance35DailyLimit     int32 = 70
	RecruitBalance35GuaranteeEvery int32 = 20
)
