package handler

import (
	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/service"
	"strings"
	"time"
)

// RegisterAdminShopRoutes attaches typed shop management to the existing authenticated CSRF group.
func RegisterAdminShopRoutes(a *gin.RouterGroup) {
	a.GET("/shops", AdminShopsHandler)
	a.GET("/shops/:id", AdminShopHandler)
	a.POST("/shops/:id/products", AdminShopProductSaveHandler)
	a.PUT("/shops/:id/products/:productId", AdminShopProductSaveHandler)
	a.DELETE("/shops/:id/products/:productId", AdminShopProductDeleteHandler)
	a.PUT("/shops/:id/settings", AdminShopSettingsSaveHandler)
}

// AdminShopsHandler lists actual original shop menus.
// @Summary 商店分类
// @Description 原消费者和支持字段
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {array} model.AdminShopDefinition
// @Router /admin/api/shops [get]
func AdminShopsHandler(c *gin.Context) {
	adminSuccess(c, gin.H{"items": service.AdminShopDefinitions()})
}

// AdminShopHandler reads actual products and source settings.
// @Summary 商店商品
// @Description 返回原目录及版本
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path string true "商店"
// @Success 200 {object} model.AdminShopResponse
// @Router /admin/api/shops/{id} [get]
func AdminShopHandler(c *gin.Context) {
	if !service.AdminShopValid(c.Param("id")) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminShop(c.Request.Context(), c.Param("id"))
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminShopProductSaveHandler validates a concrete product before changing its original consumer config.
// @Summary 保存商店商品
// @Description 不改变已有购买历史；已冻结运营期需先复制
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path string true "商店"
// @Param body body model.AdminShopProductSaveRequest true "商品"
// @Success 200 {object} model.AdminShopResponse
// @Router /admin/api/shops/{id}/products [post]
// @Router /admin/api/shops/{id}/products/{productId} [put]
func AdminShopProductSaveHandler(c *gin.Context) {
	var req model.AdminShopProductSaveRequest
	key := c.Param("id")
	if c.ShouldBindJSON(&req) != nil || !service.AdminShopValid(key) || len(req.Version) != 64 || len(req.Name) > 128 || len(req.Source) > 1000 || len(req.Rewards) < 1 || len(req.Rewards) > 50 || len(c.Param("productId")) > 180 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	for _, r := range req.Rewards {
		if strings.TrimSpace(r.ResourceID) != r.ResourceID || r.ResourceID == "" || len(r.ResourceID) > 64 || r.Amount < 1 || r.Amount > 10000 {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
	}
	if req.Cost.ResourceID == "" || len(req.Cost.ResourceID) > 64 || strings.ContainsAny(req.Cost.ResourceID, ",; \r\n") || req.Cost.Amount < 0 || req.Cost.Amount > 2147483647 || (key != "union-fight" && req.Cost.Amount == 0) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	for _, n := range []int64{req.DailyLimit, req.TotalLimit, req.MinLevel, req.MinVip, req.ContributionCost, req.GuildLevel, req.Priority, req.MaxRank} {
		if n < 0 || n > 2147483647 {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
	}
	if req.Stock < -1 || req.Stock > 2147483647 || req.Weight < 0 || req.Weight > 1000000000000 || req.DiscountBps < 0 || req.DiscountBps > 10000 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	switch key {
	case "union-fight":
		day, err := time.Parse("2006-01-02", req.Day)
		if err != nil || day.Year() < 2000 || day.Year() > 2200 || len(req.Rewards) != 1 || req.Stock < 1 || req.BuyCondition < 0 || req.BuyCondition > 2 || req.RefreshMillis < 1000 || req.RefreshMillis > 86400000 || req.RefreshMillis%1000 != 0 || strings.TrimSpace(req.Source) == "" {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
	case "pet-market":
		if req.Stock < 1 || req.Stock > 100 || req.Weight < 1 || len(req.Rewards) != 1 {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
	case "vip":
		if req.MinVip < 1 || req.MinVip > 12 || req.Cost.Amount > 100000 || strings.TrimSpace(req.Source) == "" {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
	default:
		if req.DailyLimit < 1 || req.DailyLimit > 10000 || req.MinLevel < 1 || req.Weight < 1 || req.DiscountBps < 1 {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
	}
	out, err := gameService.AdminSaveShopProduct(c.Request.Context(), adminPrincipal(c), key, c.Param("productId"), req, c.Request.Method == "POST")
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminShopProductDeleteHandler removes future availability and retains historical rows/receipts.
// @Summary 移除商店商品
// @Description 已冻结交易与库存保留
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path string true "商店"
// @Param productId path string true "商品"
// @Param body body model.AdminShopProductDeleteRequest true "版本"
// @Success 200 {object} model.AdminShopResponse
// @Router /admin/api/shops/{id}/products/{productId} [delete]
func AdminShopProductDeleteHandler(c *gin.Context) {
	var req model.AdminShopProductDeleteRequest
	if c.ShouldBindJSON(&req) != nil || !service.AdminShopValid(c.Param("id")) || len(req.Version) != 64 || c.Param("productId") == "" || len(c.Param("productId")) > 180 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminDeleteShopProduct(c.Request.Context(), adminPrincipal(c), c.Param("id"), c.Param("productId"), req.Version)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminShopSettingsSaveHandler edits a source or explicitly creates a closed successor period.
// @Summary 保存商店设置
// @Description 新期默认关闭，保留历史限购快照
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path string true "商店"
// @Param body body model.AdminShopSettingsSaveRequest true "设置"
// @Success 200 {object} model.AdminShopResponse
// @Router /admin/api/shops/{id}/settings [put]
func AdminShopSettingsSaveHandler(c *gin.Context) {
	var req model.AdminShopSettingsSaveRequest
	key := c.Param("id")
	if c.ShouldBindJSON(&req) != nil || !service.AdminShopValid(key) || len(req.Version) != 64 || len(req.Source) > 1000 || req.Slots < 0 || req.Slots > 16 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if !req.Unlimited && (req.StartAt != 0 || req.EndAt != 0 || req.ClaimEndAt != 0) && (req.StartAt <= 0 || req.StartAt >= req.EndAt || req.ClaimEndAt < req.EndAt || req.ClaimEndAt > 7258118400000) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if key != "pet-market" && key != "vip" && key != "union-fight" && strings.TrimSpace(req.Source) == "" {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if (key == "pet-market" || key == "memory") && req.Slots < 1 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminSaveShopSettings(c.Request.Context(), adminPrincipal(c), key, req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
