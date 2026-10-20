package handler

import (
	"encoding/hex"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"hyjs-server/constUtil"
	"hyjs-server/model"
)

// RegisterAdminBroadcastRoutes inherits the authenticated realm, CSRF and session boundaries.
func RegisterAdminBroadcastRoutes(a *gin.RouterGroup) {
	a.GET("/broadcast", AdminBroadcastListHandler)
	a.POST("/broadcast/preview", AdminBroadcastPreviewHandler)
	a.POST("/broadcast/:id/send", AdminBroadcastSendHandler)
}

// AdminBroadcastListHandler reads committed realm-owned compensation receipts.
// @Summary 本服群发补偿批次
// @Description 仅显示顶部所选真实运行区服已提交到原领奖中心的补偿批次
// @Tags Admin
// @Accept json
// @Produce json
// @Param serverId query integer true "区服"
// @Param page query integer false "页码"
// @Param pageSize query integer false "每页数量"
// @Success 200 {object} model.AdminBroadcastListResponse
// @Router /admin/api/broadcast [get]
func AdminBroadcastListHandler(c *gin.Context) {
	page, size, ok := adminPage(c)
	if !ok {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminBroadcastList(c.Request.Context(), adminPrincipal(c), page, size)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminBroadcastPreviewHandler freezes the current audience without delivering attachments.
// @Summary 预览本服群发补偿
// @Description 按全部玩家（跨分页）或勾选玩家冻结收件角色和领奖中心附件，冻结/保护账号排除；预览15分钟有效
// @Tags Admin
// @Accept json
// @Produce json
// @Param serverId query integer true "区服"
// @Param body body model.AdminBroadcastPreviewRequest true "补偿标题正文附件、收件范围及预览UUID"
// @Success 200 {object} model.AdminBroadcastPreviewResponse
// @Router /admin/api/broadcast/preview [post]
func AdminBroadcastPreviewHandler(c *gin.Context) {
	var req model.AdminBroadcastPreviewRequest
	if c.ShouldBindJSON(&req) != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	id, err := uuid.Parse(req.RequestID)
	req.Title = strings.TrimSpace(req.Title)
	req.Body = strings.TrimSpace(req.Body)
	req.Reason = strings.TrimSpace(req.Reason)
	req.Audience.Mode = strings.TrimSpace(req.Audience.Mode)
	if err != nil || id == uuid.Nil || req.Title == "" || len(req.Title) > 191 || strings.ContainsAny(req.Title, "\r\n\x00") || req.Body == "" || len(req.Body) > 16384 || strings.ContainsRune(req.Body, '\x00') || req.Reason == "" || len(req.Reason) > 1000 || len(req.Rewards) < 1 || len(req.Rewards) > 100 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if !adminAudienceInputValid(req.Audience.Mode, req.Audience.PlayerIDs) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	req.RequestID = id.String()
	seen := map[string]bool{}
	for _, reward := range req.Rewards {
		if reward.ResourceID == "" || len(reward.ResourceID) > 128 || strings.ContainsAny(reward.ResourceID, ",; \r\n\x00") || reward.Amount < 1 || reward.Amount > 2147483647 || seen[reward.ResourceID] {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
		seen[reward.ResourceID] = true
	}
	out, err := gameService.AdminBroadcastPreview(c.Request.Context(), adminPrincipal(c), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminBroadcastSendHandler explicitly confirms exactly one reviewed immutable audience.
// @Summary 确认群发补偿
// @Description 与预览绑定且同事务写入原领奖中心待领附件；不生成系统邮件，同UUID重试不会重复发放
// @Tags Admin
// @Accept json
// @Produce json
// @Param serverId query integer true "区服"
// @Param id path string true "预览批次UUID"
// @Param body body model.AdminBroadcastSendRequest true "发送UUID与预览版本"
// @Success 200 {object} model.AdminBroadcastSendResponse
// @Router /admin/api/broadcast/{id}/send [post]
func AdminBroadcastSendHandler(c *gin.Context) {
	id, idErr := uuid.Parse(c.Param("id"))
	var req model.AdminBroadcastSendRequest
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
	out, err := gameService.AdminBroadcastSend(c.Request.Context(), adminPrincipal(c), id.String(), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// adminAudienceInputValid validates the shared all-or-explicit player selection contract.
func adminAudienceInputValid(mode string, playerIDs []int64) bool {
	switch mode {
	case constUtil.AdminAudienceModeAll:
		return len(playerIDs) == 0
	case constUtil.AdminAudienceModePlayers:
		if len(playerIDs) < 1 || len(playerIDs) > constUtil.AdminAudienceMaximumPlayers {
			return false
		}
		seen := make(map[int64]bool, len(playerIDs))
		for _, id := range playerIDs {
			if id < 1 || seen[id] {
				return false
			}
			seen[id] = true
		}
		return true
	default:
		return false
	}
}
