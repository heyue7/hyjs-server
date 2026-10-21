package model

// AdminInventoryAttribute 是原存档中允许编辑的数值字段及当前合法范围。
type AdminInventoryAttribute struct {
	Key   string `json:"key"`   // Key 是类型化字段路径。
	Label string `json:"label"` // Label 是业务名称。
	Value int64  `json:"value"` // Value 是当前值。
	Min   int64  `json:"min"`   // Min 是当前实例最小值。
	Max   int64  `json:"max"`   // Max 是原表及角色资格允许的最大值。
}

// AdminInventoryRow 是实例或堆叠背包行。
type AdminInventoryRow struct {
	Key           string                    `json:"key"`           // Key 与Category共同确定身份。
	Category      string                    `json:"category"`      // Category 为warrior/equipment/accessory/item/gem/cooking/pet。
	ResourceID    string                    `json:"resourceId"`    // ResourceID 是原资源ID。
	Name          string                    `json:"name"`          // Name 是目录名称。
	Quality       int64                     `json:"quality"`       // Quality 为0至6统一品质。
	QualityLabel  string                    `json:"qualityLabel"`  // QualityLabel 是品质名称。
	IconURL       string                    `json:"iconUrl"`       // IconURL 是原图标地址。
	Amount        int64                     `json:"amount"`        // Amount 是实例1或堆叠数量。
	GUID          string                    `json:"guid"`          // GUID 是实例身份。
	Level         int64                     `json:"level"`         // Level 是当前成长等级。
	Attributes    []AdminInventoryAttribute `json:"attributes"`    // Attributes 是数值白名单。
	Editable      bool                      `json:"editable"`      // Editable 表示可编辑数值。
	Removable     bool                      `json:"removable"`     // Removable 表示未被关联保护。
	BlockedReason string                    `json:"blockedReason"` // BlockedReason 是保护原因。
}

// AdminInventoryCategory 是全量背包分类统计。
type AdminInventoryCategory struct {
	Value string `json:"value"` // Value 是分类键。
	Label string `json:"label"` // Label 是中文分类。
	Count int64  `json:"count"` // Count 是全量行数。
}

// AdminPlayerInventoryResponse 返回完整筛选后的分页背包。
type AdminPlayerInventoryResponse struct {
	Items      []AdminInventoryRow      `json:"items"`      // Items 是本页条目。
	Total      int64                    `json:"total"`      // Total 是筛选后的总行数。
	Version    string                   `json:"version"`    // Version 绑定存档与精灵数据。
	Categories []AdminInventoryCategory `json:"categories"` // Categories 是全部分类统计。
}

// AdminPlayerInventoryCatalogueResponse 是可通过原生创建入口添加的目录。
type AdminPlayerInventoryCatalogueResponse struct {
	Items []AdminItemDTO `json:"items"` // Items 是本页可添加资源。
	Total int64          `json:"total"` // Total 是完整筛选总数。
}

// AdminPlayerInventoryRequest 是有版本与幂等保护的单行操作。
type AdminPlayerInventoryRequest struct {
	Version    string           `json:"version" binding:"required"`   // Version 是读取版本。
	RequestID  string           `json:"requestId" binding:"required"` // RequestID 是UUID幂等键。
	Operation  string           `json:"operation" binding:"required"` // Operation 为add/update/remove。
	Category   string           `json:"category" binding:"required"`  // Category 为七种真实存储分类。
	Key        string           `json:"key,omitempty"`                // Key 是更新或移除身份。
	ResourceID string           `json:"resourceId,omitempty"`         // ResourceID 是添加资源ID。
	Amount     *int64           `json:"amount,omitempty"`             // Amount 是堆叠总数或添加数量。
	Attributes map[string]int64 `json:"attributes,omitempty"`         // Attributes 仅接受白名单数值字段。
	Reason     string           `json:"reason" binding:"required"`    // Reason 是审计原因。
}

// AdminPlayerInventoryMutationResponse 返回提交后的版本。
type AdminPlayerInventoryMutationResponse struct {
	Version string `json:"version"` // Version 是最新背包版本。
}
