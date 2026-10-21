package model

// AdminGamePlayerDTO 仅包含游戏运营需要的角色摘要，不包含登录凭据或完整存档。
type AdminGamePlayerDTO struct {
	AccountID int64                      `json:"accountId"`           // AccountID is the global account owning this realm role.
	Username  string                     `json:"username"`            // Username is the global login name.
	Nickname  string                     `json:"nickname"`            // Nickname is the global account display name.
	ServerID  int64                      `json:"serverId"`            // ServerID is immutable role ownership.
	ID        int64                      `json:"id"`                  // ID 是游戏角色ID。
	Name      string                     `json:"name"`                // Name 是游戏昵称。
	Level     int64                      `json:"level"`               // Level 是当前等级。
	Exp       int64                      `json:"exp"`                 // Exp 是当前等级的经验。
	Vip       int64                      `json:"vip"`                 // Vip 是当前VIP等级。
	Chapter   string                     `json:"chapter"`             // Chapter 是当前主线章节。
	Mission   string                     `json:"mission"`             // Mission 是当前主线关卡。
	Scene     string                     `json:"scene"`               // Scene 是所在场景。
	CreatedAt int64                      `json:"createdAt"`           // CreatedAt 是创建毫秒。
	Version   string                     `json:"version"`             // Version 绑定当前存档及会话版本，防止覆盖新登录和进度。
	Balances  map[string]int64           `json:"balances"`            // Balances 是主要原游戏货币与体力。
	Counts    map[string]int64           `json:"counts"`              // Counts 是角色/装备/饰品及物品种类数。
	Inventory []AdminPlayerInventoryItem `json:"inventory,omitempty"` // Inventory 是详情有限资源清单。
}

// AdminPlayerInventoryItem 只投影物品身份、数量及等级。
type AdminPlayerInventoryItem struct {
	// ResourceID 是原目录资源ID。
	ResourceID string `json:"resourceId"`
	// Amount 是原资源数量。
	Amount int64 `json:"amount"`
	// Guid 是实例GUID，堆叠物品为空。
	Guid string `json:"guid,omitempty"`
	// Level 是当前实例等级。
	Level int64 `json:"level,omitempty"`
}

// AdminGamePlayersResponse 返回分页角色目录。
type AdminGamePlayersResponse struct {
	// Items 是本页角色摘要。
	Items []AdminGamePlayerDTO `json:"items"`
	// Total 是符合筛选条件的角色总数。
	Total int64 `json:"total"`
}

// AdminGamePlayerNameRequest 记录有原因且不消费游戏资源的运营改名。
type AdminGamePlayerNameRequest struct {
	// Version 是读取时的乐观锁摘要。
	Version string `json:"version" binding:"required"`
	// Name 是当前业务显示名称。
	Name string `json:"name" binding:"required"`
	// Reason 是操作原因，写入审计。
	Reason string `json:"reason" binding:"required"`
}

// AdminGamePlayerKickRequest 轮换角色会话，使旧连接下次请求失效。
type AdminGamePlayerKickRequest struct {
	// Version 是读取时的乐观锁摘要。
	Version string `json:"version" binding:"required"`
	// Reason 是操作原因，写入审计。
	Reason string `json:"reason" binding:"required"`
}

// AdminGamePlayerLevelRequest 按当前版本调整角色等级。
type AdminGamePlayerLevelRequest struct {
	// Version 是读取时的存档摘要。
	Version string `json:"version" binding:"required"`
	// Level 是原等级表中存在的目标等级，当前范围1至100。
	Level int64 `json:"level" binding:"required"`
	// Reason 是记录到审计的操作原因。
	Reason string `json:"reason" binding:"required"`
}
