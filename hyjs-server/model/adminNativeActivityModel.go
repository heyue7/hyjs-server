package model

// AdminNativeActivityMilestone 是一项按原顺序保存的活动档位。
type AdminNativeActivityMilestone struct {
	// Key 是稳定档位身份；开服基金使用奖励类型与原始Threshold，其他活动使用档位序号。
	Key string `json:"key"`
	// Goal 是玩家需要达到的目标值；开服基金的原等级档不可修改。
	Goal int64 `json:"goal"`
	// Rewards 是本档实际发放资源，不能为空或包含未恢复资源。
	Rewards []AdminResourceAmount `json:"rewards,omitempty"`
}

// AdminNativeVIPTier 是会员福利的一个固定VIP档位配置。
type AdminNativeVIPTier struct {
	// Vip 是固定VIP档位，0至12，不可通过编辑改变身份。
	Vip int64 `json:"vip"`
	// DailyRewards 是该档每日可领取的全部资源。
	DailyRewards []AdminResourceAmount `json:"dailyRewards,omitempty"`
	// WeekCost 是每周领取消耗；沿用原合同，只允许单项钻石。
	WeekCost AdminResourceAmount `json:"weekCost"`
	// WeekRewards 是每周领取获得的全部资源。
	WeekRewards []AdminResourceAmount `json:"weekRewards,omitempty"`
}

// AdminNativeActivityChild 是节日/新版活动目录中的固定子活动及其真实业务表单。
type AdminNativeActivityChild struct {
	// ID 是现存子活动实例ID，不允许新增、删除或改名。
	ID string `json:"id"`
	// Domain 是原消费者域，取period或commerce，不允许跨域。
	Domain string `json:"domain"`
	// Form 是子活动可按原消费者校验编辑的奖励、商品和条件。
	Form AdminActivityForm `json:"form"`
}

// AdminNativeActivityForm 是按真实消费者裁剪的运营表单，不接收原始规则JSON。
type AdminNativeActivityForm struct {
	// StartAt 是活动开始毫秒。
	StartAt int64 `json:"startAt,omitempty"`
	// EndAt 是活动结束毫秒。
	EndAt int64 `json:"endAt,omitempty"`
	// ClaimEndAt 是最后领奖毫秒。
	ClaimEndAt int64 `json:"claimEndAt,omitempty"`
	// MinVip 是开服基金购买所需VIP等级，0至12。
	MinVip int64 `json:"minVip,omitempty"`
	// DiamondCost 是开服基金购买消耗钻石。
	DiamondCost int64 `json:"diamondCost,omitempty"`
	// Threshold 是支付活动门槛；first固定1笔，continued固定3日，single与total为人民币分，consume为钻石。
	Threshold int64 `json:"threshold,omitempty"`
	// Cents 是卡类商品价格，单位人民币分。
	Cents int64 `json:"cents,omitempty"`
	// Charge 是卡类商品计入充值额度。
	Charge int64 `json:"charge,omitempty"`
	// DailyDiamond 是卡类商品每日领取钻石。
	DailyDiamond int64 `json:"dailyDiamond,omitempty"`
	// WeekdayMask 是副本福利星期位掩码，1至127，周日至周六对应第0至6位。
	WeekdayMask int64 `json:"weekdayMask,omitempty"`
	// StartMinute 是日内开始分钟，0至1439。
	StartMinute int64 `json:"startMinute,omitempty"`
	// EndMinute 是日内结束分钟，1至1440。
	EndMinute int64 `json:"endMinute,omitempty"`
	// CurrencyFactor 是副本金币经验倍数，1至原安全上限。
	CurrencyFactor int64 `json:"currencyFactor,omitempty"`
	// DropFactor 是副战利品倍数，1至原安全上限。
	DropFactor int64 `json:"dropFactor,omitempty"`
	// Rewards 是单档奖励或卡类商品附带的固定奖励。
	Rewards []AdminResourceAmount `json:"rewards,omitempty"`
	// Milestones 是开服基金、庆典活动等按档位保存的奖励。
	Milestones []AdminNativeActivityMilestone `json:"milestones,omitempty"`
	// Prizes 是橙卡招募的可编辑奖池，占用原GameplayPoolReward字段。
	Prizes []AdminActivityPrize `json:"prizes,omitempty"`
	// VipTiers 是会员福利固定13档配置。
	VipTiers []AdminNativeVIPTier `json:"vipTiers,omitempty"`
	// Children 是节日/新版活动原有子目录及可编辑表单。
	Children []AdminNativeActivityChild `json:"children,omitempty"`
}

// AdminNativeActivityDTO 是精彩活动真实后台配置项。
type AdminNativeActivityDTO struct {
	// ID 是稳定后台配置ID，修改时须URL编码后放入路径。
	ID string `json:"id"`
	// Kind 是固定活动类型，取fund、payment-first、payment-single、payment-total、payment-continued、payment-consume、month-card、permanent-card、vip-benefits、copy-benefits、recruit-rotation、first-double、celebration、festival、update。
	Kind string `json:"kind"`
	// Name 是服务端固定的正式名称，与客户端同步，不允许后台改写。
	Name string `json:"name"`
	// Alias 仅供后台区分活动，不映射游戏协议。
	Alias string `json:"alias"`
	// Version 是乐观锁摘要，修改时原样提交；发生并发修改后返回409。
	Version string `json:"version"`
	// Enabled 是true对玩家开放，false关闭入口或领取。
	Enabled bool `json:"enabled"`
	// Frozen 是true规则已被原消费者冻结，只能启停或修改别名。
	Frozen bool `json:"frozen"`
	// Note 是当前来源、生效边界及历史保留说明。
	Note string `json:"note"`
	// Form 是按真实消费者裁剪的运营表单。
	Form AdminNativeActivityForm `json:"form"`
}

// AdminNativeActivitySaveRequest 只保存已有真实配置行，不提供新建、删除或跨类型转换。
type AdminNativeActivitySaveRequest struct {
	// RecruitSettings 随活动原子保存当前同类的全局招募次数和保底；未提交时保持原规则。
	RecruitSettings *AdminRecruitSettingsSaveRequest `json:"recruitSettings,omitempty"`
	// Version 是读取时获得的乐观锁摘要。
	Version string `json:"version" binding:"required"`
	// Enabled 是true开放，false关闭；空奖励不会因开启而自动生成。
	Enabled bool `json:"enabled"`
	// Alias 是后台备注名称，最长80个字符，不进入游戏协议。
	Alias string `json:"alias"`
	// Form 是按当前类型编辑的经济字段；冻结行必须与当前规则完全一致。
	Form AdminNativeActivityForm `json:"form"`
}

// AdminNativeActivityRemoveRequest 按当前版本移除停用配置。
type AdminNativeActivityRemoveRequest struct {
	// Version 是列表返回的配置版本。
	Version string `json:"version" binding:"required"`
}
