package handler

import (
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/service"
)

// RegisterAdminGamePlayerRoutes 在已认证后台Group注册有限游戏角色管理。
func RegisterAdminGamePlayerRoutes(a *gin.RouterGroup) {
	a.GET("/game-players", AdminGamePlayersHandler)
	a.GET("/game-players/:id", AdminGamePlayerHandler)
	a.PUT("/game-players/:id/name", AdminGamePlayerNameHandler)
	a.POST("/game-players/:id/kick", AdminGamePlayerKickHandler)
}

// AdminGamePlayersHandler 按昵称和ID分页检索游戏角色。
// @Summary 游戏角色分页列表
// @Description 返回昵称、等级、余额和进度摘要，不含密码或登录会话
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} model.AdminGamePlayersResponse
// @Router /admin/api/game-players [get]
func AdminGamePlayersHandler(c *gin.Context) {
	page, size, ok := adminPage(c)
	if !ok || len(c.Query("q")) > 128 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := service.AdminGamePlayers(c.Request.Context(), strings.TrimSpace(c.Query("q")), page, size)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminGamePlayerHandler 返回有限角色和库存详情。
// @Summary 游戏角色详情
// @Description 不返回游戏账号密码、会话、后台凭据或完整存档
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} model.AdminGamePlayerDTO
// @Router /admin/api/game-players/{id} [get]
func AdminGamePlayerHandler(c *gin.Context) {
	id, ok := adminID(c)
	if !ok || id < 1 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := service.AdminGamePlayer(c.Request.Context(), id)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminGamePlayerNameHandler 记录有原因且不扣游戏资源的运营改名。
// @Summary 运营修改游戏昵称
// @Description 保留原付费改名次数和旧回执，拒绝覆盖并发进度
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminGamePlayerNameRequest true "版本、昵称及原因"
// @Success 200 {object} model.AdminGamePlayerDTO
// @Router /admin/api/game-players/{id}/name [put]
func AdminGamePlayerNameHandler(c *gin.Context) {
	id, ok := adminID(c)
	var req model.AdminGamePlayerNameRequest
	if !ok || id < 1 || c.ShouldBindJSON(&req) != nil || len(req.Version) != 64 || !utf8.ValidString(req.Name) || req.Name == "" || strings.TrimSpace(req.Name) != req.Name || strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 1000 || strings.IndexFunc(req.Name, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) >= 0 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	width := 0
	for _, unit := range utf16.Encode([]rune(req.Name)) {
		if unit <= 256 {
			width++
		} else {
			width += 2
		}
	}
	if width > 12 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := service.AdminRenameGamePlayer(c.Request.Context(), adminPrincipal(c), id, req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminGamePlayerKickHandler 轮换原登录会话，使旧连接的下一次操作重新登录。
// @Summary 游戏角色踢下线
// @Description 保留游戏战斗、资源、密码和进度
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminGamePlayerKickRequest true "版本及原因"
// @Success 200 {object} model.AdminGamePlayerDTO
// @Router /admin/api/game-players/{id}/kick [post]
func AdminGamePlayerKickHandler(c *gin.Context) {
	id, ok := adminID(c)
	var req model.AdminGamePlayerKickRequest
	if !ok || id < 1 || c.ShouldBindJSON(&req) != nil || len(req.Version) != 64 || strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 1000 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := service.AdminKickGamePlayer(c.Request.Context(), adminPrincipal(c), id, req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
