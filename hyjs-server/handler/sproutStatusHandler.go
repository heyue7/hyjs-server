package handler

import (
	"encoding/hex"
	"github.com/gin-gonic/gin"
	"hyjs-server/model"
	"hyjs-server/routes"
	"hyjs-server/wrapper"
	"net/http"
)

func init() { routes.RegisterPostRouter("/sprout-status", wrapper.Wrapper(SproutStatusHandler)) }

// SproutStatusHandler reads authorized scene sprout eligibility.
// @Summary 查询同场小芽资格
// @Description 仅允许当前角色和同场同线已显示角色，最多100人。
// @Tags Sprout
// @Accept json
// @Produce json
// @Param body body model.SproutStatusRequest true "角色、会话和同场角色"
// @Success 200 {object} model.SproutStatusResponse
// @Router /sprout-status [post]
func SproutStatusHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var req model.SproutStatusRequest
	if c.ShouldBindJSON(&req) != nil || req.PlayerID < 1 || len(req.SessionHex) != 32 || len(req.PlayerIDs) < 1 || len(req.PlayerIDs) > 100 || gameService == nil {
		return model.NewErrParamsError()
	}
	session, err := hex.DecodeString(req.SessionHex)
	if err != nil || len(session) != 16 {
		return model.NewErrParamsError()
	}
	seen := make(map[int64]bool, len(req.PlayerIDs))
	for _, id := range req.PlayerIDs {
		if id < 1 || seen[id] {
			return model.NewErrParamsError()
		}
		seen[id] = true
	}
	out, err := gameService.SproutStatus(c.Request.Context(), req, session)
	if err != nil {
		return err
	}
	c.JSON(http.StatusOK, model.NewBizCodeData(out))
	return nil
}
