package handler

import (
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/routes"
	"hyjs-server/wrapper"
)

func init() {
	routes.RegisterPostRouter("/ggo-field/battle/resume", wrapper.Wrapper(GGOFieldBattleResumeHandler))
}

// GGOFieldBattleResumeHandler returns only the requested accepted fight or its immutable receipt.
// @Summary 恢复已接受的沙漠战斗
// @Description 按战斗编号与冻结输入摘要读取原生开战或结算消息，不创建战斗或发放奖励
// @Tags GGOField
// @Accept json
// @Produce json
// @Param body body model.GGOFieldBattleResumeRequest true "角色、会话、战斗类型与原身份"
// @Success 200 {object} model.GGOFieldBattleResumeResponse
// @Router /ggo-field/battle/resume [post]
func GGOFieldBattleResumeHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var request model.GGOFieldBattleResumeRequest
	if c.ShouldBindJSON(&request) != nil || gameService == nil ||
		(request.BattleType != constUtil.GGOBattleResumeMonster && request.BattleType != constUtil.GGOBattleResumeBoss) ||
		!ggoMonsterFightID(request.ExpectedFightID) || len(request.ExpectedInputHash) != 64 ||
		strings.ToLower(request.ExpectedInputHash) != request.ExpectedInputHash {
		return model.NewErrParamsError()
	}
	if _, err := hex.DecodeString(request.ExpectedInputHash); err != nil {
		return model.NewErrParamsError()
	}
	session, err := ggoFieldSession(request.GGOFieldAuth)
	if err != nil {
		return err
	}
	result, err := gameService.GGOFieldBattleResume(c.Request.Context(), request, session)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}
