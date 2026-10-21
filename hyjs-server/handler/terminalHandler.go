package handler

import (
	"context"
	"encoding/hex"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/routes"
	"hyjs-server/service"
	"hyjs-server/wrapper"
	"strings"
	"unicode"
)

func init() {
	routes.RegisterPostRouter("/local-wallet/burst", wrapper.Wrapper(LocalWalletBurstHandler))
	routes.RegisterPostRouter("/local-wallet/seen", wrapper.Wrapper(LocalWalletSeenHandler))
}

// RegisterAdminTerminalRoutes inherits authentication, CSRF, selected-server authorization and auditing.
func RegisterAdminTerminalRoutes(a *gin.RouterGroup) {
	a.GET("/terminal/settings", AdminTerminalSettingsHandler)
	a.PUT("/terminal/settings", AdminTerminalSettingsUpdateHandler)
	a.POST("/terminal/distribute", AdminTerminalDistributeHandler)
	a.GET("/terminal/distributions", AdminTerminalDistributionsHandler)
}

// AdminTerminalSettingsHandler reads selected-server policy.
// @Summary 蓝绿终端设置
// @Description 查询区服设置与版本
// @Tags Admin
// @Accept json
// @Produce json
// @Param serverId query int true "区服"
// @Success 200 {object} model.TerminalSettings
// @Router /admin/api/terminal/settings [get]
func AdminTerminalSettingsHandler(c *gin.Context) {
	if service.AdminSelectedServerID(c.Request.Context()) < 1 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := service.AdminTerminalSettings(c.Request.Context(), adminPrincipal(c))
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminTerminalSettingsUpdateHandler validates and saves complete future policy.
// @Summary 保存蓝绿终端设置
// @Description 乐观锁保存，不补发旧账号奖励
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.TerminalSettings true "完整设置"
// @Success 200 {object} model.TerminalSettings
// @Router /admin/api/terminal/settings [put]
func AdminTerminalSettingsUpdateHandler(c *gin.Context) {
	var req model.TerminalSettings
	max := constUtil.LocalWalletMaxCreditCents / 100
	if service.AdminSelectedServerID(c.Request.Context()) < 1 || c.ShouldBindJSON(&req) != nil || len(req.Version) != 64 || req.SignupPoints < 0 || req.SignupPoints > max || req.BurstCostPoints < 0 || req.BurstCostPoints > max || req.BurstMinPoints < 1 || req.BurstMaxPoints < req.BurstMinPoints || req.BurstMaxPoints > max || len(req.Weekly) > 100 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	ids := map[string]bool{}
	for _, rule := range req.Weekly {
		if len(rule.ID) < 1 || len(rule.ID) > 48 || strings.TrimSpace(rule.ID) != rule.ID || strings.IndexFunc(rule.ID, unicode.IsControl) >= 0 || ids[rule.ID] || rule.Weekday < 1 || rule.Weekday > 7 || rule.Points < 1 || rule.Points > max {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
		ids[rule.ID] = true
	}
	if req.Weekly == nil {
		req.Weekly = []model.TerminalWeeklyRule{}
	}
	out, err := service.AdminSaveTerminalSettings(c.Request.Context(), adminPrincipal(c), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminTerminalDistributeHandler freezes one authorized distribution.
// @Summary 单次投放超频点数
// @Description UUID重试复用冻结批次，账号去重
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.TerminalDistributeRequest true "投放与审计说明"
// @Success 200 {object} model.TerminalDistributeResponse
// @Router /admin/api/terminal/distribute [post]
func AdminTerminalDistributeHandler(c *gin.Context) {
	var req model.TerminalDistributeRequest
	if service.AdminSelectedServerID(c.Request.Context()) < 1 || c.ShouldBindJSON(&req) != nil || req.Points < 1 || req.Points > constUtil.LocalWalletMaxCreditCents/100 || len(req.Version) != 64 || len([]rune(req.Reason)) > 200 || strings.TrimSpace(req.Reason) == "" || strings.IndexFunc(req.Reason, unicode.IsControl) >= 0 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if _, err := uuid.Parse(req.RequestID); err != nil || len(req.RequestID) != 36 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := service.AdminTerminalDistribute(c.Request.Context(), adminPrincipal(c), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminTerminalDistributionsHandler reads durable selected-server batch history.
// @Summary 终端投放记录
// @Description 分页查询真实批次进度
// @Tags Admin
// @Accept json
// @Produce json
// @Param page query int false "页码"
// @Success 200 {object} model.TerminalDistributionsResponse
// @Router /admin/api/terminal/distributions [get]
func AdminTerminalDistributionsHandler(c *gin.Context) {
	page, size, ok := adminPage(c)
	if !ok || service.AdminSelectedServerID(c.Request.Context()) < 1 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := service.AdminTerminalDistributions(c.Request.Context(), adminPrincipal(c), page, size)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// LocalWalletBurstHandler applies the daily account action.
// @Summary Burst Link！
// @Description 真实北京时间每日一次，扣支与收入分列
// @Tags LocalWallet
// @Accept json
// @Produce json
// @Param body body model.LocalWalletBurstRequest true "会话与UUID"
// @Success 200 {object} model.LocalWalletView
// @Router /local-wallet/burst [post]
func LocalWalletBurstHandler(c *gin.Context) error {
	var req model.LocalWalletBurstRequest
	if c.ShouldBindJSON(&req) != nil || req.PlayerID < 1 || len(req.SessionHex) != 32 {
		return model.NewErrParamsError()
	}
	if _, err := uuid.Parse(req.RequestID); err != nil || len(req.RequestID) != 36 {
		return model.NewErrParamsError()
	}
	ctx, err := terminalClientContext(c, req.PlayerID, req.SessionHex)
	if err != nil {
		return err
	}
	out, err := gameService.LocalWalletBurst(ctx, req.PlayerID, req.RequestID)
	if err != nil {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterRejected, err.Error())
	}
	c.Header("Cache-Control", "no-store")
	adminSuccess(c, out)
	return nil
}

// LocalWalletSeenHandler confirms income actually returned to the authenticated account.
// @Summary 确认终端收入已读
// @Description 只推进已返回收入的确认序号
// @Tags LocalWallet
// @Accept json
// @Produce json
// @Param body body model.LocalWalletSeenRequest true "会话与已显示序号"
// @Success 200 {object} model.LocalWalletView
// @Router /local-wallet/seen [post]
func LocalWalletSeenHandler(c *gin.Context) error {
	var req model.LocalWalletSeenRequest
	if c.ShouldBindJSON(&req) != nil || req.PlayerID < 1 || len(req.SessionHex) != 32 || req.SeenRevision < 0 {
		return model.NewErrParamsError()
	}
	ctx, err := terminalClientContext(c, req.PlayerID, req.SessionHex)
	if err != nil {
		return err
	}
	out, err := gameService.LocalWalletSeen(ctx, req.PlayerID, req.SeenRevision)
	if err != nil {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterRejected, err.Error())
	}
	c.Header("Cache-Control", "no-store")
	adminSuccess(c, out)
	return nil
}
func terminalClientContext(c *gin.Context, id int64, raw string) (context.Context, error) {
	session, err := hex.DecodeString(raw)
	if err != nil || len(session) != 16 {
		return nil, model.NewErrParamsError()
	}
	if !testPaymentsEnabled || gameService == nil || !gameService.P3LocalWalletEnabled {
		return nil, model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterDenied, "蓝绿终端暂未开放")
	}
	owner, err := service.AuthorizeAccount(c.Request.Context(), id, session)
	if err != nil {
		return nil, model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterDenied, "登录已失效，请重新登录")
	}
	ctx := context.WithValue(c.Request.Context(), constUtil.LocalWalletAPIContextKey, true)
	ctx = context.WithValue(ctx, constUtil.LocalWalletSessionContextKey, session)
	return gameService.BindLocalPlayContext(ctx, owner)
}
