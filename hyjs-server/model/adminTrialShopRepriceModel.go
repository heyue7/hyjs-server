package model

// AdminTrialShopRepriceRequest 绑定当前试炼目录版本，执行已确认的四件红装碎片价格及星数调整。
type AdminTrialShopRepriceRequest struct {
	// Version 来自GET /admin/api/shops/point，用于并发编辑校验。
	Version string `json:"version" binding:"required,len=64"`
}
