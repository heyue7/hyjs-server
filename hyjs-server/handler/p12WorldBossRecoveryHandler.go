package handler

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	"hyjs-server/routes"
	"hyjs-server/wrapper"
)

func init() {
	routes.RegisterPostRouter("/world-boss/resume", wrapper.Wrapper(P12WorldBossResumeHandler))
	routes.RegisterPostRouter("/world-boss/abandon", wrapper.Wrapper(P12WorldBossAbandonHandler))
}

// P12WorldBossResumeHandler 让断线后的当前角色恢复服务器已冻结的开战帧。
// @Summary 恢复本地世界首领战斗
// @Description 当前会话认证；过期战斗只解除占用，客户端不能提供结果或伤害。
// @Tags LocalWorldBoss
// @Accept json
// @Produce json
// @Param X-Hyjs-Player-ID header int true "当前角色"
// @Param X-Hyjs-Session-Key header string true "当前原会话Base64"
// @Param body body model.P12WorldBossResumeRequest true "空恢复请求"
// @Success 200 {object} model.P12WorldBossResumeResponse
// @Router /world-boss/resume [post]
func P12WorldBossResumeHandler(c *gin.Context) error {
	var headers model.P12WorldBossFinishHeader
	if err := c.ShouldBindHeader(&headers); err != nil {
		return model.NewErrParamsError()
	}
	session, err := base64.StdEncoding.DecodeString(headers.SessionKey)
	if err != nil || len(session) != 16 || base64.StdEncoding.EncodeToString(session) != headers.SessionKey {
		return model.NewErrParamsError()
	}
	var request *model.P12WorldBossResumeRequest
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 1024))
	decoder.DisallowUnknownFields()
	if c.ContentType() != "application/json" || decoder.Decode(&request) != nil || request == nil {
		return model.NewErrParamsError()
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return model.NewErrParamsError()
	}
	if gameService == nil {
		return model.NewErrSystemError()
	}
	start, scene, err := gameService.ResumeP12WorldBossBattle(c.Request.Context(), headers.PlayerID, session)
	if err != nil {
		return err
	}
	result := model.P12WorldBossResumeResponse{Pending: start != nil, Scene: scene}
	if start != nil {
		raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(start)
		if err != nil {
			return err
		}
		result.StartBase64 = base64.StdEncoding.EncodeToString(raw)
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}

// P12WorldBossAbandonHandler 在当前会话中明确放弃未提交的战斗。
// @Summary 放弃本地世界首领战斗
// @Description 不产生伤害或奖励，不返还已用次数，已提交战斗拒绝放弃。
// @Tags LocalWorldBoss
// @Accept json
// @Produce json
// @Param X-Hyjs-Player-ID header int true "当前角色"
// @Param X-Hyjs-Session-Key header string true "当前原会话Base64"
// @Param body body model.P12WorldBossAbandonRequest true "放弃战斗编号"
// @Success 200 {object} model.P12WorldBossAbandonResponse
// @Router /world-boss/abandon [post]
func P12WorldBossAbandonHandler(c *gin.Context) error {
	var headers model.P12WorldBossFinishHeader
	if err := c.ShouldBindHeader(&headers); err != nil {
		return model.NewErrParamsError()
	}
	session, err := base64.StdEncoding.DecodeString(headers.SessionKey)
	if err != nil || len(session) != 16 || base64.StdEncoding.EncodeToString(session) != headers.SessionKey {
		return model.NewErrParamsError()
	}
	var request model.P12WorldBossAbandonRequest
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
	if err := gameService.AbandonP12WorldBossBattle(c.Request.Context(), headers.PlayerID, session, request.FightID); err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(model.P12WorldBossAbandonResponse{Abandoned: true}))
	return nil
}
