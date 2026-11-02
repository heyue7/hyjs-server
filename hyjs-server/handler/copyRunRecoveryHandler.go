package handler

import (
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"hyjs-server/model"
	"hyjs-server/routes"
	"hyjs-server/wrapper"
)

func init() {
	routes.RegisterPostRouter("/copy/run/status", wrapper.Wrapper(CopyRunStatusHandler))
	routes.RegisterPostRouter("/copy/run/leave", wrapper.Wrapper(CopyRunLeaveHandler))
}

// CopyRunStatusHandler reports an incomplete original story-copy Run.
// @Summary 查询当前主线副本
// @Description 校验角色会话并读取当前副本标识，不修改副本进度或角色资产
// @Tags CopyRun
// @Accept json
// @Produce json
// @Param body body model.CopyRunStatusRequest true "角色和会话"
// @Success 200 {object} model.CopyRunRecoveryResponse
// @Router /copy/run/status [post]
func CopyRunStatusHandler(c *gin.Context) error {
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.CopyRunStatusRequest
	if c.ContentType() != "application/json" || c.ShouldBindJSON(&req) != nil || gameService == nil {
		return model.NewErrParamsError()
	}
	auth := model.GGOFieldAuth{PlayerID: req.PlayerID, SessionHex: req.SessionHex}
	session, err := ggoFieldSession(auth)
	if err != nil {
		return err
	}
	result, err := gameService.CopyRunStatus(c.Request.Context(), auth, session)
	if err != nil {
		return err
	}
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}

// CopyRunLeaveHandler confirms leaving the exact observed story-copy Run.
// @Summary 确认离开当前主线副本
// @Description 仅在副本摘要仍匹配时结束活动副本，进度变化时返回冲突
// @Tags CopyRun
// @Accept json
// @Produce json
// @Param body body model.CopyRunLeaveRequest true "角色、会话和副本摘要"
// @Success 200 {object} model.CopyRunRecoveryResponse
// @Router /copy/run/leave [post]
func CopyRunLeaveHandler(c *gin.Context) error {
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.CopyRunLeaveRequest
	if c.ContentType() != "application/json" || c.ShouldBindJSON(&req) != nil || gameService == nil ||
		len(req.RunToken) != 64 || strings.ToLower(req.RunToken) != req.RunToken {
		return model.NewErrParamsError()
	}
	if _, err := hex.DecodeString(req.RunToken); err != nil {
		return model.NewErrParamsError()
	}
	auth := model.GGOFieldAuth{PlayerID: req.PlayerID, SessionHex: req.SessionHex}
	session, err := ggoFieldSession(auth)
	if err != nil {
		return err
	}
	result, err := gameService.CopyRunLeave(c.Request.Context(), auth, session, req.RunToken)
	if err != nil {
		return err
	}
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}
