package model

// AdminItemsRequest 是已经由目录Handler校验的筛选及分页条件。
type AdminItemsRequest struct {
	PreferredIDs []string `json:"preferredIds,omitempty"` // PreferredIDs changes ordering only; filters and full pagination remain available.
	// Q 是原名称、ID或说明的搜索词。
	Q string `json:"q"`
	// Category 是目录facets.categories内的分组值，空表示全部。
	Category string `json:"category"`
	// Quality 是统一颜色枚举0未标注、1白、2绿、3蓝、4紫、5橙、6红；nil表示全部。
	Quality *int64 `json:"quality,omitempty"`
	// ApplicableType 是目录facets.applicableTypes内带来源分组的原枚举键。
	ApplicableType string `json:"applicableType"`
	// IDs 是精确原资源ID集合，空表示不限。
	IDs []string `json:"ids"`
	// Page 是从1开始的页码。
	Page int64 `json:"page"`
	// PageSize 是每页条目数。
	PageSize int64 `json:"pageSize"`
}

// AdminCatalogueOption 是原资源分类或原适用类型的可解释筛选项。
type AdminCatalogueOption struct {
	// Value 是分类ID或带原字段分组的稳定键。
	Value string `json:"value"`
	// Label 是中文分组或原枚举的中文说明。
	Label string `json:"label"`
}

// AdminCatalogueQualityOption 是统一颜色筛选项，保留数字作为接口值。
type AdminCatalogueQualityOption struct {
	// Value 是0未标注、1白、2绿、3蓝、4紫、5橙、6红。
	Value int64 `json:"value"`
	// Label 是该原数值的筛选名称。
	Label string `json:"label"`
}

// AdminCatalogueFacets 是从实际目录汇总的筛选集合。
type AdminCatalogueFacets struct {
	// Categories 是真实资源分类。
	Categories []AdminCatalogueOption `json:"categories"`
	// Qualities 是目录内实际出现的统一颜色品级。
	Qualities []AdminCatalogueQualityOption `json:"qualities"`
	// ApplicableTypes 是按原字段分组的实际枚举。
	ApplicableTypes []AdminCatalogueOption `json:"applicableTypes"`
}

// AdminCatalogueData 在启动时独立加载，后续只读；不向战斗Tables加入文本或资源表。
type AdminCatalogueData struct {
	// SourceTables keeps the independently loaded original catalogue rows read-only for item details.
	SourceTables map[string][]map[string]string
	// SpriteURLs maps original sprite names to verified same-origin art, read-only after startup.
	SpriteURLs map[string]string
	// DisplayNames maps original text keys to their localized display label.
	DisplayNames map[string]string
	// Items 是按ID排序的明确物品，矿点、钓点及怪物不混入可发放目录。
	Items []AdminItemDTO
	// UnionFightItems 仅供公会战商店选择，独立于可直接发放的背包资源。
	UnionFightItems []AdminItemDTO
	// Categories 保留旧目录响应的分类ID列表。
	Categories []string
	// ActivityPreviews 是各模式的原美术部件图片，不代表完整页面截图。
	ActivityPreviews map[string]string
	// Facets 是与Items同源的实际筛选项。
	Facets AdminCatalogueFacets
}
