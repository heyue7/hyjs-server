package handler

import (
	"strings"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
)

// RegisterAdminItemDetailRoutes attaches read-only item facts to the authenticated admin group.
func RegisterAdminItemDetailRoutes(a *gin.RouterGroup) {
	a.GET("/items/:id", AdminItemDetailHandler)
}

// AdminItemDetailHandler returns interpreted facts from original catalogue tables.
// @Summary 查看道具数值与简介
// @Description 展示原表可确定的属性、成长、用途及获得物，并单独展示当前有效料理规则的实际增幅与上限；缺失值不补造，不读取用户指定路径。
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path string true "目录中的精确原道具ID，最多128字节"
// @Success 200 {object} model.AdminItemDetailResponse
// @Router /admin/api/items/{id} [get]
func AdminItemDetailHandler(c *gin.Context) {
	id := c.Param("id")
	if id == "" || len(id) > 128 || strings.Contains(id, "..") || strings.ContainsAny(id, " \t\r\n/\\?#&;\x00") {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminItemDetail(c.Request.Context(), model.AdminItemDetailRequest{ID: id})
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
