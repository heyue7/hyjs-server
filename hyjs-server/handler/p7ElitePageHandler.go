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
	routes.RegisterPostRouter("/rush/elite-page", wrapper.Wrapper(P7ElitePageHandler))
}

// P7ElitePageHandler 按当前可见窗口读取精英首领。
// @Summary 按需读取试炼精英目录
// @Description 每页最多5项，只完整复核当前选中首领；pendingIds必须等待单项复核，原挑战协议继续完整权威校验。
// @Tags Trial
// @Accept json
// @Produce json
// @Param body body model.P7ElitePageRequest true "原会话及窗口或单个首领"
// @Success 200 {object} model.P7ElitePageResponse
// @Router /rush/elite-page [post]
func P7ElitePageHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.P7ElitePageRequest
	if c.ShouldBindJSON(&req) != nil || req.PlayerID < 1 || len(req.SessionHex) != 32 || req.Limit < 1 || req.Limit > constUtil.P7ElitePageSize || req.Offset != nil && (*req.Offset < 0 || *req.Offset > constUtil.P7ElitePageMaxOffset) || req.BossID != "" && (req.Offset != nil || len(req.BossID) > 64 || strings.Trim(req.BossID, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_") != "") || len(req.SelectedBossID) > 64 || strings.Trim(req.SelectedBossID, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_") != "" {
		return model.NewErrParamsError()
	}
	session, err := hex.DecodeString(req.SessionHex)
	if err != nil || len(session) != 16 {
		return model.NewErrParamsError()
	}
	if gameService == nil {
		return model.NewErrSystemError()
	}
	out, err := gameService.P7ElitePage(c.Request.Context(), req.PlayerID, session, req.Offset, req.Limit, req.BossID, req.SelectedBossID)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(out))
	return nil
}
