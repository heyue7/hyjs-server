package handler

import (
	"encoding/hex"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
)

// AdminTrialDailyGoodsHandler 应用已确认的试炼每日商品规则。
// @Summary 应用试炼每日商品
// @Description 固定每日商品与饰品碎片价格门槛，保留原奖励和购买历史。
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminTrialDailyGoodsRequest true "当前试炼目录版本"
// @Success 200 {object} model.AdminShopResponse
// @Router /admin/api/shops/point/daily-goods [post]
func AdminTrialDailyGoodsHandler(c *gin.Context) {
	var req model.AdminTrialDailyGoodsRequest
	if c.ShouldBindJSON(&req) != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if decoded, err := hex.DecodeString(req.Version); err != nil || len(decoded) != 32 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.ApplyTrialDailyGoods(c.Request.Context(), adminPrincipal(c), req.Version)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
