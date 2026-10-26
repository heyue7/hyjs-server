package handler

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"io"
	"net/http"
	"strings"
)

// RegisterAdminGuildScheduleRoutes attaches to the existing authenticated admin group.
func RegisterAdminGuildScheduleRoutes(a *gin.RouterGroup) {
	a.GET("/guild-schedules", AdminGuildSchedulesHandler)
	a.PUT("/guild-schedules/calendar", AdminGuildCalendarSaveHandler)
	a.POST("/guild-schedules", AdminGuildScheduleCreateHandler)
	a.PUT("/guild-schedules/:day", AdminGuildScheduleSaveHandler)
}
func adminGuildScheduleDecode(c *gin.Context, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil {
		return false
	}
	var extra any
	return decoder.Decode(&extra) == io.EOF
}
func AdminGuildSchedulesHandler(c *gin.Context) {
	out, err := gameService.AdminGuildSchedules(c.Request.Context(), adminPrincipal(c))
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
func AdminGuildScheduleCreateHandler(c *gin.Context) { adminSaveGuildSchedule(c, true) }
func AdminGuildScheduleSaveHandler(c *gin.Context)   { adminSaveGuildSchedule(c, false) }
func adminSaveGuildSchedule(c *gin.Context, create bool) {
	var req model.AdminGuildScheduleSaveRequest
	if !adminGuildScheduleDecode(c, &req) || !create && (req.Day != c.Param("day") || len(req.RowVersion) != 64) || create && req.RowVersion != "" || strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 500 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminSaveGuildSchedule(c.Request.Context(), adminPrincipal(c), req, create)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
func AdminGuildCalendarSaveHandler(c *gin.Context) {
	var req model.AdminGuildCalendarSaveRequest
	if !adminGuildScheduleDecode(c, &req) || len(req.RowVersion) != 64 || strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 500 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminSaveGuildCalendar(c.Request.Context(), adminPrincipal(c), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
