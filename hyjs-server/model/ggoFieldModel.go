package model

// AdminFieldGGOConfiguration 保存沙漠搜打撤的设计配置，供后台准备。
type AdminFieldGGOConfiguration struct {
	SchemaVersion int64                   `json:"schemaVersion"` // SchemaVersion 为配置结构版本，当前固定为2。
	Title         string                  `json:"title"`
	ModeName      string                  `json:"modeName"`
	CurrencyID    string                  `json:"currencyId"`
	ShopKey       string                  `json:"shopKey"`
	CarryIcon     string                  `json:"carryIcon"`
	SproutIcon    string                  `json:"sproutIcon"`
	NPCs          []AdminFieldGGONPC      `json:"npcs"`
	Monsters      []AdminFieldGGOEnemy    `json:"monsters"`
	Bosses        []AdminFieldGGOEnemy    `json:"bosses"`
	Chips         []AdminFieldGGOChip     `json:"chips"`
	Extraction    AdminFieldGGOExtraction `json:"extraction"`
	Rules         AdminFieldGGORules      `json:"rules"` // Rules 为搜打撤政策与经济规则，尚未接入运行时消费。
}

// AdminFieldGGORules 保存已确认的固定政策、可调参数和仍待决定的项目。
// 空值字段表示尚未决定，保存草案时不推断概率、兑换值或正式限购。
type AdminFieldGGORules struct {
	EntryMethod         string `json:"entryMethod"`         // EntryMethod 固定为npc_random。
	ProtectionSeconds   int64  `json:"protectionSeconds"`   // ProtectionSeconds 为入场保护秒数，0至300，默认30。
	TerminalName        string `json:"terminalName"`        // TerminalName 固定为临时终端。
	UnlimitedCapacity   bool   `json:"unlimitedCapacity"`   // UnlimitedCapacity 固定为true。
	SearchEnabled       bool   `json:"searchEnabled"`       // SearchEnabled 固定为false。
	InitialSource       string `json:"initialSource"`       // InitialSource 固定为monsters。
	PvpTransfer         string `json:"pvpTransfer"`         // PvpTransfer 固定为winner_terminal。
	DeathReturn         string `json:"deathReturn"`         // DeathReturn 固定为safe_area。
	DeathLossPercent    *int64 `json:"deathLossPercent"`    // DeathLossPercent 为死亡损失百分比，0至100；nil表示待决定。
	PveLossMode         string `json:"pveLossMode"`         // PveLossMode 为pending待决定或destroy。
	ExtractionReturn    string `json:"extractionReturn"`    // ExtractionReturn 固定为safe_area。
	ExtractionSeconds   *int64 `json:"extractionSeconds"`   // ExtractionSeconds 为撤离秒数，0至300；nil表示待决定。
	SettlementMode      string `json:"settlementMode"`      // SettlementMode 固定为safe_npc_or_confirm_leave。
	RetainAfterDeath    bool   `json:"retainAfterDeath"`    // RetainAfterDeath 固定为true。
	SafeTradeEnabled    bool   `json:"safeTradeEnabled"`    // SafeTradeEnabled 默认true，可调整。
	TradeAssets         string `json:"tradeAssets"`         // TradeAssets 为pending待决定或chips_only。
	AllowGift           *bool  `json:"allowGift"`           // AllowGift 为是否允许赠送；nil表示待决定。
	CreditCap           string `json:"creditCap"`           // CreditCap 固定为none。
	ShopLimitScope      string `json:"shopLimitScope"`      // ShopLimitScope 固定为account_weekly。
	WeeklyLimit         *int64 `json:"weeklyLimit"`         // WeeklyLimit 为账号每周限购，1至10000；nil表示待决定。
	WeeklyResetDay      *int64 `json:"weeklyResetDay"`      // WeeklyResetDay 为周重置日，1至7表示周一到周日；nil表示待决定。
	WeeklyResetHour     *int64 `json:"weeklyResetHour"`     // WeeklyResetHour 为重置小时，0至23；nil表示待决定。
	WeeklyResetMinute   *int64 `json:"weeklyResetMinute"`   // WeeklyResetMinute 为重置分钟，0至59；nil表示待决定。
	WeeklyResetTimezone string `json:"weeklyResetTimezone"` // WeeklyResetTimezone 固定为Asia/Shanghai。
	WeeklyServerScope   string `json:"weeklyServerScope"`   // WeeklyServerScope 为pending、current_server或all_servers。
	LeaveConfirmation   string `json:"leaveConfirmation"`   // LeaveConfirmation 为离开确认文案，可保留{信用点}占位。
	LeaveConfirmButton  string `json:"leaveConfirmButton"`  // LeaveConfirmButton 为确认离开按钮文案。
	LeaveCancelButton   string `json:"leaveCancelButton"`   // LeaveCancelButton 为取消按钮文案。
}

// AdminFieldGGOPosition 为可选场景坐标；填写时三个轴均必需。
type AdminFieldGGOPosition struct {
	X *float64 `json:"x"`
	Y *float64 `json:"y"`
	Z *float64 `json:"z"`
}

// AdminFieldGGONPC 为入口传送、撤离接应或终端结算的配置候选。
type AdminFieldGGONPC struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	HeroID       string                 `json:"heroId"`
	Role         string                 `json:"role"`
	PointLabel   string                 `json:"pointLabel"`
	Dialogue     string                 `json:"dialogue"`
	Position     *AdminFieldGGOPosition `json:"position"`
	SettlesChips bool                   `json:"settlesChips"`
}

// AdminFieldGGOEnemy 为预备怪物模型及独立复活时间。
type AdminFieldGGOEnemy struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	HeroID    string `json:"heroId"`
	RespawnMS int64  `json:"respawnMs"`
}

// AdminFieldGGOChip 的兑换值为空表示待决定；掉落权重与数量为空表示待配置，不阻止草案保存。
type AdminFieldGGOChip struct {
	ItemID        string `json:"itemId"`
	Name          string `json:"name"`
	ExchangeValue *int64 `json:"exchangeValue"`
	DropWeight    *int64 `json:"dropWeight"` // DropWeight 为六品质统一小怪掉落池权重，0至1000000；nil表示待配置。
	DropAmount    *int64 `json:"dropAmount"` // DropAmount 为单次掉落数量，1至10000；nil表示待配置。
}

// AdminFieldGGOExtraction 为可编辑的撤离提示；只说明返回初始安全区，不交付芯片结算。
type AdminFieldGGOExtraction struct {
	Arrival        string `json:"arrival"`
	Confirmation   string `json:"confirmation"`
	ConfirmButton  string `json:"confirmButton"`
	ContinueButton string `json:"continueButton"`
	Countdown      string `json:"countdown"`
	Cancelled      string `json:"cancelled"`
}
