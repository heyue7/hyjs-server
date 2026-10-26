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
	routes.RegisterPostRouter("/guild-war/map-entry", wrapper.Wrapper(P13GuildMapEntryHandler))
}

// P13GuildMapEntryHandler probes the eligible map adapter before original field entry.
// @Summary 查询公会战入场地图能力
// @Description 校验当前角色会话和入场资格，读取准备窗口的地图投影，不接受客户端修改场次或规则。
// @Tags GuildWar
// @Accept json
// @Produce json
// @Param body body model.P13GuildMapEntryRequest true "角色与当前会话"
// @Success 200 {object} model.P13GuildMapEntryResponse
// @Router /guild-war/map-entry [post]
func P13GuildMapEntryHandler(c *gin.Context) error {
	var req model.P13GuildMapEntryRequest
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
	out, err := gameService.P13GuildMapEntry(ctx, req.PlayerID, session)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(out))
	return nil
}
