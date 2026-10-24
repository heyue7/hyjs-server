package handler

import (
	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"strings"
)

// RegisterAdminMonthSignRoutes 注册全局每日签到奖历。
func RegisterAdminMonthSignRoutes(a *gin.RouterGroup) {
	a.GET("/luxury-sign-rules", AdminLuxurySignRulesHandler)
	a.PUT("/luxury-sign-rules", AdminSaveLuxurySignRulesHandler)
	a.GET("/month-sign-rules", AdminMonthSignRulesHandler)
	a.PUT("/month-sign-rules", AdminSaveMonthSignRulesHandler)
}

// AdminMonthSignRulesHandler 读取全局31档奖历。
// @Summary 每日签到奖历
// @Description 返回31档原签到奖励、双倍VIP及启停。
// @Tags AdminActivities
// @Accept json
// @Produce json
// @Success 200 {object} model.AdminMonthSignRulesResponse
// @Router /admin/api/month-sign-rules [get]
func AdminMonthSignRulesHandler(c *gin.Context) {
	if c.Request.URL.RawQuery != "" {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminMonthSignRules(c.Request.Context(), adminPrincipal(c))
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminSaveMonthSignRulesHandler 校验完整奖历后保存。
// @Summary 保存每日签到奖历
// @Description 修改未来未领取档位；既有领取快照及历史保持。
// @Tags AdminActivities
// @Accept json
// @Produce json
// @Param body body model.AdminMonthSignRulesSaveRequest true "完整31档奖历"
// @Success 200 {object} model.AdminMonthSignRulesResponse
// @Router /admin/api/month-sign-rules [put]
func AdminSaveMonthSignRulesHandler(c *gin.Context) {
	var req model.AdminMonthSignRulesSaveRequest
	if c.Request.URL.RawQuery != "" || c.ShouldBindJSON(&req) != nil || len(req.Version) != 64 || len(req.Rows) != 31 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	seen := map[int64]bool{}
	for _, row := range req.Rows {
		if row.Day < 1 || row.Day > 31 || seen[row.Day] || row.DoubleVIP < 0 || row.DoubleVIP > 12 || len(row.Reward) != 1 {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
		seen[row.Day] = true
		resource := row.Reward[0]
		if resource.ResourceID == "" || len(resource.ResourceID) > 64 || strings.TrimSpace(resource.ResourceID) != resource.ResourceID || resource.Amount < 1 || resource.Amount > 2147483647 {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
	}
	out, err := gameService.AdminSaveMonthSignRules(c.Request.Context(), adminPrincipal(c), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
