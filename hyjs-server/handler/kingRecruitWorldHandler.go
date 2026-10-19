package handler

import (
	"encoding/hex"
	"github.com/gin-gonic/gin"
	"hyjs-server/model"
	"hyjs-server/routes"
	"hyjs-server/service"
	"hyjs-server/wrapper"
	"net/http"
	"strings"
	"unicode"
)

func init() {
	routes.RegisterPostRouter("/king-recruit/worlds", wrapper.Wrapper(KingRecruitWorldsHandler))
	routes.RegisterPostRouter("/king-recruit/select", wrapper.Wrapper(KingRecruitSelectHandler))
}

// KingRecruitWorldsHandler 查询当前角色可用的开放世界。
// @Summary 查询王者招募开放世界
// @Description 复用原游戏会话、服务器日期与活动资格，不扣费、不重置幸运值
// @Tags KingRecruit
// @Accept json
// @Produce json
// @Param body body model.KingRecruitWorldsRequest true "原角色与游戏会话"
// @Success 200 {object} model.KingRecruitWorldsResponse
// @Router /king-recruit/worlds [post]
func KingRecruitWorldsHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var req model.KingRecruitWorldsRequest
	if c.ShouldBindJSON(&req) != nil || req.PlayerID < 1 || len(req.SessionHex) != 32 {
		return model.NewErrParamsError()
	}
	session, err := hex.DecodeString(req.SessionHex)
	if err != nil || len(session) != 16 || gameService == nil {
		return model.NewErrParamsError()
	}
	if _, err = service.AuthorizeAccount(c.Request.Context(), req.PlayerID, session); err != nil {
		return err
	}
	result, err := gameService.KingRecruitWorlds(c.Request.Context(), req.PlayerID, session, "", "")
	if err != nil {
		return err
	}
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}

// KingRecruitSelectHandler 保存当前角色选择的开放世界。
// @Summary 选择王者招募世界
// @Description 校验原游戏会话、当前目录版本、活动资格及开放时间；原招募协议沿用该选择
// @Tags KingRecruit
// @Accept json
// @Produce json
// @Param body body model.KingRecruitSelectRequest true "当前目录中的世界与版本"
// @Success 200 {object} model.KingRecruitSelectResponse
// @Router /king-recruit/select [post]
func KingRecruitSelectHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.KingRecruitSelectRequest
	if c.ShouldBindJSON(&req) != nil || req.PlayerID < 1 || len(req.SessionHex) != 32 || len(req.PoolID) > 128 || len(req.Version) > 128 || strings.TrimSpace(req.PoolID) != req.PoolID || strings.IndexFunc(req.PoolID, unicode.IsControl) >= 0 {
		return model.NewErrParamsError()
	}
	session, err := hex.DecodeString(req.SessionHex)
	if err != nil || len(session) != 16 || gameService == nil {
		return model.NewErrParamsError()
	}
	if _, err = service.AuthorizeAccount(c.Request.Context(), req.PlayerID, session); err != nil {
		return err
	}
	result, err := gameService.KingRecruitWorlds(c.Request.Context(), req.PlayerID, session, req.PoolID, req.Version)
	if err != nil {
		return err
	}
	c.JSON(http.StatusOK, model.NewBizCodeData((*model.KingRecruitSelectResponse)(result)))
	return nil
}
