package handler

import (
	"encoding/hex"
	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"strings"
	"unicode/utf8"
)

// RegisterAdminSproutRoutes 接入全局小芽设置。
func RegisterAdminSproutRoutes(a *gin.RouterGroup) {
	a.GET("/sprout-settings", AdminSproutSettingsHandler)
	a.PUT("/sprout-settings", AdminSproutSettingsSaveHandler)
}

// AdminSproutSettingsHandler 读取全局小芽配置。
// @Summary 小芽设置
// @Description 按角色创建天数配置，全区服共用
// @Tags Admin
// @Produce json
// @Success 200 {object} model.AdminSproutSettingsResponse
// @Router /admin/api/sprout-settings [get]
func AdminSproutSettingsHandler(c *gin.Context) {
	out, err := gameService.AdminSproutSettings(c.Request.Context(), adminPrincipal(c))
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminSproutSettingsSaveHandler 校验并保存小芽配置。
// @Summary 保存小芽设置
// @Description 启用时必须填写创建天数；检查版本并记录审计
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminSproutSettingsSaveRequest true "小芽设置"
// @Success 200 {object} model.AdminSproutSettingsResponse
// @Router /admin/api/sprout-settings [put]
func AdminSproutSettingsSaveHandler(c *gin.Context) {
	var req model.AdminSproutSettingsSaveRequest
	if c.ShouldBindJSON(&req) != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if !validAdminSproutSettings(req) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminSaveSproutSettings(c.Request.Context(), adminPrincipal(c), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
func validAdminSproutSettings(req model.AdminSproutSettingsSaveRequest) bool {
	if len(req.Version) != 64 || req.Reason == "" || utf8.RuneCountInString(req.Reason) > 500 {
		return false
	}
	if _, err := hex.DecodeString(req.Version); err != nil {
		return false
	}
	if req.Enabled && req.Days == nil {
		return false
	}
	return req.Days == nil || *req.Days >= 1 && *req.Days <= constUtil.AdminSproutMaxDays
}
