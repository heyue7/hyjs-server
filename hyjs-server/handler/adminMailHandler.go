package handler

import (
	"encoding/hex"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"hyjs-server/constUtil"
	"hyjs-server/model"
)

// RegisterAdminMailRoutes inherits the authenticated realm, CSRF and session boundaries.
func RegisterAdminMailRoutes(a *gin.RouterGroup) {
	a.GET("/mail", AdminMailListHandler)
	a.POST("/mail/preview", AdminMailPreviewHandler)
	a.POST("/mail/:id/send", AdminMailSendHandler)
}

// AdminMailListHandler reads committed realm-owned mail receipts.
// @Summary 本服发送邮件批次
// @Description 仅显示顶部所选真实运行区服已提交的原系统邮件批次
// @Tags Admin
// @Accept json
// @Produce json
// @Param serverId query integer true "区服"
// @Param page query integer false "页码"
// @Param pageSize query integer false "每页数量"
// @Success 200 {object} model.AdminMailListResponse
// @Router /admin/api/mail [get]
func AdminMailListHandler(c *gin.Context) {
	page, size, ok := adminPage(c)
	if !ok {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminMailList(c.Request.Context(), adminPrincipal(c), page, size)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminMailPreviewHandler freezes the current audience without creating mailbox messages.
// @Summary 预览本服发送邮件
// @Description 按全部玩家（跨分页）或勾选玩家冻结收件角色，冻结/保护账号排除；预览15分钟有效
// @Tags Admin
// @Accept json
// @Produce json
// @Param serverId query integer true "区服"
// @Param body body model.AdminMailPreviewRequest true "邮件标题正文、收件范围及预览UUID"
// @Success 200 {object} model.AdminMailPreviewResponse
// @Router /admin/api/mail/preview [post]
func AdminMailPreviewHandler(c *gin.Context) {
	var req model.AdminMailPreviewRequest
	if c.ShouldBindJSON(&req) != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	id, err := uuid.Parse(req.RequestID)
	req.Title = strings.TrimSpace(req.Title)
	req.Body = strings.TrimSpace(req.Body)
	req.Reason = strings.TrimSpace(req.Reason)
	req.Audience.Mode = strings.TrimSpace(req.Audience.Mode)
	if err != nil || id == uuid.Nil || req.Title == "" || len(req.Title) > 191 || strings.ContainsAny(req.Title, "\r\n\x00") || req.Body == "" || len(req.Body) > 16384 || strings.ContainsRune(req.Body, '\x00') || req.Reason == "" || len(req.Reason) > 1000 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if !adminAudienceInputValid(req.Audience.Mode, req.Audience.PlayerIDs) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	req.RequestID = id.String()
	out, err := gameService.AdminMailPreview(c.Request.Context(), adminPrincipal(c), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminMailSendHandler explicitly confirms exactly one reviewed immutable mail audience.
// @Summary 确认发送邮件
// @Description 与预览绑定且同事务写入原系统邮箱；不创建领奖中心附件，同UUID重试不会重复发送
// @Tags Admin
// @Accept json
// @Produce json
// @Param serverId query integer true "区服"
// @Param id path string true "预览批次UUID"
// @Param body body model.AdminMailSendRequest true "发送UUID与预览版本"
// @Success 200 {object} model.AdminMailSendResponse
// @Router /admin/api/mail/{id}/send [post]
func AdminMailSendHandler(c *gin.Context) {
	id, idErr := uuid.Parse(c.Param("id"))
	var req model.AdminMailSendRequest
	if idErr != nil || id == uuid.Nil || c.ShouldBindJSON(&req) != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	requestID, requestErr := uuid.Parse(req.RequestID)
	version, versionErr := hex.DecodeString(req.Version)
	if requestErr != nil || requestID == uuid.Nil || versionErr != nil || len(version) != 32 || len(req.Version) != 64 || req.Version != strings.ToLower(req.Version) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	req.RequestID = requestID.String()
	out, err := gameService.AdminMailSend(c.Request.Context(), adminPrincipal(c), id.String(), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
