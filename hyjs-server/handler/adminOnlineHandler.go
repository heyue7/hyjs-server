package handler

import (
	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"strings"
	"unicode/utf8"
)

// RegisterAdminOnlineRoutes 接入已认证后台的只读在线快照。
func RegisterAdminOnlineRoutes(a *gin.RouterGroup) {
	a.GET("/online-players", AdminOnlinePlayersHandler)
}

// AdminOnlinePlayersHandler 查询当前有效连接的区服人数和角色列表。
// @Summary 在线角色快照
// @Description 全区服统计及按区服、昵称或精确ID筛选的在线列表；未接入区服单独标记。
// @Tags Admin
// @Accept json
// @Produce json
// @Param q query string false "昵称或精确角色ID"
// @Param serverId query integer false "区服ID，省略为全部"
// @Param page query integer false "页码，从1开始"
// @Param pageSize query integer false "每页1至100条"
// @Success 200 {object} model.AdminOnlinePlayersResponse
// @Router /admin/api/online-players [get]
func AdminOnlinePlayersHandler(c *gin.Context) {
	page, size, ok := adminPage(c)
	q := strings.TrimSpace(c.Query("q"))
	if !ok || len(c.Query("q")) > 128 || !utf8.ValidString(q) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	for key, values := range c.Request.URL.Query() {
		if len(values) != 1 || (key != "q" && key != "page" && key != "pageSize" && key != "serverId") {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
	}
	out, err := gameService.AdminOnlinePlayers(c.Request.Context(), adminPrincipal(c), q, page, size)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
