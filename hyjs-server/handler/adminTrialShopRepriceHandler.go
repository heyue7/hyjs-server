package handler

import (
	"encoding/hex"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
)

// AdminTrialShopRepriceHandler 通过既有管理鉴权、CSRF及版本校验执行固定试炼商品调整。
// @Summary 应用试炼红装碎片价格
// @Description 仅将四件指定的十片红装碎片调整为150攻略点并设置历史星数门槛，保留其它商品、购买历史和库存。
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminTrialShopRepriceRequest true "当前试炼目录版本"
// @Success 200 {object} model.AdminShopResponse
// @Router /admin/api/shops/point/trial-reprice [post]
func AdminTrialShopRepriceHandler(c *gin.Context) {
	var req model.AdminTrialShopRepriceRequest
	if c.ShouldBindJSON(&req) != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if decoded, err := hex.DecodeString(req.Version); err != nil || len(decoded) != 32 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.ApplyTrialShopReprice(c.Request.Context(), adminPrincipal(c), req.Version)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
