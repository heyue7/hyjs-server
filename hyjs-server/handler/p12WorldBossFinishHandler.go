package handler

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"hyjs-server/model"
	"hyjs-server/routes"
	"hyjs-server/wrapper"
)

func init() {
	routes.RegisterPostRouter("/world-boss/finish", wrapper.Wrapper(P12WorldBossFinishHandler))
}

// P12WorldBossFinishHandler 在原跳过/自然结束回调提交服务器已冻结的一场伤害。
// @Summary 提交本地世界首领战斗结束
// @Description 当前会话认证；请求只提供fightId，扣血及重复回执均由服务器事务决定。
// @Tags LocalWorldBoss
// @Accept json
// @Produce json
// @Param X-Hyjs-Player-ID header int true "当前角色"
// @Param X-Hyjs-Session-Key header string true "当前原会话Base64"
// @Param body body model.P12WorldBossFinishRequest true "已预计算战斗编号"
// @Success 200 {object} model.P12WorldBossFinishResponse
// @Router /world-boss/finish [post]
func P12WorldBossFinishHandler(c *gin.Context) error {
	var headers model.P12WorldBossFinishHeader
	if err := c.ShouldBindHeader(&headers); err != nil {
		return model.NewErrParamsError()
	}
	session, err := base64.StdEncoding.DecodeString(headers.SessionKey)
	if err != nil || len(session) != 16 || base64.StdEncoding.EncodeToString(session) != headers.SessionKey {
		return model.NewErrParamsError()
	}
	// 原兼容接口只接收服务器战斗编号，未知结果/伤害字段和第二个JSON值均拒绝。
	var request model.P12WorldBossFinishRequest
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 1024))
	decoder.DisallowUnknownFields()
	if c.ContentType() != "application/json" || decoder.Decode(&request) != nil || request.FightID == "" || len(request.FightID) > 64 || strings.Trim(request.FightID, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_") != "" {
		return model.NewErrParamsError()
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return model.NewErrParamsError()
	}
	if gameService == nil {
		return model.NewErrSystemError()
	}
	result, err := gameService.FinishP12WorldBossBattle(c.Request.Context(), headers.PlayerID, session, request.FightID)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}
