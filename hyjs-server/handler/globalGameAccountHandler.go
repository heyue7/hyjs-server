package handler

import (
	"encoding/hex"
	"errors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/routes"
	"hyjs-server/service"
	"hyjs-server/wrapper"
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func init() {
	routes.RegisterPostRouter("/game-account/login", wrapper.Wrapper(GameAccountLoginHandler))
	routes.RegisterPostRouter("/game-account/profile", wrapper.Wrapper(GameAccountProfileHandler))
	routes.RegisterPostRouter("/game-account/nickname", wrapper.Wrapper(GameAccountNicknameHandler))
	routes.RegisterPostRouter("/game-account/password", wrapper.Wrapper(GameAccountPasswordHandler))
	routes.RegisterPostRouter("/game-account/logout", wrapper.Wrapper(GameAccountLogoutHandler))
}

// GameAccountLogoutHandler releases the supplied session without affecting a newer login.
// @Summary 退出 Brain Burst 2039 登录
// @Description 撤销当前区服会话；重复退出成功，不影响后续新登录或角色财产。
// @Tags Game
// @Accept json
// @Produce json
// @Param body body model.GameAccountLogoutRequest true "当前角色会话"
// @Success 200 {object} model.GameAccountLogoutResponse
// @Router /game-account/logout [post]
func GameAccountLogoutHandler(c *gin.Context) error {
	var req model.GameAccountLogoutRequest
	if c.ShouldBindJSON(&req) != nil || !gameAccountCredentialsValid(req.AccountSession, req.PlayerID, req.SessionHex) {
		return model.NewErrParamsError()
	}
	session, err := hex.DecodeString(req.SessionHex)
	if err != nil || (req.PlayerID > 0 && len(session) != 16) {
		return model.NewErrParamsError()
	}
	out, err := service.LogoutGlobalGameAccountSession(c.Request.Context(), req.AccountSession, req.PlayerID, session)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(out))
	return nil
}

// RegisterGlobalGameAccountRoutes uses the authenticated admin group for global user operations.
func RegisterGlobalGameAccountRoutes(a *gin.RouterGroup) {
	a.GET("/game-accounts", AdminGameAccountsHandler)
	a.POST("/game-accounts", AdminGameAccountCreateHandler)
	a.PUT("/game-accounts/:id", AdminGameAccountUpdateHandler)
	a.PUT("/game-accounts/:id/password", AdminGameAccountPasswordHandler)
	a.GET("/game-accounts/:id/wallet", AdminGameAccountWalletHandler)
}
func gameAccountNicknameValid(value string) bool {
	return utf8.ValidString(value) && strings.TrimSpace(value) == value && utf8.RuneCountInString(value) >= 1 && utf8.RuneCountInString(value) <= 24 && strings.IndexFunc(value, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) < 0
}
func gameAccountPasswordValid(value string) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) >= constUtil.GameAccountPasswordMinCharacters && len(value) <= constUtil.GameAccountPasswordMaxBytes
}
func gameAccountUsernameValid(value string) bool {
	return len(value) >= constUtil.GameAccountUsernameMinBytes && len(value) <= constUtil.GameAccountUsernameMaxBytes && strings.IndexFunc(value, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-')
	}) < 0
}

// AdminGameAccountsHandler lists global game identities without password or session fields.
// @Summary 全局用户列表
// @Description 账号跨服共用，角色保留各服归属
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} model.AdminGameAccountsResponse
// @Router /admin/api/game-accounts [get]
func AdminGameAccountsHandler(c *gin.Context) {
	page, size, ok := adminPage(c)
	if !ok || len(c.Query("q")) > 128 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	var accountID int64
	if raw, present := c.GetQuery("accountId"); present {
		var e error
		accountID, e = strconv.ParseInt(raw, 10, 64)
		if e != nil || accountID < 1 {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
	}
	out, err := service.AdminGlobalGameAccounts(c.Request.Context(), adminPrincipal(c), strings.TrimSpace(c.Query("q")), page, size, accountID)
	if err != nil {
		gameAccountAdminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminGameAccountCreateHandler creates a global account with its own nickname.
// @Summary 新增全局用户
// @Description 不自动创建任何区服角色
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminGameAccountCreateRequest true "账号昵称密码及原因"
// @Success 200 {object} model.GameAccountDTO
// @Router /admin/api/game-accounts [post]
func AdminGameAccountCreateHandler(c *gin.Context) {
	var req model.AdminGameAccountCreateRequest
	if c.ShouldBindJSON(&req) != nil || !gameAccountUsernameValid(req.Username) || !gameAccountNicknameValid(req.Nickname) || !gameAccountPasswordValid(req.Password) || req.Password != req.ConfirmPassword || strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 1000 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := service.AdminCreateGlobalGameAccount(c.Request.Context(), adminPrincipal(c), req)
	if err != nil {
		gameAccountAdminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminGameAccountUpdateHandler updates global nickname and account state.
// @Summary 编辑全局用户
// @Description 变更账号状态会使旧会话失效，保留各服存档
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminGameAccountUpdateRequest true "昵称状态版本及原因"
// @Success 200 {object} model.GameAccountDTO
// @Router /admin/api/game-accounts/{id} [put]
func AdminGameAccountUpdateHandler(c *gin.Context) {
	id, ok := adminID(c)
	var req model.AdminGameAccountUpdateRequest
	if !ok || id < 1 || c.ShouldBindJSON(&req) != nil || !gameAccountNicknameValid(req.Nickname) || len(req.Version) != 64 || strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 1000 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := service.AdminUpdateGlobalGameAccount(c.Request.Context(), adminPrincipal(c), id, req)
	if err != nil {
		gameAccountAdminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminGameAccountPasswordHandler resets a password without exposing its current credential.
// @Summary 重置全局用户密码
// @Description 校验后台权限和版本，记录原因并撤销旧会话
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminGameAccountPasswordRequest true "新密码确认版本及原因"
// @Success 200 {object} model.GameAccountDTO
// @Router /admin/api/game-accounts/{id}/password [put]
func AdminGameAccountPasswordHandler(c *gin.Context) {
	id, ok := adminID(c)
	var req model.AdminGameAccountPasswordRequest
	if !ok || id < 1 || c.ShouldBindJSON(&req) != nil || !gameAccountPasswordValid(req.Password) || req.Password != req.ConfirmPassword || len(req.Version) != 64 || strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 1000 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := service.AdminResetGlobalGameAccountPassword(c.Request.Context(), adminPrincipal(c), id, req)
	if err != nil {
		gameAccountAdminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// GameAccountProfileHandler returns global account information from a current original game session.
// @Summary 钱包账号资料
// @Description 角色会话授权，不返回账号密码或摘要
// @Tags Game
// @Accept json
// @Produce json
// @Param body body model.GameAccountProfileRequest true "当前角色会话"
// @Success 200 {object} model.GameAccountDTO
// @Router /game-account/profile [post]
func GameAccountProfileHandler(c *gin.Context) error {
	var req model.GameAccountProfileRequest
	if c.ShouldBindJSON(&req) != nil || !gameAccountCredentialsValid(req.AccountSession, req.PlayerID, req.SessionHex) {
		return model.NewErrParamsError()
	}
	session, err := hex.DecodeString(req.SessionHex)
	if err != nil || (req.PlayerID > 0 && len(session) != 16) {
		return model.NewErrParamsError()
	}
	out, err := service.CurrentGlobalGameAccountSession(c.Request.Context(), req.AccountSession, req.PlayerID, session)
	if err != nil {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterDenied, err.Error())
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(out))
	return nil
}

// GameAccountNicknameHandler changes only the account-level nickname.
// @Summary 修改钱包账号昵称
// @Description 不修改区服角色名
// @Tags Game
// @Accept json
// @Produce json
// @Param body body model.GameAccountNicknameRequest true "角色会话及昵称"
// @Success 200 {object} model.GameAccountDTO
// @Router /game-account/nickname [post]
func GameAccountNicknameHandler(c *gin.Context) error {
	var req model.GameAccountNicknameRequest
	if c.ShouldBindJSON(&req) != nil || !gameAccountCredentialsValid(req.AccountSession, req.PlayerID, req.SessionHex) || !gameAccountNicknameValid(req.Nickname) {
		return model.NewErrParamsError()
	}
	session, err := hex.DecodeString(req.SessionHex)
	if err != nil || (req.PlayerID > 0 && len(session) != 16) {
		return model.NewErrParamsError()
	}
	out, err := service.UpdateGlobalGameAccountSession(c.Request.Context(), req.AccountSession, req.PlayerID, session, req.Nickname, "", "")
	if err != nil {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterRejected, err.Error())
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(out))
	return nil
}

// GameAccountPasswordHandler verifies the old credential and keeps this session while revoking older sessions in other realms.
// @Summary 修改钱包账号密码
// @Description 原密码验证成功后修改全局密码，当前角色可继续使用
// @Tags Game
// @Accept json
// @Produce json
// @Param body body model.GameAccountPasswordRequest true "角色会话及原新密码"
// @Success 200 {object} model.GameAccountDTO
// @Router /game-account/password [post]
func GameAccountPasswordHandler(c *gin.Context) error {
	var req model.GameAccountPasswordRequest
	if c.ShouldBindJSON(&req) != nil || !gameAccountCredentialsValid(req.AccountSession, req.PlayerID, req.SessionHex) || len(req.OldPassword) > constUtil.GameAccountPasswordMaxBytes || !gameAccountPasswordValid(req.Password) || req.Password != req.ConfirmPassword {
		return model.NewErrParamsError()
	}
	session, err := hex.DecodeString(req.SessionHex)
	if err != nil || (req.PlayerID > 0 && len(session) != 16) {
		return model.NewErrParamsError()
	}
	out, err := service.UpdateGlobalGameAccountSession(c.Request.Context(), req.AccountSession, req.PlayerID, session, "", req.OldPassword, req.Password)
	if err != nil {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterRejected, err.Error())
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(out))
	return nil
}

// AdminGameAccountWalletHandler reads an account-wide ledger independently from a selected role or realm.
// @Summary 全局用户钱包账单
// @Description 按全局账号分页读取原入账出账历史，受保护账号拒绝访问
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} model.LocalWalletView
// @Router /admin/api/game-accounts/{id}/wallet [get]
func AdminGameAccountWalletHandler(c *gin.Context) {
	id, ok := adminID(c)
	page, size, pageOK := adminPage(c)
	start, end := int64(0), int64(0)
	var err error
	if raw := c.Query("startAt"); raw != "" {
		start, err = strconv.ParseInt(raw, 10, 64)
		if err != nil {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
	}
	if raw := c.Query("endAt"); raw != "" {
		end, err = strconv.ParseInt(raw, 10, 64)
		if err != nil {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
	}
	kind := c.Query("kind")
	if !ok || !pageOK || id < 1 || start < 0 || end < 0 || end > 0 && end <= start || kind != "" && kind != "credit" && kind != "payment" {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := service.AdminAccountWalletLedger(c.Request.Context(), adminPrincipal(c), id, start, end, kind, page, size)
	if err != nil {
		gameAccountAdminError(c, err)
		return
	}
	adminSuccess(c, out)
}

func gameAccountAdminError(c *gin.Context, err error) {
	if errors.Is(err, constUtil.ErrGameAccountExists) {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": constUtil.ErrGameAccountExists.Error(), "data": nil})
		return
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "用户不存在", "data": nil})
		return
	}
	adminError(c, err)
}

func gameAccountCredentialsValid(token string, id int64, session string) bool {
	if token != "" {
		raw, e := hex.DecodeString(token)
		if e != nil || len(raw) != 32 || token != strings.ToLower(token) {
			return false
		}
	}
	if id == 0 && session == "" {
		return token != ""
	}
	raw, e := hex.DecodeString(session)
	return id > 0 && e == nil && len(raw) == 16
}

// GameAccountLoginHandler authenticates an account before selecting a realm.
// @Summary 登录全局账号
// @Description 验证账号密码，返回全局账号会话和角色列表，不创建区服角色。
// @Tags Game
// @Accept json
// @Produce json
// @Param body body model.GameAccountLoginRequest true "账号密码"
// @Success 200 {object} model.GameAccountLoginResponse
// @Router /game-account/login [post]
func GameAccountLoginHandler(c *gin.Context) error {
	var req model.GameAccountLoginRequest
	if c.ShouldBindJSON(&req) != nil || len(req.Username) < 1 || len(req.Username) > 1024 || !utf8.ValidString(req.Password) || utf8.RuneCountInString(req.Password) < 6 || utf8.RuneCountInString(req.Password) > 128 || len(req.Password) > constUtil.PasswordCredentialMaxBytes {
		return model.NewErrParamsError()
	}
	out, err := service.LoginGlobalGameAccount(c.Request.Context(), req.Username, req.Password, c.ClientIP())
	if err != nil {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterDenied, err.Error())
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(out))
	return nil
}
