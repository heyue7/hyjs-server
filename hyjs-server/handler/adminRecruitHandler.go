package handler

import (
	"strings"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
)

// RegisterAdminRecruitRoutes 接入两项全局招募运行规则，沿用已认证与CSRF保护的后台分组。
func RegisterAdminRecruitRoutes(a *gin.RouterGroup) {
	a.GET("/recruit-settings", AdminRecruitSettingsHandler)
	a.PUT("/recruit-settings", AdminRecruitSettingsSaveHandler)
}

// AdminRecruitSettingsHandler 返回强者与王者招募当前每日次数和保底间隔。
// @Summary 招募运行规则
// @Description 返回全局强者招募（costType2）与王者招募（costType3）的每日次数、保底间隔及乐观版本；不返回账号凭据。
// @Tags AdminActivities
// @Accept json
// @Produce json
// @Success 200 {object} model.AdminRecruitSettingsResponse
// @Router /admin/api/recruit-settings [get]
func AdminRecruitSettingsHandler(c *gin.Context) {
	if c.Request.URL.RawQuery != "" {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminRecruitSettings(c.Request.Context(), adminPrincipal(c))
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminRecruitSettingsSaveHandler 保存两项完整且唯一的全局招募运行规则。
// @Summary 保存招募运行规则
// @Description 按读取版本保存强者、王者两项规则；王者每日1至100000，强者每日0表示不限，保底1至10000。不改原奖励、价格、免费冷却和历史进度。
// @Tags AdminActivities
// @Accept json
// @Produce json
// @Param body body model.AdminRecruitSettingsSaveRequest true "两项招募运行规则"
// @Success 200 {object} model.AdminRecruitSettingsResponse
// @Router /admin/api/recruit-settings [put]
func AdminRecruitSettingsSaveHandler(c *gin.Context) {
	var req model.AdminRecruitSettingsSaveRequest
	if c.ShouldBindJSON(&req) != nil || len(req.Version) != 64 || strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 1000 || len(req.Items) != 2 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	seen := map[int64]bool{}
	for _, item := range req.Items {
		if seen[item.Kind] || item.Name != constUtil.RecruitRuntimeName(int32(item.Kind)) || item.GuaranteeEvery < int64(constUtil.RecruitRuntimeGuaranteeMin) || item.GuaranteeEvery > int64(constUtil.RecruitRuntimeGuaranteeMax) {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
		switch item.Kind {
		case int64(constUtil.RecruitRuntimeKindStrong):
			if item.DailyLimit < int64(constUtil.RecruitRuntimeDailyUnlimited) || item.DailyLimit > int64(constUtil.RecruitRuntimeDailyMaximum) {
				adminError(c, constUtil.ErrAdminInput)
				return
			}
		case int64(constUtil.RecruitRuntimeKindKing):
			if item.DailyLimit < 1 || item.DailyLimit > int64(constUtil.RecruitRuntimeDailyMaximum) {
				adminError(c, constUtil.ErrAdminInput)
				return
			}
		default:
			adminError(c, constUtil.ErrAdminInput)
			return
		}
		seen[item.Kind] = true
	}
	if !seen[int64(constUtil.RecruitRuntimeKindStrong)] || !seen[int64(constUtil.RecruitRuntimeKindKing)] {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminSaveRecruitSettings(c.Request.Context(), adminPrincipal(c), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
