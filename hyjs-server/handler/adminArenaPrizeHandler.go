package handler

import (
	"encoding/hex"
	"time"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/service"
)

// RegisterAdminArenaPrizeRoutes uses the existing authenticated CSRF group.
func RegisterAdminArenaPrizeRoutes(a *gin.RouterGroup) {
	a.GET("/arena/prizes", AdminArenaPrizeHandler)
	a.PUT("/arena/prizes", AdminArenaPrizeSaveHandler)
}

// AdminArenaPrizeHandler returns typed actual arena victory prize settings.
// @Summary 竞技胜利翻牌配置
// @Description 常规金币范围及原独立运营奖池，附版本与冻结状态
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} model.AdminArenaPrizeResponse
// @Router /admin/api/arena/prizes [get]
func AdminArenaPrizeHandler(c *gin.Context) {
	out, err := gameService.AdminArenaPrizes(c.Request.Context(), adminPrincipal(c))
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminArenaPrizeSaveHandler validates the typed editor before an audited save.
// @Summary 保存竞技胜利翻牌配置
// @Description 新范围只影响后继胜利，旧回执不重抽；省略operatorPool保留运营奖池
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminArenaPrizeSaveRequest true "金币范围与可选运营奖池"
// @Success 200 {object} model.AdminArenaPrizeResponse
// @Router /admin/api/arena/prizes [put]
func AdminArenaPrizeSaveHandler(c *gin.Context) {
	var req model.AdminArenaPrizeSaveRequest
	if c.ShouldBindJSON(&req) != nil || len(req.Version) != 64 || req.RegularCoin.Min < 1 || req.RegularCoin.Max < req.RegularCoin.Min || req.RegularCoin.Max > constUtil.P11ArenaPrizeCoinMaximum {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if decoded, err := hex.DecodeString(req.Version); err != nil || len(decoded) != 32 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if pool := req.OperatorPool; pool != nil {
		if pool.MinLevel < 10 || pool.MinLevel > 2147483647 || pool.DailyWins < 1 || pool.DailyWins > 100 || pool.WindowWins < pool.DailyWins || pool.WindowWins > 3100 || len(pool.Items) < 1 || len(pool.Items) > 32 {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
		if pool.StartAt == 0 && pool.EndAt == 0 && !pool.Enabled {
		} else {
			if pool.StartAt < 1 || pool.StartAt >= pool.EndAt || pool.EndAt > 7258118400000 {
				adminError(c, constUtil.ErrAdminInput)
				return
			}
			zone := time.FixedZone("UTC+8", 28800)
			if time.UnixMilli(pool.StartAt).In(zone).Format("200601") != time.UnixMilli(pool.EndAt-1).In(zone).Format("200601") {
				adminError(c, constUtil.ErrAdminInput)
				return
			}
		}
		seen := map[string]map[int64]bool{}
		for _, item := range pool.Items {
			if !service.AdminArenaPrizeResourceAllowed(item.ResourceID) || item.Amount < 1 || item.Amount > 2147483647 || item.Weight < 1 || item.Weight > 1000000000 {
				adminError(c, constUtil.ErrAdminInput)
				return
			}
			if seen[item.ResourceID] == nil {
				seen[item.ResourceID] = map[int64]bool{}
			}
			if seen[item.ResourceID][item.Amount] {
				adminError(c, constUtil.ErrAdminInput)
				return
			}
			seen[item.ResourceID][item.Amount] = true
		}
	}
	out, err := gameService.AdminSaveArenaPrizes(c.Request.Context(), adminPrincipal(c), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
