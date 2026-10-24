package handler

import (
	"encoding/hex"
	"strings"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
)

func RegisterAdminFieldResourceRoutes(a *gin.RouterGroup) {
	a.GET("/field-resources", AdminFieldResourcesHandler)
	a.PUT("/field-resources", AdminFieldResourcesSaveHandler)
}

// AdminFieldResourcesHandler 读取普通野外首领跨地图每日奖励限额。
// @Summary 野外资源设置
// @Description 世界首领使用独立次数，配置仅用于后续普通野外首领新开战。
// @Tags AdminFieldResources
// @Produce json
// @Success 200 {object} model.AdminFieldResourcesResponse
// @Router /admin/api/field-resources [get]
func AdminFieldResourcesHandler(c *gin.Context) {
	if c.Request.URL.RawQuery != "" {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminFieldResources(c.Request.Context(), adminPrincipal(c))
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminFieldResourcesSaveHandler 按版本保存并审计普通野外首领资源规则。
// @Summary 保存野外资源设置
// @Description -1不限次数，0停止奖励；普通首领奖励跨地图共用每日总次数，世界首领独立。
// @Tags AdminFieldResources
// @Accept json
// @Produce json
// @Param body body model.AdminFieldResourcesSaveRequest true "奖励总次数与修改原因"
// @Success 200 {object} model.AdminFieldResourcesResponse
// @Router /admin/api/field-resources [put]
func AdminFieldResourcesSaveHandler(c *gin.Context) {
	var req model.AdminFieldResourcesSaveRequest
	if c.ShouldBindJSON(&req) != nil || req.BossDailyRewards < constUtil.FieldBossRewardsUnlimited || req.BossDailyRewards > constUtil.FieldBossRewardsDailyMaximum || strings.TrimSpace(req.Reason) == "" || len([]rune(req.Reason)) > 500 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if decoded, err := hex.DecodeString(req.Version); err != nil || len(decoded) != 32 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminSaveFieldResources(c.Request.Context(), adminPrincipal(c), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
