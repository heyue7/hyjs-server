package handler

import (
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"
	"hyjs-server/model"
	"hyjs-server/routes"
	"hyjs-server/wrapper"
)

func init() {
	routes.RegisterPostRouter("/shop-refresh/status", wrapper.Wrapper(ShopRefresh75StatusHandler))
}

// ShopRefresh75StatusHandler 查询原试炼商店各菜单的实际刷新时间。
// @Summary 查询试炼商店刷新时间
// @Description 校验当前角色会话及区服归属；日菜单按游戏日0点刷新，红装菜单沿实际已发布周货窗口，不新建活动或重置库存
// @Tags Shop
// @Accept json
// @Produce json
// @Param body body model.ShopRefresh75StatusRequest true "当前角色会话与原商店类型5"
// @Success 200 {object} model.ShopRefresh75StatusResponse
// @Router /shop-refresh/status [post]
func ShopRefresh75StatusHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var req model.ShopRefresh75StatusRequest
	if c.ShouldBindJSON(&req) != nil || req.PlayerID < 1 || req.ShopType != 5 || len(req.SessionHex) != 32 || gameService == nil {
		return model.NewErrParamsError()
	}
	session, err := hex.DecodeString(req.SessionHex)
	if err != nil || len(session) != 16 {
		return model.NewErrParamsError()
	}
	result, err := gameService.ShopRefresh75Status(c.Request.Context(), req.PlayerID, session)
	if err != nil {
		return err
	}
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}
