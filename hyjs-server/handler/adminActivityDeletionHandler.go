package handler

import (
	"encoding/hex"
	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"strings"
)

// RegisterAdminActivityDeletionRoutes attaches activity template deletion to the protected admin group.
func RegisterAdminActivityDeletionRoutes(a *gin.RouterGroup) {
	a.DELETE("/activity-templates/:id", AdminActivityDefinitionDeleteHandler)
}

// AdminActivityDefinitionDeleteHandler archives future scheduling input and preserves issued history.
// @Summary 删除全局活动模板
// @Description 使用版本保护软删除；已发布排期快照和玩家领取记录保持，不会被初始化重新导入
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path string true "活动模板ID"
// @Param body body model.AdminActivityDefinitionDeleteRequest true "模板版本"
// @Success 200 {object} model.AdminActivityDefinitionDeleteResponse
// @Router /admin/api/activity-templates/{id} [delete]
func AdminActivityDefinitionDeleteHandler(c *gin.Context) {
	var req model.AdminActivityDefinitionDeleteRequest
	id := c.Param("id")
	if id == "" || len(id) > 64 || strings.TrimSpace(id) != id || strings.ContainsAny(id, "/\\\r\n\x00") || c.ShouldBindJSON(&req) != nil || len(req.Version) != 64 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if _, err := hex.DecodeString(req.Version); err != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if err := gameService.AdminDeleteActivityDefinition(c.Request.Context(), adminPrincipal(c), id, req.Version); err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, model.AdminActivityDefinitionDeleteResponse{OK: true, Archived: true})
}
