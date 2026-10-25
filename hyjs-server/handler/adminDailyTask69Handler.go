package handler

import (
	"encoding/hex"
	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/service"
)

// RegisterAdminDailyTask69Routes 接入已鉴权和CSRF保护的后台组。
func RegisterAdminDailyTask69Routes(a *gin.RouterGroup) {
	a.GET("/daily-tasks", AdminDailyTask69Handler)
	a.PUT("/daily-tasks", AdminDailyTask69SaveHandler)
}

// AdminDailyTask69Handler 读取每日任务配置。
// @Summary 每日任务奖励配置
// @Description 展示只读条件及可编辑单条奖励
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} model.AdminDailyTask69Response
// @Router /admin/api/daily-tasks [get]
func AdminDailyTask69Handler(c *gin.Context) {
	out, err := gameService.AdminDailyTasks69(c.Request.Context(), adminPrincipal(c))
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminDailyTask69SaveHandler 校验并保存整批奖励。
// @Summary 保存每日任务奖励
// @Description 当天已生成日档保持原奖励，次日或首次创建日档使用新配置
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminDailyTask69SaveRequest true "版本、七项奖励和修改原因"
// @Success 200 {object} model.AdminDailyTask69Response
// @Router /admin/api/daily-tasks [put]
func AdminDailyTask69SaveHandler(c *gin.Context) {
	var req model.AdminDailyTask69SaveRequest
	if c.ShouldBindJSON(&req) != nil || !service.AdminDailyTask69InputValid(req) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if _, err := hex.DecodeString(req.Version); err != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminSaveDailyTasks69(c.Request.Context(), adminPrincipal(c), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
