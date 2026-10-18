package handler

import (
	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/service"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// RegisterAdminGameContentRoutes binds original achievement/title management.
func RegisterAdminGameContentRoutes(a *gin.RouterGroup) {
	a.GET("/achievements", AdminAchievementsHandler)
	a.POST("/achievements", AdminAchievementCreateHandler)
	a.PUT("/achievements/:id", AdminAchievementUpdateHandler)
	a.DELETE("/achievements/:id", AdminAchievementArchiveHandler)
	a.GET("/titles", AdminTitlesHandler)
	a.PUT("/titles/:id", AdminTitleUpdateHandler)
}

// AdminAchievementsHandler manages original content while preserving acquired history.
// @Summary 游戏成就称号Achievements
// @Description 不恢复已删除的原创奖励；运营修改只影响今后的新合同
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} model.AdminAchievementsResponse
// @Router /admin/api/achievements [get]
func AdminAchievementsHandler(c *gin.Context) {
	out, err := service.AdminAchievements(c.Request.Context(), adminPrincipal(c))
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminAchievementCreateHandler manages original content while preserving acquired history.
// @Summary 游戏成就称号AchievementCreate
// @Description 不恢复已删除的原创奖励；运营修改只影响今后的新合同
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} model.AdminAchievementDTO
// @Router /admin/api/achievements [post]
func AdminAchievementCreateHandler(c *gin.Context) {
	var req model.AdminAchievementDTO
	if c.ShouldBindJSON(&req) != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Source = strings.TrimSpace(req.Source)
	if !utf8.ValidString(req.Name) || utf8.RuneCountInString(req.Name) < 1 || utf8.RuneCountInString(req.Name) > 80 || req.Source == "" || len(req.Source) > 4000 || req.Target < 1 || len(req.Rewards) > 100 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	known := false
	for _, kind := range service.AdminAchievementKinds() {
		if kind.ID == req.Kind {
			known = true
			if (req.Enabled && !kind.Supported) || (kind.FixedTarget > 0 && req.Target != kind.FixedTarget) {
				adminError(c, constUtil.ErrAdminInput)
				return
			}
		}
	}
	if !known {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if req.Enabled && len(req.Rewards) == 0 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	for _, r := range req.Rewards {
		if r.ResourceID == "" || strings.ContainsAny(r.ResourceID, ",; \r\n") || r.Amount < 1 || r.Amount > math.MaxInt32 {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
	}
	out, err := gameService.AdminSaveAchievement(c.Request.Context(), adminPrincipal(c), c.Param("id"), req, true, false)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminAchievementUpdateHandler manages original content while preserving acquired history.
// @Summary 游戏成就称号AchievementUpdate
// @Description 不恢复已删除的原创奖励；运营修改只影响今后的新合同
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} model.AdminAchievementDTO
// @Router /admin/api/achievements/{id} [put]
func AdminAchievementUpdateHandler(c *gin.Context) {
	var req model.AdminAchievementDTO
	if c.ShouldBindJSON(&req) != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if len(req.Version) != 64 || len(c.Param("id")) > 64 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Source = strings.TrimSpace(req.Source)
	if !utf8.ValidString(req.Name) || utf8.RuneCountInString(req.Name) < 1 || utf8.RuneCountInString(req.Name) > 80 || req.Source == "" || len(req.Source) > 4000 || req.Target < 1 || len(req.Rewards) > 100 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	known := false
	for _, kind := range service.AdminAchievementKinds() {
		if kind.ID == req.Kind {
			known = true
			if (req.Enabled && !kind.Supported) || (kind.FixedTarget > 0 && req.Target != kind.FixedTarget) {
				adminError(c, constUtil.ErrAdminInput)
				return
			}
		}
	}
	if !known {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if req.Enabled && len(req.Rewards) == 0 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	for _, r := range req.Rewards {
		if r.ResourceID == "" || strings.ContainsAny(r.ResourceID, ",; \r\n") || r.Amount < 1 || r.Amount > math.MaxInt32 {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
	}
	out, err := gameService.AdminSaveAchievement(c.Request.Context(), adminPrincipal(c), c.Param("id"), req, false, false)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminAchievementArchiveHandler manages original content while preserving acquired history.
// @Summary 游戏成就称号AchievementArchive
// @Description 不恢复已删除的原创奖励；运营修改只影响今后的新合同
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} model.AdminAchievementDTO
// @Router /admin/api/achievements/{id} [delete]
func AdminAchievementArchiveHandler(c *gin.Context) {
	var req model.AdminAchievementDTO
	if c.ShouldBindJSON(&req) != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if len(req.Version) != 64 || len(c.Param("id")) > 64 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminSaveAchievement(c.Request.Context(), adminPrincipal(c), c.Param("id"), req, false, true)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminTitlesHandler manages original content while preserving acquired history.
// @Summary 游戏成就称号Titles
// @Description 不恢复已删除的原创奖励；运营修改只影响今后的新合同
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} model.AdminTitlesResponse
// @Router /admin/api/titles [get]
func AdminTitlesHandler(c *gin.Context) {
	out, err := gameService.AdminTitles(c.Request.Context(), adminPrincipal(c))
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminTitleUpdateHandler manages original content while preserving acquired history.
// @Summary 游戏成就称号TitleUpdate
// @Description 不恢复已删除的原创奖励；运营修改只影响今后的新合同
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} model.AdminAchievementDTO
// @Router /admin/api/titles/{id} [put]
func AdminTitleUpdateHandler(c *gin.Context) {
	var req model.AdminTitleDTO
	if c.ShouldBindJSON(&req) != nil || len(req.Version) != 64 || len(c.Param("id")) > 64 || len(req.Param) > 64 || req.EffectTime < 0 || req.EffectTime > 87600 || len(req.Effects) > 11 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if req.Param != "" {
		value, err := strconv.ParseInt(req.Param, 10, 32)
		if err != nil || value < 1 {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
	}
	seen := map[string]bool{}
	for _, e := range req.Effects {
		switch e.Key {
		case "hp", "att", "pdef", "mdef", "cri", "hit", "dodge", "antiCri", "criFactor", "damageAdd", "damageReduce":
		default:
			adminError(c, constUtil.ErrAdminInput)
			return
		}
		if seen[e.Key] || e.Value < 0 || e.Value > math.MaxInt32 {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
		seen[e.Key] = true
	}
	out, err := gameService.AdminSaveTitle(c.Request.Context(), adminPrincipal(c), c.Param("id"), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
