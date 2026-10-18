package constUtil

const (
	NoviceGuideSessionInvalid = "登录已失效，请重新登录"
	// NoviceGuideSkipKey 只记录玩家主动跳过初始教学，后续等级功能指引继续使用原表。
	NoviceGuideSkipKey = "client.noviceGuideSkipped"
	// NoviceGuideInitialCompleted 对应原 TableGuide 的 guide_equip_strengthen，初始教学最后一项。
	NoviceGuideInitialCompleted int32 = 12
)
