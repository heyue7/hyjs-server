package handler

import (
	"context"
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/routes"
	"hyjs-server/service"
	"hyjs-server/wrapper"
)

func init() {
	routes.RegisterPostRouter("/local-play/time/state", wrapper.Wrapper(LocalPlayTimeStateHandler))
	// 日期变更仅由已认证的后台玩家管理入口开放。
}

// LocalPlayTimeStateHandler reads the current personal play calendar.
// @Summary 查询个人游玩日期
// @Description 仅已显式开启的本地demo角色工具，支付账单与网络时钟保持真实时间。
// @Tags LocalPlay
// @Accept json
// @Produce json
// @Param body body model.LocalPlayTimeStateRequest true "当前游戏会话"
// @Success 200 {object} model.LocalPlayTimeView
// @Router /local-play/time/state [post]
func LocalPlayTimeStateHandler(c *gin.Context) error {
	var req model.LocalPlayTimeStateRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlayerID <= 1 || len(req.SessionHex) != 32 {
		return model.NewErrParamsError()
	}
	session, err := hex.DecodeString(req.SessionHex)
	if err != nil || len(session) != 16 {
		return model.NewErrParamsError()
	}
	if !testPaymentsEnabled || gameService == nil || !gameService.LocalPlayToolsEnabled {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterDenied, constUtil.LocalPlayDisabledMessage)
	}
	owner, err := service.AuthorizeAccount(c.Request.Context(), req.PlayerID, session)
	if err != nil {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterDenied, constUtil.LocalPlayInvalidMessage)
	}
	ctx := context.WithValue(c.Request.Context(), constUtil.LocalWalletAPIContextKey, true)
	ctx = context.WithValue(ctx, constUtil.LocalWalletSessionContextKey, session)
	ctx, err = gameService.BindLocalPlayContext(ctx, owner)
	if err != nil {
		return err
	}
	view, err := gameService.LocalPlayTimeState(ctx, req.PlayerID)
	if err != nil {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterRejected, err.Error())
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(view))
	return nil
}

// LocalPlayTimeChangeHandler moves the current player's play calendar once.
// @Summary 调整个人游玩日期
// @Description 支持前一天、后一天、恢复当前时间；相同操作编号重试不重复调整。
// @Tags LocalPlay
// @Accept json
// @Produce json
// @Param body body model.LocalPlayTimeChangeRequest true "动作及唯一请求编号"
// @Success 200 {object} model.LocalPlayTimeView
// @Router /local-play/time/change [post]
func LocalPlayTimeChangeHandler(c *gin.Context) error {
	var req model.LocalPlayTimeChangeRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlayerID <= 1 || len(req.SessionHex) != 32 || len(req.RequestID) < 1 || len(req.RequestID) > 128 || (req.Action != "previousDay" && req.Action != "nextDay" && req.Action != "reset") {
		return model.NewErrParamsError()
	}
	for _, char := range req.RequestID {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_' || char == '.') {
			return model.NewErrParamsError()
		}
	}
	session, err := hex.DecodeString(req.SessionHex)
	if err != nil || len(session) != 16 {
		return model.NewErrParamsError()
	}
	if !testPaymentsEnabled || gameService == nil || !gameService.LocalPlayToolsEnabled {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterDenied, constUtil.LocalPlayDisabledMessage)
	}
	owner, err := service.AuthorizeAccount(c.Request.Context(), req.PlayerID, session)
	if err != nil {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterDenied, constUtil.LocalPlayInvalidMessage)
	}
	ctx := context.WithValue(c.Request.Context(), constUtil.LocalWalletAPIContextKey, true)
	ctx = context.WithValue(ctx, constUtil.LocalWalletSessionContextKey, session)
	ctx, err = gameService.BindLocalPlayContext(ctx, owner)
	if err != nil {
		return err
	}
	view, err := gameService.LocalPlayTimeChange(ctx, req.PlayerID, req.Action, req.RequestID)
	if err != nil {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterRejected, err.Error())
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(view))
	return nil
}
