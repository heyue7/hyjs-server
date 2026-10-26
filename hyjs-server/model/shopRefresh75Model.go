package model

// ShopRefresh75StatusRequest 查询当前角色试炼商店各菜单的刷新时间。
type ShopRefresh75StatusRequest struct {
	PlayerID   int64  `json:"playerId" binding:"required"`   // 当前已创建角色编号。
	SessionHex string `json:"sessionHex" binding:"required"` // 该角色16字节原会话，使用32位十六进制。
	ShopType   int64  `json:"shopType" binding:"required"`   // 原商店类型，当前仅支持5（试炼商店）。
}

// ShopRefresh75Menu 是原试炼商店一个菜单的游戏日历刷新状态。
type ShopRefresh75Menu struct {
	Group     int64  `json:"group"`     // 原菜单序号：1商品、2紫装、3橙装、4红装。
	Name      string `json:"name"`      // 玩家可读菜单名称。
	Period    string `json:"period"`    // day表示每日，week表示当前已发布的周货周期。
	Available bool   `json:"available"` // true有可用商品周期；false时resetAt为空，不编造结束时间。
	ResetAt   *int64 `json:"resetAt"`   // 下次游戏日0点或当前真实周货窗口结束毫秒；未开放为null。
}

// ShopRefresh75StatusResponse 供客户端以同一游戏时钟累计倒计时。
type ShopRefresh75StatusResponse struct {
	ServerTime int64               `json:"serverTime"` // 当前已绑定区服的虚拟游戏Unix毫秒。
	Menus      []ShopRefresh75Menu `json:"menus"`      // 四个原菜单状态，按group顺序返回。
}
