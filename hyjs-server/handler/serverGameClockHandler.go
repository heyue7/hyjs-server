package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"hyjs-server/constUtil"
	"hyjs-server/model"
)

// RegisterAdminServerGameClockRoutes exposes the calendar as an independent server operation.
func RegisterAdminServerGameClockRoutes(a *gin.RouterGroup) {
	a.GET("/server-clock", AdminServerGameClockHandler)
	a.POST("/server-clock", AdminServerGameClockChangeHandler)
}

// AdminServerGameClockHandler reads the complete server calendar.
// @Summary 全服游戏时间
// @Description 所有游戏账号共用此时间，真实账务和登录有效期独立。
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} model.ServerGameClockView
// @Router /admin/api/server-clock [get]
func AdminServerGameClockHandler(c *gin.Context) {
	out, err := gameService.AdminServerGameClock(c.Request.Context(), adminPrincipal(c))
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminServerGameClockChangeHandler changes the server date by one day with revision and idempotency guards.
// @Summary 全服游戏日期前后一天
// @Description 仅管理员操作；活动战斗未结束时拒绝，保留历史领取与恢复高水位。
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.ServerGameClockChangeRequest true "全服日期调整"
// @Success 200 {object} model.ServerGameClockView
// @Router /admin/api/server-clock [post]
func AdminServerGameClockChangeHandler(c *gin.Context) {
	var req model.ServerGameClockChangeRequest
	if c.ShouldBindJSON(&req) != nil || req.Revision < 0 || (req.DeltaDays != -1 && req.DeltaDays != 1) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	parsed, err := uuid.Parse(req.RequestID)
	if err != nil || parsed.String() != req.RequestID {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminChangeServerGameClock(c.Request.Context(), adminPrincipal(c), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
