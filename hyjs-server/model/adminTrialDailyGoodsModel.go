package model

// AdminTrialDailyGoodsRequest 使用当前试炼目录版本应用每日商品配置。
type AdminTrialDailyGoodsRequest struct {
	// Version 来自 GET /admin/api/shops/point，防止覆盖并发编辑。
	Version string `json:"version" binding:"required,len=64"`
}
