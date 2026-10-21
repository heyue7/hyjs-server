package handler

import (
	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
)

// RegisterAdminActivityModeRoutes 在已认证并具CSRF保护的后台Group注册按模式活动操作。
func RegisterAdminActivityModeRoutes(a *gin.RouterGroup) {
	RegisterAdminNativeActivityRoutes(a)
	RegisterAdminPersonalOpening40Routes(a)
	a.GET("/activity-modes", AdminActivityModesHandler)
	a.GET("/activity-instances", AdminActivityInstancesHandler)
	a.POST("/activity-instances", AdminTypedActivityCreateHandler)
	a.PUT("/activity-instances/:id", AdminTypedActivitySaveHandler)
	a.PUT("/activity-instances/:id/state", AdminTypedActivityStateHandler)
	a.DELETE("/activity-instances/:id", AdminTypedActivityDeleteHandler)
}

// AdminActivityModesHandler 返回原UI模板白名单及实际可生效的模式字段。
// @Summary 活动模式与原美术模板
// @Description 返回已实现模式、原prefab、资格枚举和空奖励默认表单
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {array} model.AdminActivityMode
// @Router /admin/api/activity-modes [get]
func AdminActivityModesHandler(c *gin.Context) {
	adminSuccess(c, gin.H{"items": gameService.AdminActivityModes()})
}

// AdminActivityInstancesHandler 读取真实配置的业务表单及冻结状态。
// @Summary 按模式活动目录
// @Description 隐藏已归档入口，保留真实游戏规则与玩家历史
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {array} model.AdminActivityInstance
// @Router /admin/api/activity-instances [get]
func AdminActivityInstancesHandler(c *gin.Context) {
	out, err := gameService.AdminActivityInstances(c.Request.Context())
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, gin.H{"items": out})
}

// AdminTypedActivityCreateHandler 从模式原模板创建独立业务活动，不默认开启空奖励。
// @Summary 创建按模式活动
// @Description 表单字段直接展开，新ID由服务端生成
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminTypedActivityRequest true "业务表单"
// @Success 200 {object} model.AdminActivityInstance
// @Router /admin/api/activity-instances [post]
func AdminTypedActivityCreateHandler(c *gin.Context) {
	var req model.AdminTypedActivityRequest
	if c.ShouldBindJSON(&req) != nil || req.Version != "" || len(req.Mode) > 64 || len(req.TemplateID) > 96 || len([]rune(req.Name)) > 80 || len([]rune(req.Alias)) > 80 || len(req.Source) > 1000 || req.StartAt <= 0 || req.EndAt <= req.StartAt || req.ClaimEndAt < req.EndAt || len(req.Products) > 100 || len(req.Milestones) > 31 || (len(req.Prizes) > 1000 || req.Mode != "king-recruit" && len(req.Prizes) > 9) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if !adminRecruitSettingsValid(req.RecruitSettings) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminSaveTypedActivity(c.Request.Context(), adminPrincipal(c), "", req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminTypedActivitySaveHandler 按版本保存未冻结的业务表单。
// @Summary 保存按模式活动
// @Description 修改旧冻结规则需创建新一期
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminTypedActivityRequest true "业务表单与版本"
// @Success 200 {object} model.AdminActivityInstance
// @Router /admin/api/activity-instances/{id} [put]
func AdminTypedActivitySaveHandler(c *gin.Context) {
	var req model.AdminTypedActivityRequest
	if len(c.Param("id")) > 160 || c.ShouldBindJSON(&req) != nil || len(req.Version) != 64 || len(req.Mode) > 64 || len(req.TemplateID) > 96 || len([]rune(req.Name)) > 80 || len([]rune(req.Alias)) > 80 || len(req.Source) > 1000 || req.StartAt <= 0 || req.EndAt <= req.StartAt || req.ClaimEndAt < req.EndAt || len(req.Products) > 100 || len(req.Milestones) > 31 || (len(req.Prizes) > 1000 || req.Mode != "king-recruit" && len(req.Prizes) > 9) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if !adminRecruitSettingsValid(req.RecruitSettings) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminSaveTypedActivity(c.Request.Context(), adminPrincipal(c), c.Param("id"), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminTypedActivityStateHandler 按版本启停，可显式设定当前转盘。
// @Summary 启停活动
// @Description 旧玩家冻结快照保留，新全局转盘源仅在makeCurrent时切换
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminActivityStateRequest true "开关与版本"
// @Success 200 {object} model.AdminActivityInstance
// @Router /admin/api/activity-instances/{id}/state [put]
func AdminTypedActivityStateHandler(c *gin.Context) {
	var req model.AdminActivityStateRequest
	if len(c.Param("id")) > 160 || c.ShouldBindJSON(&req) != nil || len(req.Version) != 64 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminTypedActivityState(c.Request.Context(), adminPrincipal(c), c.Param("id"), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminTypedActivityDeleteHandler 删除运营入口并保留旧玩家记录。
// @Summary 归档活动
// @Description 关闭后续参与，原冻结历史、回执及领奖数据保留
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminActivityDeleteRequest true "版本"
// @Success 200 {object} map[string]bool
// @Router /admin/api/activity-instances/{id} [delete]
func AdminTypedActivityDeleteHandler(c *gin.Context) {
	var req model.AdminActivityDeleteRequest
	if len(c.Param("id")) > 160 || c.ShouldBindJSON(&req) != nil || len(req.Version) != 64 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if err := gameService.AdminDeleteTypedActivity(c.Request.Context(), adminPrincipal(c), c.Param("id"), req.Version); err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, gin.H{"ok": true, "archived": true})
}
