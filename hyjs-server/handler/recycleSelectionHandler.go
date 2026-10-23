package handler

import (
	"encoding/hex"
	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/routes"
	"hyjs-server/service"
	"hyjs-server/wrapper"
	"net/http"
)

func init() {
	routes.RegisterPostRouter("/recycle/selection", wrapper.Wrapper(RecycleSelectionHandler))
}

// RecycleSelectionHandler returns eligible candidates to the original recycle picker.
// @Summary 查询原生一键分解可选实例
// @Description 只读取当前角色实际资格，不扣费、不删除、不分解；确认后仍使用原生分解协议。
// @Tags Recycle
// @Accept json
// @Produce json
// @Param body body model.RecycleSelectionRequest true "当前会话、原类型与1至100件上限"
// @Success 200 {object} model.RecycleSelectionResponse
// @Router /recycle/selection [post]
func RecycleSelectionHandler(c *gin.Context) error {
	var req model.RecycleSelectionRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlayerID < 1 || len(req.SessionHex) != 32 || req.Limit < 1 || req.Limit > constUtil.RecycleDecomposeMaxBatch || (req.Kind != "hero" && req.Kind != "equipment" && req.Kind != "pet") {
		return model.NewErrParamsError()
	}
	session, err := hex.DecodeString(req.SessionHex)
	if err != nil || len(session) != 16 {
		return model.NewErrParamsError()
	}
	if gameService == nil {
		return model.NewErrSystemError()
	}
	owner, err := service.AuthorizeAccount(c.Request.Context(), req.PlayerID, session)
	if err != nil {
		return model.NewErrBizCode("登录已失效，请重新登录")
	}
	ctx, err := gameService.BindLocalPlayContext(c.Request.Context(), owner)
	if err != nil {
		return err
	}
	result, err := gameService.RecycleSelection(ctx, req.PlayerID, session, req.Kind, int(req.Limit))
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}
