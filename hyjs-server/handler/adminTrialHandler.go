package handler

import (
	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"strings"
)

// RegisterAdminTrialRoutes 复用后台认证、区服范围及CSRF保护。
func RegisterAdminTrialRoutes(a *gin.RouterGroup) {
	a.GET("/trials/config", AdminTrialConfigHandler)
	a.POST("/trials/preview", AdminTrialPreviewHandler)
	a.PUT("/trials/config", AdminTrialConfigSaveHandler)
}

// AdminTrialConfigHandler 读取试炼当前发布状态及生成参数。
// @Summary 试炼之地配置
// @Description 返回实际已发布层数、来源、版本及Re102层生成参数。
// @Tags AdminTrials
// @Accept json
// @Produce json
// @Success 200 {object} model.AdminTrialConfigResponse
// @Router /admin/api/trials/config [get]
func AdminTrialConfigHandler(c *gin.Context) {
	if c.Request.URL.RawQuery != "" {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminTrialConfig(c.Request.Context(), adminPrincipal(c))
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminTrialPreviewHandler 返回指定楼层或全目录三难度实际生成预览。
// @Summary 预览试炼生成结果
// @Description 只读生成预览，floor=0返回全102层；不修改游戏目录。
// @Tags AdminTrials
// @Accept json
// @Produce json
// @Param body body model.AdminTrialPreviewRequest true "生成参数与预览楼层"
// @Success 200 {object} model.AdminTrialPreviewResponse
// @Router /admin/api/trials/preview [post]
func AdminTrialPreviewHandler(c *gin.Context) {
	var req model.AdminTrialPreviewRequest
	if c.ShouldBindJSON(&req) != nil || req.Floor < 0 || req.Floor > constUtil.P7ReRushLastFloor || gameService.ValidateAdminTrialGeneration(req.Config) != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminTrialPreview(c.Request.Context(), adminPrincipal(c), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminTrialConfigSaveHandler 按版本原子发布试炼生成目录。
// @Summary 保存并发布试炼配置
// @Description 保存后下一场使用新目录；进行中战斗冻结快照、进度和已选加成保留。只管理普通试炼，精英目录不变。
// @Tags AdminTrials
// @Accept json
// @Produce json
// @Param body body model.AdminTrialSaveRequest true "完整参数、读取版本及修改原因"
// @Success 200 {object} model.AdminTrialConfigResponse
// @Router /admin/api/trials/config [put]
func AdminTrialConfigSaveHandler(c *gin.Context) {
	var req model.AdminTrialSaveRequest
	if c.ShouldBindJSON(&req) != nil || req.ExpectedVersion < 0 || strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 1000 || gameService.ValidateAdminTrialGeneration(req.Config) != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminSaveTrialConfig(c.Request.Context(), adminPrincipal(c), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
