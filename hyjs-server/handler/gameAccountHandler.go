package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/routes"
	"hyjs-server/service"
	"hyjs-server/utils"
	"hyjs-server/wrapper"
)

func init() {
	routes.RegisterPostRouter("/game-account/register", wrapper.Wrapper(GameAccountRegisterHandler))
}

// RegisterAdminAccountCreationRoutes adds original game account creation to the authenticated admin group.
func RegisterAdminAccountCreationRoutes(a *gin.RouterGroup) {
	a.POST("/game-players", AdminGamePlayerCreateHandler)
	RegisterGlobalGameAccountRoutes(a)
}

// GameAccountRegisterHandler strictly registers an account before the original game login/new-player flow.
// @Summary 注册游戏账号
// @Description 重复账号明确拒绝；密码摘要沿用原账号体系，不返回凭据或登录会话。
// @Tags Game
// @Accept json
// @Produce json
// @Param body body model.GameAccountRegisterRequest true "账号密码与确认密码"
// @Success 200 {object} model.GameAccountRegisterResponse
// @Router /game-account/register [post]
func GameAccountRegisterHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, constUtil.GameAccountRegisterBodyBytes)
	host := c.ClientIP()
	if !service.GameAccountRegisterAllowed(host, time.Now()) {
		c.Header("Retry-After", "60")
		c.JSON(http.StatusTooManyRequests, gin.H{"code": 429, "msg": constUtil.ErrGameAccountRegisterLimited.Error(), "data": nil})
		return nil
	}
	var req model.GameAccountRegisterRequest
	if c.ShouldBindJSON(&req) != nil || !gameAccountNicknameValid(req.Nickname) || len(req.Username) < constUtil.GameAccountUsernameMinBytes || len(req.Username) > constUtil.GameAccountUsernameMaxBytes || utf8.RuneCountInString(req.Password) < constUtil.GameAccountPasswordMinCharacters || len(req.Password) > constUtil.GameAccountPasswordMaxBytes || !utf8.ValidString(req.Password) || req.Password != req.ConfirmPassword || strings.IndexFunc(req.Username, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-')
	}) >= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "昵称需1–24个字符；账号需3–32位字母、数字、下划线或短横线；密码需至少6个字符、最多72字节且两次一致", "data": nil})
		return nil
	}
	ctx := c.Request.Context()
	if gameService != nil {
		ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, gameService)
	}
	out, err := service.RegisterGameAccount(ctx, req.Username, req.Password, req.Nickname)
	if err != nil {
		if errors.Is(err, utils.ErrPasswordWorkBusy) {
			c.Header("Retry-After", "2")
			c.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "msg": "注册服务繁忙，请稍后重试", "data": nil})
			return nil
		}
		if errors.Is(err, constUtil.ErrGameAccountExists) {
			c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": err.Error(), "data": nil})
			return nil
		}
		return err
	}
	c.Header("Cache-Control", "no-store")
	adminSuccess(c, out)
	return nil
}

// AdminGamePlayerCreateHandler creates an account and its original level-one starter character.
// @Summary 新增游戏用户
// @Description 账号和初始角色同事务创建，不接受VIP、货币或额外背包投放。
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminGamePlayerCreateRequest true "账号、密码、原初始角色及原因"
// @Success 200 {object} model.AdminGamePlayerDTO
// @Router /admin/api/game-players [post]
func AdminGamePlayerCreateHandler(c *gin.Context) {
	var req model.AdminGamePlayerCreateRequest
	if c.ShouldBindJSON(&req) != nil || req.AccountID < 0 || (req.AccountID == 0 && (!gameAccountUsernameValid(req.Username) || !gameAccountPasswordValid(req.Password))) || !utf8.ValidString(req.Name) || req.Name == "" || strings.TrimSpace(req.Name) != req.Name || strings.IndexFunc(req.Name, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) >= 0 || strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 1000 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	width := 0
	for _, unit := range utf16.Encode([]rune(req.Name)) {
		if unit <= 256 {
			width++
		} else {
			width += 2
		}
	}
	if width > 12 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if req.HeroID == "" {
		req.HeroID = "hero_043"
	}
	switch req.HeroID {
	case "hero_001", "hero_043", "hero_044", "hero_045", "hero_046", "hero_047", "hero_048", "hero_049", "hero_050":
	default:
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if req.ServerID < 0 || (service.AdminSelectedServerID(c.Request.Context()) > 0 && req.ServerID > 0 && req.ServerID != service.AdminSelectedServerID(c.Request.Context())) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminCreateGamePlayer(c.Request.Context(), adminPrincipal(c), req)
	if err != nil {
		gameAccountAdminError(c, err)
		return
	}
	adminSuccess(c, out)
}
