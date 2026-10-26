package handler

import (
	"encoding/hex"
	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/routes"
	"net/http"
	"strings"
)

func init() {
	routes.RegisterPostRouter("/client-update/status", ClientUpdateStatusHandler)
	routes.RegisterPostRouter("/client-update/ack", ClientUpdateAckHandler)
}

// RegisterAdminClientUpdateRoutes 将更新管理接入既有后台鉴权和CSRF组。
func RegisterAdminClientUpdateRoutes(a *gin.RouterGroup) {
	a.GET("/client-updates", AdminClientUpdatesHandler)
	a.POST("/client-updates/draft", AdminClientUpdateDraftHandler)
	a.POST("/client-updates/action", AdminClientUpdateActionHandler)
	a.GET("/client-updates/history", AdminClientUpdateHistoryHandler)
}

// AdminClientUpdatesHandler 查询当前更新策略。
// @Summary 查询客户端更新
// @Description 返回当前服务连接统计与最近发布记录。
// @Tags Admin
// @Accept json
// @Produce json
// @Param scope query string false "当前区服"
// @Success 200 {object} model.AdminClientUpdatesResponse
// @Router /admin/api/client-updates [get]
func AdminClientUpdatesHandler(c *gin.Context) {
	out, err := gameService.AdminClientUpdates(c.Request.Context(), adminPrincipal(c))
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminClientUpdateDraftHandler 保存未生效草稿。
// @Summary 保存更新草稿
// @Description 校验更新模式与兼容门槛后保存草稿。
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminClientUpdateDraftRequest true "草稿"
// @Success 200 {object} model.AdminClientUpdatesResponse
// @Router /admin/api/client-updates/draft [post]
func AdminClientUpdateDraftHandler(c *gin.Context) {
	var req model.AdminClientUpdateDraftRequest
	if c.ShouldBindJSON(&req) != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	req.VersionName = strings.TrimSpace(req.VersionName)
	req.Title = strings.TrimSpace(req.Title)
	req.Content = strings.TrimSpace(req.Content)
	req.Reason = strings.TrimSpace(req.Reason)
	if req.ID < 0 || req.BuildCode < 1 || req.BuildCode > 2100000000 || req.MinLoginBuild < 0 || req.MinRunBuild < 0 || req.MinRunBuild > req.MinLoginBuild || req.MinLoginBuild > req.BuildCode || req.Version == "" || len(req.Version) > 40 || req.VersionName == "" || len(req.VersionName) > 80 || req.Title == "" || len([]rune(req.Title)) > 80 || req.Content == "" || len(req.Content) > 16000 || req.Reason == "" || len(req.Reason) > 1000 || (req.Mode != "notice" && req.Mode != "next_login" && req.Mode != "maintenance") || req.MaintenanceStartAt < 0 || req.MaintenanceEndAt < 0 || (req.Mode == "maintenance" && (req.MaintenanceStartAt == 0 || req.MaintenanceEndAt <= req.MaintenanceStartAt)) || (req.Mode != "maintenance" && (req.MaintenanceStartAt != 0 || req.MaintenanceEndAt != 0)) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminClientUpdateDraft(c.Request.Context(), adminPrincipal(c), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminClientUpdateActionHandler 幂等提交更新操作。
// @Summary 执行更新操作
// @Description 仅管理员可发布、提醒、撤回和切换维护。
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminClientUpdateActionRequest true "操作"
// @Success 200 {object} model.AdminClientUpdatesResponse
// @Router /admin/api/client-updates/action [post]
func AdminClientUpdateActionHandler(c *gin.Context) {
	var req model.AdminClientUpdateActionRequest
	if c.ShouldBindJSON(&req) != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if (req.ID < 1 && req.Action != "server_stop" && req.Action != "server_start") || req.Version == "" || len(req.Version) > 40 || len(req.RequestID) < 16 || len(req.RequestID) > 80 || strings.ContainsAny(req.RequestID, " /\\\r\n\x00") || req.Reason == "" || len(req.Reason) > 1000 || (req.Action != "publish" && req.Action != "remind" && req.Action != "withdraw" && req.Action != "start_maintenance" && req.Action != "end_maintenance" && req.Action != "server_stop" && req.Action != "server_start") {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminClientUpdateAction(c.Request.Context(), adminPrincipal(c), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminClientUpdateHistoryHandler 查询发布历史。
// @Summary 查询更新历史
// @Description 按页读取发布记录。
// @Tags Admin
// @Accept json
// @Produce json
// @Param page query integer false "页码"
// @Param pageSize query integer false "每页数量"
// @Success 200 {object} model.AdminClientUpdateHistoryResponse
// @Router /admin/api/client-updates/history [get]
func AdminClientUpdateHistoryHandler(c *gin.Context) {
	page, size, ok := adminPage(c)
	if !ok {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminClientUpdateHistory(c.Request.Context(), adminPrincipal(c), page, size)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// ClientUpdateStatusHandler 查询启动进程的更新资格。
// @Summary 查询客户端更新状态
// @Description 校验启动ID和真实会话绑定，返回更新门槛。
// @Tags Game
// @Accept json
// @Produce json
// @Param body body model.ClientUpdateStatusRequest true "客户端状态"
// @Success 200 {object} model.ClientUpdateStatusResponse
// @Router /client-update/status [post]
func ClientUpdateStatusHandler(c *gin.Context) {
	var req model.ClientUpdateStatusRequest
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(400, gin.H{"code": 400, "msg": "参数无效", "data": nil})
		return
	}
	req.StartupID = strings.ToLower(strings.ReplaceAll(req.StartupID, "-", ""))
	_, startupErr := hex.DecodeString(req.StartupID)
	session, sessionErr := hex.DecodeString(req.SessionHex)
	if req.BuildCode < 0 || len(req.StartupID) != 32 || startupErr != nil || req.PlayerID < 0 || ((req.PlayerID > 0 || req.SessionHex != "") && (req.PlayerID == 0 || sessionErr != nil || len(session) != 16)) {
		c.JSON(400, gin.H{"code": 400, "msg": "参数无效", "data": nil})
		return
	}
	out, err := gameService.ClientUpdateStatus(c.Request.Context(), req)
	if err != nil {
		c.JSON(500, gin.H{"code": 500, "msg": "更新状态暂不可用", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "", "data": out})
}

// ClientUpdateAckHandler 保存真实展示与确认回执。
// @Summary 确认客户端更新提示
// @Description 匿名回执不计入已认证玩家送达统计。
// @Tags Game
// @Accept json
// @Produce json
// @Param body body model.ClientUpdateAckRequest true "回执"
// @Success 200 {object} model.ClientUpdateAckResponse
// @Router /client-update/ack [post]
func ClientUpdateAckHandler(c *gin.Context) {
	var req model.ClientUpdateAckRequest
	if c.ShouldBindJSON(&req) != nil {
		c.JSON(400, gin.H{"code": 400, "msg": "参数无效", "data": nil})
		return
	}
	req.StartupID = strings.ToLower(strings.ReplaceAll(req.StartupID, "-", ""))
	_, startupErr := hex.DecodeString(req.StartupID)
	session, sessionErr := hex.DecodeString(req.SessionHex)
	if req.BuildCode < 0 || len(req.StartupID) != 32 || startupErr != nil || req.BatchID < 1 || req.PlayerID < 0 || (req.Kind != "shown" && req.Kind != "acknowledged") || ((req.PlayerID > 0 || req.SessionHex != "") && (req.PlayerID == 0 || sessionErr != nil || len(session) != 16)) {
		c.JSON(400, gin.H{"code": 400, "msg": "参数无效", "data": nil})
		return
	}
	out, err := gameService.ClientUpdateAck(c.Request.Context(), req)
	if err != nil {
		c.JSON(409, gin.H{"code": 409, "msg": "回执资格已变化，请刷新", "data": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "", "data": out})
}
