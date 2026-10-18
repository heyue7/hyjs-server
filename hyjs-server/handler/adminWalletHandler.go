package handler

import (
	"context"
	"encoding/hex"
	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/routes"
	"hyjs-server/service"
	"hyjs-server/wrapper"
	"strconv"
	"strings"
)

func init() {
	routes.RegisterPostRouter("/local-wallet/ledger", wrapper.Wrapper(LocalWalletLedgerHandler))
}

// RegisterAdminWalletRoutes 接入后台角色账单、理由与日期管理。
func RegisterAdminWalletRoutes(a *gin.RouterGroup) {
	a.GET("/wallets", AdminWalletsHandler)
	a.GET("/wallets/:id/ledger", AdminWalletLedgerHandler)
	a.GET("/wallet-reasons", AdminWalletReasonsHandler)
	a.PUT("/wallet-reasons/:id", AdminWalletReasonUpdateHandler)
	// 日期入口由独立的全服时间管理提供。
}

// LocalWalletLedgerHandler 返回当前登录角色分页账单及真实时间范围汇总。
// @Summary 查询蓝绿钱包完整账单
// @Description 以原游戏会话校验归属，服务器累计收支，使用真实账务时间。
// @Tags LocalWallet
// @Accept json
// @Produce json
// @Param body body model.LocalWalletLedgerRequest true "查询条件"
// @Success 200 {object} model.LocalWalletView
// @Router /local-wallet/ledger [post]
func LocalWalletLedgerHandler(c *gin.Context) error {
	var req model.LocalWalletLedgerRequest
	if c.ShouldBindJSON(&req) != nil || req.PlayerID <= 1 || len(req.SessionHex) != 32 {
		return model.NewErrParamsError()
	}
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 20
	}
	if req.Page < 1 || req.Page > 1000000 || req.PageSize < 1 || req.PageSize > 100 || req.StartAt < 0 || req.EndAt < 0 || (req.EndAt > 0 && req.EndAt <= req.StartAt) || (req.Kind != "" && req.Kind != "credit" && req.Kind != "payment") {
		return model.NewErrParamsError()
	}
	session, err := hex.DecodeString(req.SessionHex)
	if err != nil || len(session) != 16 {
		return model.NewErrParamsError()
	}
	if !testPaymentsEnabled || gameService == nil || !gameService.P3LocalWalletEnabled {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterDenied, "蓝绿钱包暂未开放")
	}
	if _, err = service.AuthorizeAccount(c.Request.Context(), req.PlayerID, session); err != nil {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterDenied, "登录已失效，请重新登录")
	}
	ctx := context.WithValue(c.Request.Context(), constUtil.LocalWalletAPIContextKey, true)
	ctx = context.WithValue(ctx, constUtil.LocalWalletSessionContextKey, session)
	view, err := gameService.LocalWalletLedger(ctx, req)
	if err != nil {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterRejected, err.Error())
	}
	c.Header("Cache-Control", "no-store")
	adminSuccess(c, view)
	return nil
}

// AdminWalletsHandler 分页展示游戏角色绑定的钱包。
// @Summary 钱包列表
// @Description 不返回登录凭据。
// @Tags Admin
// @Accept json
// @Produce json
// @Param q query string false "角色昵称或ID"
// @Success 200 {object} model.LocalWalletView
// @Router /admin/api/wallets [get]
func AdminWalletsHandler(c *gin.Context) {
	page, size, ok := adminPage(c)
	if !ok || len(c.Query("q")) > 100 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	result, err := service.AdminWallets(c.Request.Context(), strings.TrimSpace(c.Query("q")), page, size)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, result)
}

// AdminWalletLedgerHandler 按角色和真实时间分页查询账单。
// @Summary 角色钱包账单
// @Description 收入支出由服务端累计。
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path integer true "角色ID"
// @Success 200 {object} model.LocalWalletView
// @Router /admin/api/wallets/{id}/ledger [get]
func AdminWalletLedgerHandler(c *gin.Context) {
	id, ok := adminID(c)
	page, size, pageOK := adminPage(c)
	start, end := int64(0), int64(0)
	var err error
	if c.Query("startAt") != "" {
		start, err = strconv.ParseInt(c.Query("startAt"), 10, 64)
		if err != nil {
			ok = false
		}
	}
	if c.Query("endAt") != "" {
		end, err = strconv.ParseInt(c.Query("endAt"), 10, 64)
		if err != nil {
			ok = false
		}
	}
	kind := c.Query("kind")
	if !ok || !pageOK || id < 1 || start < 0 || end < 0 || (end > 0 && end <= start) || (kind != "" && kind != "credit" && kind != "payment") {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	result, err := service.AdminWalletLedger(c.Request.Context(), adminPrincipal(c), id, start, end, kind, page, size)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, gin.H{"balanceCents": result.BalanceCents, "summary": result.Summary, "items": result.Entries, "total": result.Total, "page": result.Page, "pageSize": result.PageSize})
}

// AdminWalletReasonsHandler 返回实际入账情境和相对权重。
// @Summary 入账理由目录
// @Description 现有流水文案保持冻结。
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} model.AdminWalletReasonRequest
// @Router /admin/api/wallet-reasons [get]
func AdminWalletReasonsHandler(c *gin.Context) {
	rows, err := service.AdminWalletReasons(c.Request.Context())
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, gin.H{"items": rows})
}

// AdminWalletReasonUpdateHandler 保存未来加款的金额区间和文案。
// @Summary 编辑入账理由
// @Description 版本冲突时保留原记录，权限由事务再次验证。
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path string true "理由ID"
// @Param body body model.AdminWalletReasonRequest true "未来入账情境"
// @Success 200 {object} map[string]bool
// @Router /admin/api/wallet-reasons/{id} [put]
func AdminWalletReasonUpdateHandler(c *gin.Context) {
	var req model.AdminWalletReasonRequest
	if c.ShouldBindJSON(&req) != nil || len(c.Param("id")) > 64 || len([]rune(req.Label)) > 170 || req.MinCents < 1 || req.MaxCents < req.MinCents || req.MaxCents > constUtil.LocalWalletMaxCreditCents || req.Weight < 1 || req.Weight > 1000000 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if err := service.AdminSaveWalletReason(c.Request.Context(), adminPrincipal(c), c.Param("id"), req); err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, gin.H{"saved": true})
}
