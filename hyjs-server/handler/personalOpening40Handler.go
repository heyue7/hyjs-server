package handler

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
)

// RegisterAdminPersonalOpening40Routes registers authenticated personal rule administration.
func RegisterAdminPersonalOpening40Routes(a *gin.RouterGroup) {
	a.GET("/personal-opening-activities", AdminPersonalOpening40Handler)
	a.PUT("/personal-opening-activities/:id", AdminSavePersonalOpening40Handler)
}

// AdminPersonalOpening40Handler lists the selected realm's personal opening rules.
// @Summary 个人开服活动列表
// @Description 返回按角色创建日计算的常驻个人活动配置
// @Tags Admin
// @Accept json
// @Produce json
// @Param serverId query int true "区服ID"
// @Success 200 {object} model.AdminPersonalOpening40ListResponse
// @Router /admin/api/personal-opening-activities [get]
func AdminPersonalOpening40Handler(c *gin.Context) {
	server, err := strconv.ParseInt(c.Query("serverId"), 10, 64)
	if err != nil || server < 1 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	items, err := gameService.AdminPersonalOpening40(c.Request.Context(), adminPrincipal(c), server)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, model.AdminPersonalOpening40ListResponse{Items: items})
}

// AdminSavePersonalOpening40Handler updates future individual activity rules.
// @Summary 保存个人开服活动
// @Description 版本校验后保存开关、角色天数及达标奖励，已生成实例保持原快照
// @Tags Admin
// @Accept json
// @Produce json
// @Param serverId query int true "区服ID"
// @Param id path string true "稳定个人活动ID"
// @Param body body model.AdminPersonalOpening40SaveRequest true "个人活动配置"
// @Success 200 {object} model.AdminPersonalOpening40DTO
// @Router /admin/api/personal-opening-activities/{id} [put]
func AdminSavePersonalOpening40Handler(c *gin.Context) {
	server, err := strconv.ParseInt(c.Query("serverId"), 10, 64)
	id := strings.TrimSpace(c.Param("id"))
	var req model.AdminPersonalOpening40SaveRequest
	if err != nil || server < 1 || id == "" || len(id) > 64 || c.ShouldBindJSON(&req) != nil || len(req.Version) != 64 || req.StartDay < 1 || req.StartDay > 366 || req.DurationDays < 1 || req.DurationDays > 366 || req.ClaimDays < 0 || req.ClaimDays > 90 || len(req.Form.Milestones) < 1 || !adminNativeActivityFormValid(req.Form) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	previous := int64(0)
	for _, tier := range req.Form.Milestones {
		if tier.Goal <= previous || len(tier.Rewards) == 0 || len(tier.Options) > 0 {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
		previous = tier.Goal
		for _, resource := range tier.Rewards {
			if resource.Amount < 1 {
				adminError(c, constUtil.ErrAdminInput)
				return
			}
		}
	}
	out, err := gameService.AdminSavePersonalOpening40(c.Request.Context(), adminPrincipal(c), server, id, req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
