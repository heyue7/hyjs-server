package model

// AdminResourceAmount 表示原资源目录内的明确数量。
type AdminResourceAmount struct {
	ResourceID string `json:"resourceId"` // ResourceID 是原物品或货币ID。
	Amount     int64  `json:"amount"`     // Amount 是正整数，空草稿可为0。
}

// AdminActivityProduct 是原商品交易规则的业务表单。
type AdminActivityProduct struct {
	ID          string                `json:"id"`          // ID 是稳定商品ID。
	Name        string                `json:"name"`        // Name 是商品标题。
	Index       int64                 `json:"index"`       // Index 是原界面顺序。
	Rewards     []AdminResourceAmount `json:"rewards"`     // Rewards 是实际获得资源。
	Cost        AdminResourceAmount   `json:"cost"`        // Cost 是每份实际消耗。
	DailyLimit  int64                 `json:"dailyLimit"`  // DailyLimit 是单日限购。
	TotalLimit  int64                 `json:"totalLimit"`  // TotalLimit 是整期限购，0不限。
	Stock       int64                 `json:"stock"`       // Stock 是共享库存，-1不限。
	MinLevel    int64                 `json:"minLevel"`    // MinLevel 是购买等级门槛。
	MinVip      int64                 `json:"minVip"`      // MinVip 是原VIP门槛，0不限。
	DiscountBps int64                 `json:"discountBps"` // DiscountBps 是折扣基点。
	Weight      int64                 `json:"weight"`      // Weight 是刷新候选相对权重。
}

// AdminActivityMilestone 是一档累计条件或一天登录奖励。
type AdminActivityMilestone struct {
	Goal    int64                 `json:"goal"`              // Goal 是目标值；按天奖历对应天序。
	Rewards []AdminResourceAmount `json:"rewards"`           // Rewards 是全部获得资源。
	Options []AdminResourceAmount `json:"options,omitempty"` // Options 是最多3个单项选一，与Rewards互斥。
}

// AdminActivityPrize 是转盘明确奖位，不包含动态钻石大奖。
type AdminActivityPrize struct {
	ID         int64  `json:"id"`         // ID 是0..8原奖位。
	ResourceID string `json:"resourceId"` // ResourceID 是原奖品ID。
	Amount     int64  `json:"amount"`     // Amount 是每次命中数量。
	Weight     int64  `json:"weight"`     // Weight 是相对权重。
	Stock      int64  `json:"stock"`      // Stock 是全局库存，0不限。
	Guaranteed bool   `json:"guaranteed"` // Guaranteed 表示是否在保底候选中。
}

// AdminMealWindow 使用原餐厅的日内分钟窗口。
type AdminMealWindow struct {
	// StartMinute 是日内起始分钟，0至1439。
	StartMinute int `json:"startMinute"`
	// EndMinute 是日内结束分钟，1至1440。
	EndMinute int `json:"endMinute"`
	// Amount 是原资源数量。
	Amount int64 `json:"amount"`
}

// AdminCoinPrize 是藏宝箱金币增量与相对权重。
type AdminCoinPrize struct {
	// Coin 是金币增量。
	Coin int64 `json:"coin"`
	// Weight 是相对权重。
	Weight int64 `json:"weight"`
}

// AdminActivityOptions 只包含当前模式原消费者可读的运营参数。
type AdminActivityOptions struct {
	KingWorld      int64                 `json:"kingWorld,omitempty"`      // KingWorld selects SAO=1, ALO=2, GGO=3; 0 preserves legacy rotation.
	DailySpend     int64                 `json:"dailySpend,omitempty"`     // DailySpend 是连续消费每个实际消费日的钻石门槛。
	KingPoolSource string                `json:"kingPoolSource,omitempty"` // KingPoolSource is a server-generated costType3 fixed-price and rotation snapshot; not an editable reward field.
	RankPlaces     int64                 `json:"rankPlaces,omitempty"`     // RankPlaces is the power-race award range, 0 disables ranking, 1..1000 enables equal awards within that range.
	RankRewards    []AdminResourceAmount `json:"rankRewards,omitempty"`    // RankRewards is the actual same-tier power-race deadline award, separate from the goal reward.
	ShopKey        string                `json:"shopKey,omitempty"`        // ShopKey references an actual ordinary shop, not an activity sale.
	ShopSource     string                `json:"shopSource,omitempty"`     // ShopSource is the server-generated immutable native catalogue snapshot.
	// RebateBps 是返利比例基点，1至10000。
	RebateBps int64 `json:"rebateBps,omitempty"`
	// RebateCap 是每日钻石返利上限。
	RebateCap int64 `json:"rebateCap,omitempty"`
	// Meals 是原餐厅两个不重叠时间窗。
	Meals []AdminMealWindow `json:"meals,omitempty"`
	// DayLimit 是藏宝箱每日次数，固定3。
	DayLimit int64 `json:"dayLimit,omitempty"`
	// TotalLimit 是整期累计次数上限。
	TotalLimit int64 `json:"totalLimit,omitempty"`
	// CooldownSeconds 是两次领取间隔秒。
	CooldownSeconds int64 `json:"cooldownSeconds,omitempty"`
	// ExploreRewards 是每次探索明确奖励。
	ExploreRewards []AdminResourceAmount `json:"exploreRewards,omitempty"`
	// FinalRewards 是最终明确奖励。
	FinalRewards []AdminResourceAmount `json:"finalRewards,omitempty"`
	// CoinPrizes 是金币增量及抽取权重。
	CoinPrizes []AdminCoinPrize `json:"coinPrizes,omitempty"`
	// Discounts 是原折扣池的折扣基点。
	Discounts []int32 `json:"discounts,omitempty"`
	// ScorePerDraw 是每抽积分。
	ScorePerDraw int64 `json:"scorePerDraw,omitempty"`
	// TopPrize 是第一名明确单项奖励。
	TopPrize AdminResourceAmount `json:"topPrize"`
	// AdvancePrize 是后续名次明确单项奖励。
	AdvancePrize AdminResourceAmount `json:"advancePrize"`
	// AdvancePlaces 是排名发奖范围。
	AdvancePlaces int64 `json:"advancePlaces,omitempty"`
	// InitialDiamondPool 是初始公共钻石池。
	InitialDiamondPool int64 `json:"initialDiamondPool,omitempty"`
	// JackpotPerMillion 是钻石大奖百万分概率。
	JackpotPerMillion int64 `json:"jackpotPerMillion,omitempty"`
	// PoolContributionBps 是每次抽取钻石池注入基点。
	PoolContributionBps int64 `json:"poolContributionBps,omitempty"`
}

// AdminActivityForm 是按模式编辑的业务字段，不接收原始规则JSON。
type AdminActivityForm struct {
	Alias        string                   `json:"alias,omitempty" binding:"max=80"`                             // Alias 仅供后台区分活动，不映射游戏协议。
	RepeatPolicy string                   `json:"repeatPolicy,omitempty" binding:"omitempty,oneof=once repeat"` // RepeatPolicy freezes per-template reuse: once per role, or independent participation in each occurrence.
	Mode         string                   `json:"mode" binding:"required"`                                      // Mode 必须来自模式目录。
	TemplateID   string                   `json:"templateId" binding:"required"`                                // TemplateID 必须属于当前模式原美术模板。
	Name         string                   `json:"name" binding:"required"`                                      // Name 是活动标题。
	Source       string                   `json:"source"`                                                       // Source 是运营来源说明。
	StartAt      int64                    `json:"startAt"`                                                      // StartAt 是开始毫秒。
	EndAt        int64                    `json:"endAt"`                                                        // EndAt 是结束毫秒。
	ClaimEndAt   int64                    `json:"claimEndAt"`                                                   // ClaimEndAt 是最后领奖毫秒。
	Products     []AdminActivityProduct   `json:"products"`                                                     // Products 是商品模式列表。
	Milestones   []AdminActivityMilestone `json:"milestones"`                                                   // Milestones 是奖历或累计目标。
	Prizes       []AdminActivityPrize     `json:"prizes"`                                                       // Prizes 是转盘九个奖位。
	Options      AdminActivityOptions     `json:"options"`                                                      // Options 是当前模式特定参数。
}

// AdminTypedActivityRequest 创建或保存一份模式表单。
type AdminTypedActivityRequest struct {
	// RecruitSettings 随活动原子保存当前同类的全局招募次数和保底；未提交时保持原规则。
	RecruitSettings   *AdminRecruitSettingsSaveRequest `json:"recruitSettings,omitempty"`
	AdminActivityForm                                  // AdminActivityForm 的字段直接展开为业务表单。
	Version           string                           `json:"version"`     // Version 是编辑时摘要，创建为空。
	Enabled           bool                             `json:"enabled"`     // Enabled 表示是否开放。
	MakeCurrent       bool                             `json:"makeCurrent"` // MakeCurrent 仅转盘可显式设为新全局期。
}

// AdminActivityStateRequest 按版本启停，转盘可显式切换全局源。
type AdminActivityStateRequest struct {
	// Version 是读取时的乐观锁摘要。
	Version string `json:"version" binding:"required"`
	// Enabled 是true开放，false关闭。
	Enabled bool `json:"enabled"`
	// MakeCurrent 是true显式选为当前全局转盘，false保留当前源。
	MakeCurrent bool `json:"makeCurrent"`
}

// AdminActivityDeleteRequest 归档一份活动，历史玩家记录保留。
type AdminActivityDeleteRequest struct {
	// Version 是读取时的乐观锁摘要。
	Version string `json:"version" binding:"required"`
}

// AdminActivityAvailability 区分管理表单的可用性与活动启停状态。
type AdminActivityAvailability struct {
	// Usable 表示当前配置可经此模式运营；false时查看Reason并补齐配置。
	Usable bool `json:"usable"`
	// Reason 是不可用的中文原因，可用时为空。
	Reason string `json:"reason"`
	// Missing 是可操作的缺项说明。
	Missing []string `json:"missing"`
}

// AdminActivityInstance 是业务目录与对应真实消费者状态。
type AdminActivityInstance struct {
	// ID 是当前业务ID。
	ID string `json:"id"`
	// Name 是当前业务显示名称。
	Name string `json:"name"`
	// Mode 是当前模式，取模式目录的mode值。
	Mode string `json:"mode"`
	// TemplateID 是当前模式限定的原美术模板ID。
	TemplateID string `json:"templateId"`
	// Enabled 是true开放，false关闭。
	Enabled bool `json:"enabled"`
	// Version 是读取时的乐观锁摘要。
	Version string `json:"version"`
	// Frozen 是true已经产生冻结规则，false尚未冻结。
	Frozen bool `json:"frozen"`
	// Editable 是true可修改规则，false仅允许启停或复制新期。
	Editable bool `json:"editable"`
	// Form 是按业务模式组织的表单。
	Form AdminActivityForm `json:"form"`
	// Availability 是配置完整性与此运营入口的适配状态，不代表是否已启用。
	Availability AdminActivityAvailability `json:"availability"`
}

// AdminActivityTemplate 将模式绑定原客户端真实资源路径。
type AdminActivityTemplate struct {
	// ID 是当前业务ID。
	ID string `json:"id"`
	// Name 是当前业务显示名称。
	Name string `json:"name"`
	// Prefab 是原客户端真实资源路径。
	Prefab string `json:"prefab"`
	// Preview 是已导出原资源预览URL，缺真实预览时为空。
	Preview string `json:"preview"`
}

// AdminConditionOption 是原VIP等级等实际枚举。
type AdminConditionOption struct {
	// Label 是显示标签。
	Label string `json:"label"`
	// Value 是实际枚举值。
	Value int64 `json:"value"`
}

// AdminActivityCondition 是当前模式可生效的资格字段。
type AdminActivityCondition struct {
	// Key 是实际消费者支持的字段。
	Key string `json:"key"`
	// Label 是显示标签。
	Label string `json:"label"`
	// Type 是number数值或select枚举。
	Type string `json:"type"`
	// Options 是可选原枚举或当前模式运营参数。
	Options []AdminConditionOption `json:"options,omitempty"`
}

// AdminActivityMode 是可创建模式及适配范围，空奖品不会自动投放。
type AdminActivityMode struct {
	// Mode 是当前模式，取模式目录的mode值。
	Mode string `json:"mode"`
	// Label 是显示标签。
	Label string `json:"label"`
	// Templates 是当前模式可选的原美术模板。
	Templates []AdminActivityTemplate `json:"templates"`
	// Conditions 是当前模式真实生效的购买资格。
	Conditions []AdminActivityCondition `json:"conditions"`
	// SupportsProducts 是true使用商品表单，false不显示商品字段。
	SupportsProducts bool `json:"supportsProducts"`
	// SupportsMilestones 是true使用目标/奖历，false不显示目标字段。
	SupportsMilestones bool `json:"supportsMilestones"`
	// SupportsPrizes 是true使用转盘奖位，false不显示奖位。
	SupportsPrizes bool `json:"supportsPrizes"`
	// MaxMilestones 是当前原消费者支持的最大档数。
	MaxMilestones int64 `json:"maxMilestones"`
	// Notes 是模式适配边界。
	Notes string `json:"notes"`
	// DefaultForm 是不自动填奖的空白业务表单。
	DefaultForm AdminActivityForm `json:"defaultForm"`
	// FixedValues 是当前原客户端固定数值，仅展示。
	FixedValues map[string]int64 `json:"fixedValues,omitempty"`
}
