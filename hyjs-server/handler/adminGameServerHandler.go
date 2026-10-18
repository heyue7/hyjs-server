package handler

import (
	"context"
	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/service"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// RegisterAdminGameServerRoutes mounts realm CRUD behind existing authentication and CSRF.
func RegisterAdminGameServerRoutes(a *gin.RouterGroup) {
	a.GET("/servers", AdminGameServersHandler)
	a.POST("/servers", AdminGameServerCreateHandler)
	a.PUT("/servers/:id", AdminGameServerUpdateHandler)
}

// adminServerScope accepts an optional validated realm for existing user deep links.
func adminServerScope(c *gin.Context) {
	raw := c.Query("serverId")
	if raw == "" {
		c.Next()
		return
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 1 {
		adminError(c, constUtil.ErrAdminInput)
		c.Abort()
		return
	}
	if err = service.AdminRequireGameServer(c.Request.Context(), id); err != nil {
		adminError(c, err)
		c.Abort()
		return
	}
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), constUtil.AdminServerScopeContextKey, id))
	c.Next()
}

// AdminGameServersHandler lists configured and planned realms.
// @Summary 区服目录
// @Description 返回区服名、开服日期及运行绑定状态，不公开连接凭据
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} model.AdminGameServersResponse
// @Router /admin/api/servers [get]
func AdminGameServersHandler(c *gin.Context) {
	out, err := service.AdminGameServers(c.Request.Context())
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminGameServerCreateHandler adds a disabled planned realm.
// @Summary 新建区服配置
// @Description 新区服默认待部署，不自动创建游戏进程
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminGameServerCreateRequest true "区服名称和开服时间"
// @Success 200 {object} model.AdminGameServerDTO
// @Router /admin/api/servers [post]
func AdminGameServerCreateHandler(c *gin.Context) {
	var req model.AdminGameServerCreateRequest
	if c.ShouldBindJSON(&req) != nil || !utf8.ValidString(req.Name) || utf8.RuneCountInString(req.Name) > 40 || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Name) != req.Name || strings.IndexFunc(req.Name, unicode.IsControl) >= 0 || req.OpenAt < 1 || req.OpenAt > 253402300799000 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := service.AdminCreateGameServer(c.Request.Context(), adminPrincipal(c), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminGameServerUpdateHandler edits metadata and runtime login availability.
// @Summary 更新区服配置
// @Description 版本校验防覆盖，未绑定运行进程的区服不能开放登录
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path int true "区服编号"
// @Param body body model.AdminGameServerUpdateRequest true "版本、名称、日期和开放状态"
// @Success 200 {object} model.AdminGameServerDTO
// @Router /admin/api/servers/{id} [put]
func AdminGameServerUpdateHandler(c *gin.Context) {
	id, ok := adminID(c)
	var req model.AdminGameServerUpdateRequest
	if !ok || id < 1 || c.ShouldBindJSON(&req) != nil || len(req.Version) != 64 || !utf8.ValidString(req.Name) || utf8.RuneCountInString(req.Name) > 40 || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Name) != req.Name || strings.IndexFunc(req.Name, unicode.IsControl) >= 0 || req.OpenAt < 1 || req.OpenAt > 253402300799000 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := service.AdminUpdateGameServer(c.Request.Context(), adminPrincipal(c), id, req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
