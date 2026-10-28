package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"hyjs-server/model"
	"hyjs-server/routes"
	"hyjs-server/wrapper"
)

func init() {
	routes.RegisterPostRouter("/ggo-field/trade/invite", wrapper.Wrapper(GGOFieldTradeInviteHandler))
	routes.RegisterPostRouter("/ggo-field/trade/current", wrapper.Wrapper(GGOFieldTradeCurrentHandler))
	routes.RegisterPostRouter("/ggo-field/trade/status", wrapper.Wrapper(GGOFieldTradeStatusHandler))
	for _, action := range []string{"accept", "offer", "confirm", "cancel"} {
		routes.RegisterPostRouter("/ggo-field/trade/"+action, wrapper.Wrapper(ggoFieldTradeActionHandler(action)))
	}
}

// @Summary 邀请同线安全区玩家交易芯片
// @Tags GGOField
// @Accept json
// @Produce json
// @Param body body model.GGOFieldTradeInviteRequest true "角色、会话、请求编号和目标"
// @Success 200 {object} model.GGOFieldTradeView
// @Router /ggo-field/trade/invite [post]
func GGOFieldTradeInviteHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.GGOFieldTradeInviteRequest
	if c.ShouldBindJSON(&req) != nil || gameService == nil || !ggoFieldRequestID(req.RequestID) || req.TargetPlayerID < 1 || req.TargetPlayerID == req.PlayerID {
		return model.NewErrParamsError()
	}
	session, err := ggoFieldSession(req.GGOFieldAuth)
	if err != nil {
		return err
	}
	result, err := gameService.GGOFieldTradeInvite(c.Request.Context(), req, session)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}

// @Summary 读取当前芯片交易
// @Tags GGOField
// @Accept json
// @Produce json
// @Param body body model.GGOFieldTradeCurrentRequest true "角色与会话"
// @Success 200 {object} model.GGOFieldTradeView
// @Router /ggo-field/trade/current [post]
func GGOFieldTradeCurrentHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.GGOFieldTradeCurrentRequest
	if c.ShouldBindJSON(&req) != nil || gameService == nil {
		return model.NewErrParamsError()
	}
	session, err := ggoFieldSession(req.GGOFieldAuth)
	if err != nil {
		return err
	}
	result, err := gameService.GGOFieldTradeCurrent(c.Request.Context(), req.GGOFieldAuth, session)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}

// @Summary 恢复指定芯片交易的当前状态或成交回执
// @Tags GGOField
// @Accept json
// @Produce json
// @Param body body model.GGOFieldTradeStatusRequest true "交易编号和双方角色"
// @Success 200 {object} model.GGOFieldTradeView
// @Router /ggo-field/trade/status [post]
func GGOFieldTradeStatusHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.GGOFieldTradeStatusRequest
	if c.ShouldBindJSON(&req) != nil || gameService == nil || !ggoFieldRequestID(req.TradeID) || req.InitiatorID < 1 || req.ReceiverID < 1 || req.InitiatorID == req.ReceiverID || req.PlayerID != req.InitiatorID && req.PlayerID != req.ReceiverID {
		return model.NewErrParamsError()
	}
	session, err := ggoFieldSession(req.GGOFieldAuth)
	if err != nil {
		return err
	}
	result, err := gameService.GGOFieldTradeStatus(c.Request.Context(), req, session)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}

// ggoFieldTradeActionHandler accepts a versioned trade mutation.
func ggoFieldTradeActionHandler(action string) func(*gin.Context) error {
	return func(c *gin.Context) error {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
		var req model.GGOFieldTradeActionRequest
		if c.ShouldBindJSON(&req) != nil || gameService == nil || !ggoFieldRequestID(req.RequestID) || !ggoFieldRequestID(req.TradeID) || req.RequestID == req.TradeID || req.InitiatorID < 1 || req.ReceiverID < 1 || req.InitiatorID == req.ReceiverID || req.PlayerID != req.InitiatorID && req.PlayerID != req.ReceiverID || action != "cancel" && req.Version < 1 || action == "offer" && len(req.Offer) != 6 || action != "offer" && len(req.Offer) != 0 {
			return model.NewErrParamsError()
		}
		session, err := ggoFieldSession(req.GGOFieldAuth)
		if err != nil {
			return err
		}
		result, err := gameService.GGOFieldTradeAction(c.Request.Context(), action, req, session)
		if err != nil {
			return err
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, model.NewBizCodeData(result))
		return nil
	}
}
