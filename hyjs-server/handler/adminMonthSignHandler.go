package handler

import (
	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/service"
	"net/url"
	"strings"
)

// RegisterAdminMonthSignRoutes 注册按月每日签到奖历。
func RegisterAdminMonthSignRoutes(a *gin.RouterGroup) {
	a.GET("/luxury-sign-rules", AdminLuxurySignRulesHandler)
	a.PUT("/luxury-sign-rules", AdminSaveLuxurySignRulesHandler)
	a.GET("/month-sign-rules", AdminMonthSignRulesHandler)
	a.PUT("/month-sign-rules", AdminSaveMonthSignRulesHandler)
}

// AdminMonthSignRulesHandler 读取所选月31档奖历。
// @Summary 每日签到奖历
// @Description 返回所选自然月31档签到奖励、双倍VIP及启停，缺省为北京时间当前月。
// @Tags AdminActivities
// @Accept json
// @Produce json
// @Param month query string false "自然月YYYY-MM，缺省北京时间当前月"
// @Success 200 {object} model.AdminMonthSignRulesResponse
// @Router /admin/api/month-sign-rules [get]
func AdminMonthSignRulesHandler(c *gin.Context) {
	query, queryErr := url.ParseQuery(c.Request.URL.RawQuery)
	month := query.Get("month")
	if queryErr != nil || len(query) > 1 || (len(query) == 1 && len(query["month"]) != 1) || (month != "" && !service.ValidMonthSignMonth(month)) || (len(query) == 1 && month == "") {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminMonthSignRules(c.Request.Context(), adminPrincipal(c), month)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminSaveMonthSignRulesHandler 校验完整奖历后保存。
// @Summary 保存每日签到奖历
// @Description 保存当前或未来自然月31档奖历；过去月份只读，既有领取快照及历史保持。
// @Tags AdminActivities
// @Accept json
// @Produce json
// @Param body body model.AdminMonthSignRulesSaveRequest true "完整31档奖历"
// @Success 200 {object} model.AdminMonthSignRulesResponse
// @Router /admin/api/month-sign-rules [put]
func AdminSaveMonthSignRulesHandler(c *gin.Context) {
	var req model.AdminMonthSignRulesSaveRequest
	if c.Request.URL.RawQuery != "" || c.ShouldBindJSON(&req) != nil || len(req.Version) != 64 || len(req.Rows) != 31 || (req.Month != "" && !service.ValidMonthSignMonth(req.Month)) {
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
