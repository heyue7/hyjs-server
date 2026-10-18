package model

import "encoding/json"

// AdminItemDTO 是原资源目录条目，名称沿原文本，不创建新物品。
type AdminItemDTO struct {
	// ID 是原资源ID。
	ID string `json:"id"`
	// Name 是独立TableText索引解析的名称，无法确定时显示ID。
	Name string `json:"name"`
	// Category 是响应facets.categories中的原资源分组。
	Category string `json:"category"`
	// Quality 是后台统一颜色枚举：0未标注、1白、2绿、3蓝、4紫、5橙、6红。
	Quality int64 `json:"quality"`
	// QualityLabel 是颜色与稀有度的中文显示名称。
	QualityLabel string `json:"qualityLabel"`
	// QualityColor 是gray/white/green/blue/purple/orange/red颜色语义。
	QualityColor string `json:"qualityColor"`
	// OriginalQuality 保留原表数值，供兼容与追溯，不参与颜色筛选。
	OriginalQuality int64 `json:"originalQuality"`
	// Description 是原说明文本。
	Description string `json:"description"`
	// Icon 是原sprite名称。
	Icon string `json:"icon"`
	// IconURL 是已导出原像素的同源地址，无可用原图时为空。
	IconURL string `json:"iconUrl"`
	// PreviewURL 是已导出原图预览地址，无大图时复用原缩略图。
	PreviewURL string `json:"previewUrl"`
	// ApplicableTypes 是按原表字段分组的实际枚举键。
	ApplicableTypes []string `json:"applicableTypes"`
}

// AdminItemsResponse 返回真实分页目录及完整的原数据筛选项。
type AdminItemsResponse struct {
	// Items 是筛选后的本页条目。
	Items []AdminItemDTO `json:"items"`
	// Total 是全部筛选结果数量。
	Total int64 `json:"total"`
	// Categories 保留既有前端分类ID合同。
	Categories []string `json:"categories"`
	// Facets 是实际资源对应的中文筛选选项。
	Facets AdminCatalogueFacets `json:"facets"`
}

// AdminActivityDTO 展示原业务配置及乐观锁版本。
type AdminActivityDTO struct {
	ID         string          `json:"id"`         // ID 是域前缀与原实例ID。
	Name       string          `json:"name"`       // Name 是活动名称。
	Type       string          `json:"type"`       // Type 是pool/commerce/welfare/period/registry。
	Enabled    bool            `json:"enabled"`    // Enabled 表示实际运行开关。
	Source     string          `json:"source"`     // Source 是来源说明。
	Configured bool            `json:"configured"` // Configured 表示存在可编辑业务规则。
	Version    string          `json:"version"`    // Version 是当前内容摘要。
	Config     json.RawMessage `json:"config"`     // Config 是该类型的有限运营字段。
}

// AdminActivityUpdateRequest 更新已存在业务定义，不重写已冻结快照。
type AdminActivityUpdateRequest struct {
	Enabled bool            `json:"enabled"`                    // Enabled 是目标开关。
	Version string          `json:"version" binding:"required"` // Version 是读取时的摘要。
	Config  json.RawMessage `json:"config" binding:"required"`  // Config 是完整编辑对象。
}

// AdminActivityCreateRequest 从既有同类模板复制新运营期，旧记录保持。
type AdminActivityCreateRequest struct {
	TemplateID string          `json:"templateId" binding:"required"` // TemplateID 是已有活动域ID。
	NewID      string          `json:"newId" binding:"required"`      // NewID 是同类新实例ID。
	Config     json.RawMessage `json:"config" binding:"required"`     // Config 是新规则。
	Enabled    bool            `json:"enabled"`                       // Enabled 是是否立即开放。
}

// AdminAnnouncementRequest 保存纯文本公告和发布窗口。
type AdminAnnouncementRequest struct {
	Title   string `json:"title" binding:"required"`   // Title 是公告标题。
	Content string `json:"content" binding:"required"` // Content 是纯文本正文。
	Enabled bool   `json:"enabled"`                    // Enabled 是是否发布。
	StartAt int64  `json:"startAt"`                    // StartAt 是起始毫秒，0立即。
	EndAt   int64  `json:"endAt"`                      // EndAt 是截止毫秒，0不限。
	Version string `json:"version"`                    // Version 是编辑时摘要，新建为空。
}

// AdminGrantRequest 经原领奖中心投递明确单项附件。
type AdminGrantRequest struct {
	PlayerID   int64  `json:"playerId" binding:"required"`   // PlayerID 是目标游戏角色。
	ResourceID string `json:"resourceId" binding:"required"` // ResourceID 是原资源ID。
	Amount     int64  `json:"amount" binding:"required"`     // Amount 是正整数数量。
	Reason     string `json:"reason" binding:"required"`     // Reason 是投递原因。
	RequestID  string `json:"requestId" binding:"required"`  // RequestID 是幂等UUID。
}

// AdminGrantResponse 返回已持久化的待领取附件。
type AdminGrantResponse struct {
	DeliveryID int64  `json:"deliveryId"` // DeliveryID 是原领奖中心ID。
	RequestID  string `json:"requestId"`  // RequestID 是幂等键。
}

// AdminLoginRequest 是后台独立登录凭据。
type AdminLoginRequest struct {
	Username string `json:"username" binding:"required"` // Username 是后台账号。
	Password string `json:"password" binding:"required"` // Password 是口令。
}
