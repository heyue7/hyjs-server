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
// @Description 采集点按北京时间整点刷新；世界首领使用独立次数，首领配置用于后续新开战。
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
// @Description 采集刷新时刻为1至24个唯一0至23整数；首领奖励-1不限次数，0停止奖励，跨地图共用每日次数。
// @Tags AdminFieldResources
// @Accept json
// @Produce json
// @Param body body model.AdminFieldResourcesSaveRequest true "奖励总次数与修改原因"
// @Success 200 {object} model.AdminFieldResourcesResponse
// @Router /admin/api/field-resources [put]
func AdminFieldResourcesSaveHandler(c *gin.Context) {
	var req model.AdminFieldResourcesSaveRequest
	if c.ShouldBindJSON(&req) != nil || req.GatherRefreshHours != nil && !adminValidGatherHours(req.GatherRefreshHours) || req.BossDailyRewards < constUtil.FieldBossRewardsUnlimited || req.BossDailyRewards > constUtil.FieldBossRewardsDailyMaximum || strings.TrimSpace(req.Reason) == "" || len([]rune(req.Reason)) > 500 {
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

func adminValidGatherHours(hours []int64) bool {
	if len(hours) < 1 || len(hours) > 24 {
		return false
	}
	seen := map[int64]bool{}
	for _, h := range hours {
		if h < 0 || h > 23 || seen[h] {
			return false
		}
		seen[h] = true
	}
	return true
}
