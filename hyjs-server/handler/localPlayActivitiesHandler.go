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
	routes.RegisterPostRouter("/local-play/activities/state", wrapper.Wrapper(LocalPlayActivitiesStateHandler))
	routes.RegisterPostRouter("/local-play/activities/apply", wrapper.Wrapper(LocalPlayActivitiesApplyHandler))
}

// LocalPlayActivitiesStateHandler reads the current player's private activity directory.
// @Summary 查询本地活动工具目录
// @Description 显示每项活动的开放、刷新能力及未实现状态，使用当前原游戏会话。
// @Tags LocalPlay
// @Accept json
// @Produce json
// @Param body body model.LocalPlayActivitiesStateRequest true "当前游戏会话"
// @Success 200 {object} model.LocalPlayActivitiesView
// @Router /local-play/activities/state [post]
func LocalPlayActivitiesStateHandler(c *gin.Context) error {
	var req model.LocalPlayActivitiesStateRequest
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
	view, err := gameService.LocalPlayActivitiesSnapshot(ctx, req.PlayerID)
	if err != nil {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterRejected, err.Error())
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(view))
	return nil
}

// LocalPlayActivitiesApplyHandler opens or refreshes selected private activity cycles.
// @Summary 开启或刷新本地活动
// @Description 新一期独立保存，历史回执和付费资格保持；批量操作返回逐项结果。
// @Tags LocalPlay
// @Accept json
// @Produce json
// @Param body body model.LocalPlayActivitiesApplyRequest true "动作及唯一操作编号"
// @Success 200 {object} model.LocalPlayActivitiesView
// @Router /local-play/activities/apply [post]
func LocalPlayActivitiesApplyHandler(c *gin.Context) error {
	var req model.LocalPlayActivitiesApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlayerID <= 1 || len(req.SessionHex) != 32 || len(req.ActivityID) < 1 || len(req.ActivityID) > 64 || len(req.RequestID) < 1 || len(req.RequestID) > 128 || (req.Action != "open" && req.Action != "refresh") {
		return model.NewErrParamsError()
	}
	for _, value := range []string{req.ActivityID, req.RequestID} {
		for _, char := range value {
			if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_' || char == '.') {
				return model.NewErrParamsError()
			}
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
	view, err := gameService.LocalPlayActivitiesApply(ctx, req.PlayerID, req.Action, req.ActivityID, req.RequestID)
	if err != nil {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterRejected, err.Error())
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(view))
	return nil
}
