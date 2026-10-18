package handler

import (
	"context"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/routes"
	"hyjs-server/service"
	"hyjs-server/wrapper"
)

func init() {
	routes.RegisterPostRouter("/local-wallet/state", wrapper.Wrapper(LocalWalletStateHandler))
	routes.RegisterPostRouter("/local-wallet/add", wrapper.Wrapper(LocalWalletAddHandler))
	routes.RegisterPostRouter("/local-wallet/pay", wrapper.Wrapper(LocalWalletPayHandler))
}

// LocalWalletStateHandler returns the current balance, recent entries and pending original orders.
// @Summary 查询蓝绿钱包
// @Description 仅dev/test显式启用的钱包及demo账号；使用当前游戏会话，不涉及真实收费。
// @Tags LocalWallet
// @Accept json
// @Produce json
// @Param body body model.LocalWalletStateRequest true "当前角色会话"
// @Success 200 {object} model.LocalWalletView
// @Router /local-wallet/state [post]
func LocalWalletStateHandler(c *gin.Context) error {
	var req model.LocalWalletStateRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlayerID <= 1 || len(req.SessionHex) != 32 {
		return model.NewErrParamsError()
	}
	session, err := hex.DecodeString(req.SessionHex)
	if err != nil || len(session) != 16 {
		return model.NewErrParamsError()
	}
	if !testPaymentsEnabled || gameService == nil || !gameService.P3LocalWalletEnabled {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterDenied, "蓝绿钱包暂未开放")
	}
	owner, err := service.AuthorizeAccount(c.Request.Context(), req.PlayerID, session)
	if err != nil {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterDenied, "登录已失效，请重新登录")
	}
	ctx := context.WithValue(c.Request.Context(), constUtil.LocalWalletAPIContextKey, true)
	ctx = context.WithValue(ctx, constUtil.LocalWalletSessionContextKey, session)
	ctx, err = gameService.BindLocalPlayContext(ctx, owner)
	if err != nil {
		return err
	}
	result, err := gameService.LocalWalletSnapshot(ctx, req.PlayerID)
	if err != nil {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterRejected, err.Error())
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}

// LocalWalletAddHandler records an explicit, idempotent addition of local test funds.
// @Summary 增加蓝绿钱包余额
// @Description 余额以人民币分整数记录，仅用于本地试玩，不发生真实扣款。
// @Tags LocalWallet
// @Accept json
// @Produce json
// @Param body body model.LocalWalletAddRequest true "金额及本次操作唯一编号"
// @Success 200 {object} model.LocalWalletView
// @Router /local-wallet/add [post]
func LocalWalletAddHandler(c *gin.Context) error {
	var req model.LocalWalletAddRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlayerID <= 1 || len(req.SessionHex) != 32 || req.AmountCents <= 0 || req.AmountCents > constUtil.LocalWalletMaxCreditCents || len(req.RequestID) < 1 || len(req.RequestID) > 128 || req.RequestID != strings.TrimSpace(req.RequestID) {
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
	if !testPaymentsEnabled || gameService == nil || !gameService.P3LocalWalletEnabled {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterDenied, "蓝绿钱包暂未开放")
	}
	owner, err := service.AuthorizeAccount(c.Request.Context(), req.PlayerID, session)
	if err != nil {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterDenied, "登录已失效，请重新登录")
	}
	ctx := context.WithValue(c.Request.Context(), constUtil.LocalWalletAPIContextKey, true)
	ctx = context.WithValue(ctx, constUtil.LocalWalletSessionContextKey, session)
	ctx, err = gameService.BindLocalPlayContext(ctx, owner)
	if err != nil {
		return err
	}
	result, err := gameService.LocalWalletAdd(ctx, req.PlayerID, req.RequestID, req.AmountCents)
	if err != nil {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterRejected, err.Error())
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}

// LocalWalletPayHandler pays the selected owned pending order using the frozen original amount.
// @Summary 使用蓝绿钱包支付充值订单
// @Description 余额扣账与原充值奖励、VIP和会员资格在同一事务中保存；重试不重复扣款。
// @Tags LocalWallet
// @Accept json
// @Produce json
// @Param body body model.LocalWalletPayRequest true "当前角色会话及订单编号"
// @Success 200 {object} model.LocalWalletView
// @Router /local-wallet/pay [post]
func LocalWalletPayHandler(c *gin.Context) error {
	var req model.LocalWalletPayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlayerID <= 1 || len(req.SessionHex) != 32 || len(req.OrderID) != 35 || !strings.HasPrefix(req.OrderID, "p3-") {
		return model.NewErrParamsError()
	}
	if _, err := hex.DecodeString(req.OrderID[3:]); err != nil {
		return model.NewErrParamsError()
	}
	session, err := hex.DecodeString(req.SessionHex)
	if err != nil || len(session) != 16 {
		return model.NewErrParamsError()
	}
	if !testPaymentsEnabled || gameService == nil || !gameService.P3LocalWalletEnabled {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterDenied, "蓝绿钱包暂未开放")
	}
	owner, err := service.AuthorizeAccount(c.Request.Context(), req.PlayerID, session)
	if err != nil {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterDenied, "登录已失效，请重新登录")
	}
	ctx := context.WithValue(c.Request.Context(), constUtil.LocalWalletAPIContextKey, true)
	ctx = context.WithValue(ctx, constUtil.LocalWalletSessionContextKey, session)
	ctx, err = gameService.BindLocalPlayContext(ctx, owner)
	if err != nil {
		return err
	}
	result, err := gameService.LocalWalletPay(ctx, req.PlayerID, req.OrderID)
	if err != nil {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterRejected, err.Error())
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}
