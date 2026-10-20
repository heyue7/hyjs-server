package handler

import (
	"strings"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/service"
)

// RegisterAdminSecurityEventRoutes 在已认证后台域挂载独立安全事件只读接口。
func RegisterAdminSecurityEventRoutes(a *gin.RouterGroup) {
	a.GET("/security-events", AdminSecurityEventsHandler)
}

// AdminSecurityEventsHandler 查询分钟聚合的安全事件，仅服务端复核后的admin可读。
// @Summary 安全事件
// @Description 仅admin角色可读；按动作、结果和账号筛选，时间与来源由服务端生成
// @Tags Admin
// @Produce json
// @Param page query integer false "页码，从1开始"
// @Param pageSize query integer false "每页数量，1至100"
// @Param action query string false "固定事件短码，最多64字符"
// @Param outcome query string false "结果：空、success、failure、denied、throttled"
// @Param actor query string false "账号或后台用户名，最多64字符"
// @Success 200 {object} model.AdminSecurityEventsResponse
// @Router /admin/api/security-events [get]
func AdminSecurityEventsHandler(c *gin.Context) {
	page, size, ok := adminPage(c)
	req := model.AdminSecurityEventsRequest{
		Page: page, PageSize: size,
		Action:  strings.TrimSpace(c.Query("action")),
		Outcome: strings.ToLower(strings.TrimSpace(c.Query("outcome"))),
		Actor:   strings.TrimSpace(c.Query("actor")),
	}
	if !ok || len([]rune(req.Action)) > constUtil.AdminSecurityEventActionMax ||
		len([]rune(req.Actor)) > constUtil.AdminSecurityEventActorMax ||
		!adminSecurityEventOutcomeQueryValid(req.Outcome) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := service.AdminSecurityEvents(c.Request.Context(), adminPrincipal(c), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// adminSecurityEventOutcomeQueryValid 保持查询枚举与采集枚举一致，空值表示全部结果。
func adminSecurityEventOutcomeQueryValid(outcome string) bool {
	switch outcome {
	case "", constUtil.AdminSecurityEventOutcomeSuccess, constUtil.AdminSecurityEventOutcomeFailure,
		constUtil.AdminSecurityEventOutcomeDenied, constUtil.AdminSecurityEventOutcomeThrottled:
		return true
	default:
		return false
	}
}
