package handler

import (
	"github.com/gin-gonic/gin"
	"hyjs-server/model"
	"hyjs-server/routes"
	"hyjs-server/wrapper"
	"net/http"
)

func init() {
	routes.RegisterPostRouter("/ggo-field/boss/active", wrapper.Wrapper(GGOFieldBossActiveHandler))
	routes.RegisterPostRouter("/ggo-field/boss/receipt", wrapper.Wrapper(GGOFieldBossReceiptHandler))
}

// GGOFieldBossActiveHandler identifies the accepted native battle after reconnect.
// @Summary 查询沙漠首领进行中战斗
// @Tags GGOField
// @Accept json
// @Produce json
// @Param body body model.GGOFieldBossActiveRequest true "角色和会话"
// @Success 200 {object} model.GGOFieldBossActiveResponse
// @Router /ggo-field/boss/active [post]
func GGOFieldBossActiveHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var request model.GGOFieldBossActiveRequest
	if c.ShouldBindJSON(&request) != nil || gameService == nil {
		return model.NewErrParamsError()
	}
	session, err := ggoFieldSession(request.GGOFieldAuth)
	if err != nil {
		return err
	}
	result, err := gameService.GGOFieldBossActive(c.Request.Context(), request.GGOFieldAuth, session)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}

// GGOFieldBossReceiptHandler reads an immutable native end reply without awarding again.
// @Summary 查询沙漠首领战斗回执
// @Tags GGOField
// @Accept json
// @Produce json
// @Param body body model.GGOFieldBossReceiptRequest true "角色、会话和战斗编号"
// @Success 200 {object} model.GGOFieldBossBattleReceipt
// @Router /ggo-field/boss/receipt [post]
func GGOFieldBossReceiptHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var request model.GGOFieldBossReceiptRequest
	if c.ShouldBindJSON(&request) != nil || gameService == nil || !ggoMonsterFightID(request.FightID) {
		return model.NewErrParamsError()
	}
	session, err := ggoFieldSession(request.GGOFieldAuth)
	if err != nil {
		return err
	}
	result, err := gameService.GGOFieldBossReceipt(c.Request.Context(), request, session)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}
