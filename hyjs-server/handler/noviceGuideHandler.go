package handler

import (
	"encoding/hex"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/routes"
	"hyjs-server/service"
	"hyjs-server/wrapper"
	"net/http"

	"github.com/gin-gonic/gin"
)

func init() {
	routes.RegisterPostRouter("/game-guide/skip", wrapper.Wrapper(GameGuideSkipHandler))
}

// GameGuideSkipHandler 保存当前角色主动跳过初始教学的选择。
// @Summary 跳过初始新手教学
// @Description 复用原游戏会话，保留主线和后续等级功能指引，不发放任何奖励
// @Tags GameGuide
// @Accept json
// @Produce json
// @Param body body model.GameGuideSkipRequest true "当前角色与游戏会话"
// @Success 200 {object} model.GameGuideSkipResponse
// @Router /game-guide/skip [post]
func GameGuideSkipHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var req model.GameGuideSkipRequest
	if c.ShouldBindJSON(&req) != nil || req.PlayerID < 1 || len(req.SessionHex) != 32 {
		return model.NewErrParamsError()
	}
	session, err := hex.DecodeString(req.SessionHex)
	if err != nil || len(session) != 16 {
		return model.NewErrParamsError()
	}
	// 常规授权先拒绝失效会话，事务内部再次核对并发换号。
	if _, err := service.AuthorizeAccount(c.Request.Context(), req.PlayerID, session); err != nil {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterDenied, constUtil.NoviceGuideSessionInvalid)
	}
	result, err := service.SkipNoviceGuide(c.Request.Context(), req.PlayerID, session)
	if err != nil {
		return err
	}
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}
