package model

// AdminFieldGGOConfiguration 保存沙漠搜打撤的设计配置，供后台准备。
type AdminFieldGGOConfiguration struct {
	Title      string                  `json:"title"`
	ModeName   string                  `json:"modeName"`
	CurrencyID string                  `json:"currencyId"`
	ShopKey    string                  `json:"shopKey"`
	CarryIcon  string                  `json:"carryIcon"`
	SproutIcon string                  `json:"sproutIcon"`
	NPCs       []AdminFieldGGONPC      `json:"npcs"`
	Monsters   []AdminFieldGGOEnemy    `json:"monsters"`
	Bosses     []AdminFieldGGOEnemy    `json:"bosses"`
	Chips      []AdminFieldGGOChip     `json:"chips"`
	Extraction AdminFieldGGOExtraction `json:"extraction"`
}

// AdminFieldGGOPosition 为可选场景坐标；填写时三个轴均必需。
type AdminFieldGGOPosition struct {
	X *float64 `json:"x"`
	Y *float64 `json:"y"`
	Z *float64 `json:"z"`
}

// AdminFieldGGONPC 为向导或兼任撤离与结算的接应人。
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

// AdminFieldGGOChip 的兑换值为空表示待决定。
type AdminFieldGGOChip struct {
	ItemID        string `json:"itemId"`
	Name          string `json:"name"`
	ExchangeValue *int64 `json:"exchangeValue"`
}

// AdminFieldGGOExtraction 为可编辑的撤离提示。
type AdminFieldGGOExtraction struct {
	Arrival        string `json:"arrival"`
	Confirmation   string `json:"confirmation"`
	ConfirmButton  string `json:"confirmButton"`
	ContinueButton string `json:"continueButton"`
	Countdown      string `json:"countdown"`
	Cancelled      string `json:"cancelled"`
}
