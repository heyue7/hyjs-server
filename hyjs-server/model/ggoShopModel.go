package model

// GGOShopCatalogueRequest 查询信用商店。
type GGOShopCatalogueRequest struct {
	PlayerID   int64  `json:"playerId" binding:"required"`   // 角色ID。
	SessionHex string `json:"sessionHex" binding:"required"` // 当前16字节会话的十六进制字符串。
}

// GGOShopBuyRequest 购买一份当前版本商品；requestId为UUID。
type GGOShopBuyRequest struct {
	PlayerID   int64  `json:"playerId" binding:"required"`   // 角色ID。
	SessionHex string `json:"sessionHex" binding:"required"` // 当前16字节会话的十六进制字符串。
	ProductID  string `json:"productId" binding:"required"`  // 目录中的商品ID。
	Version    string `json:"version" binding:"required"`    // 当前目录版本。
	RequestID  string `json:"requestId" binding:"required"`  // 本次购买唯一UUID。
}

// GGOShopItem 服务器权威价格、数量与剩余次数。
type GGOShopItem struct {
	ID          string `json:"id"`          // 商品ID。
	Name        string `json:"name"`        // 展示名称。
	ResourceID  string `json:"resourceId"`  // 奖励道具ID。
	Amount      int64  `json:"amount"`      // 每份奖励数量。
	Price       int64  `json:"price"`       // 每份所需信用点。
	Remaining   int64  `json:"remaining"`   // 当前角色剩余可购买次数。
	Description string `json:"description"` // 道具说明。
	Icon        string `json:"icon"`        // 原客户端图标资源名。
	Quality     int64  `json:"quality"`     // 品质，1至6。
}

// GGOShopCatalogueResponse 包含道具货币余额和可购买目录。
type GGOShopCatalogueResponse struct {
	Name         string        `json:"name"`         // 展示名称。
	CurrencyID   string        `json:"currencyId"`   // 货币道具ID。
	CurrencyName string        `json:"currencyName"` // 货币名称。
	CurrencyIcon string        `json:"currencyIcon"` // 货币图标资源名。
	Balance      int64         `json:"balance"`      // 当前货币余额。
	Version      string        `json:"version"`      // 当前目录版本。
	Enabled      bool          `json:"enabled"`      // false关闭，true开放。
	Items        []GGOShopItem `json:"items"`        // 可展示商品，空目录返回空数组。
}

// GGOShopBuyResponse 保存原始成功回执，重试返回相同内容；terminalPointsAdded单位为点。
type GGOShopBuyResponse struct {
	Catalogue           *GGOShopCatalogueResponse `json:"catalogue"`           // 购买后权威目录。
	RewardID            string                    `json:"rewardId"`            // 本次获得的道具ID。
	RewardAmount        int64                     `json:"rewardAmount"`        // 本次获得数量。
	TerminalPointsAdded int64                     `json:"terminalPointsAdded"` // 本次新增完整超频点数，非终端最小金额单位。
	RequestID           string                    `json:"requestId"`           // 本次购买唯一UUID。
}
