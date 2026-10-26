package handler

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/routes"
	"hyjs-server/wrapper"
)

func init() {
	routes.RegisterPostRouter("/guild-war/state", wrapper.Wrapper(P13GuildWarStateHandler))
}

// P13GuildWarStateHandler returns the authenticated live new-admission switch.
// @Summary 查询公会战总开关
// @Description 校验当前角色会话和区服，只读总开关；不会创建报名或修改已有场次。
// @Tags GuildWar
// @Accept json
// @Produce json
// @Param body body model.P13GuildWarStateRequest true "角色与当前会话"
// @Success 200 {object} model.P13GuildWarStateResponse
// @Router /guild-war/state [post]
func P13GuildWarStateHandler(c *gin.Context) error {
	var req model.P13GuildWarStateRequest
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 1024))
	decoder.DisallowUnknownFields()
	if c.ContentType() != "application/json" || decoder.Decode(&req) != nil || req.PlayerID < 1 || len(req.SessionHex) != 32 {
		return model.NewErrParamsError()
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return model.NewErrParamsError()
	}
	if gameService == nil {
		return model.NewErrSystemError()
	}
	session, err := hex.DecodeString(req.SessionHex)
	if err != nil || len(session) != 16 {
		return model.NewErrParamsError()
	}
	ctx := context.WithValue(c.Request.Context(), constUtil.GameServiceContextKey, gameService)
	out, err := gameService.P13GuildWarState(ctx, req.PlayerID, session)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(out))
	return nil
}
