package handler

import (
	"encoding/hex"
	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"strings"
	"unicode/utf8"
)

// RegisterAdminCopyDropsRoutes registers the fragment policy beside dungeon operations.
func RegisterAdminCopyDropsRoutes(a *gin.RouterGroup) {
	a.GET("/copy-drops", AdminCopyDropsHandler)
	a.PUT("/copy-drops", AdminCopyDropsSaveHandler)
}

// AdminCopyDropsHandler reads the local fragment cadence.
// @Summary 副本碎片保底配置
// @Tags Admin
// @Produce json
// @Success 200 {object} model.AdminCopyDropsResponse
// @Router /admin/api/copy-drops [get]
func AdminCopyDropsHandler(c *gin.Context) {
	out, err := gameService.AdminCopyDrops(c.Request.Context(), adminPrincipal(c))
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminCopyDropsSaveHandler validates and saves the fragment cadence.
// @Summary 保存副本碎片保底
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminCopyDropsSaveRequest true "开关、周期、版本及原因"
// @Success 200 {object} model.AdminCopyDropsResponse
// @Router /admin/api/copy-drops [put]
func AdminCopyDropsSaveHandler(c *gin.Context) {
	var r model.AdminCopyDropsSaveRequest
	if c.ShouldBindJSON(&r) != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	r.Reason = strings.TrimSpace(r.Reason)
	_, err := hex.DecodeString(r.Version)
	if r.Cycle < 1 || r.Cycle > 10000 || len(r.Version) != 64 || err != nil || r.Reason == "" || utf8.RuneCountInString(r.Reason) > 500 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminSaveCopyDrops(c.Request.Context(), adminPrincipal(c), r)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
