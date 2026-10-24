package handler

import (
	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"strings"
)

// AdminLuxurySignRulesHandler 读取原豪华签到运营配置。
// @Summary 豪华签到配置
// @Description 返回启停、每日奖励及原充值门槛。
// @Tags AdminActivities
// @Accept json
// @Produce json
// @Success 200 {object} model.AdminLuxurySignRulesResponse
// @Router /admin/api/luxury-sign-rules [get]
func AdminLuxurySignRulesHandler(c *gin.Context) {
	if c.Request.URL.RawQuery != "" {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminLuxurySignRules(c.Request.Context(), adminPrincipal(c))
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminSaveLuxurySignRulesHandler 校验并保存固定豪华签到源。
// @Summary 保存豪华签到
// @Description 保留原支付门槛与玩家领取历史。
// @Tags AdminActivities
// @Accept json
// @Produce json
// @Param body body model.AdminLuxurySignRulesSaveRequest true "豪华签到配置"
// @Success 200 {object} model.AdminLuxurySignRulesResponse
// @Router /admin/api/luxury-sign-rules [put]
func AdminSaveLuxurySignRulesHandler(c *gin.Context) {
	var req model.AdminLuxurySignRulesSaveRequest
	if c.Request.URL.RawQuery != "" || c.ShouldBindJSON(&req) != nil || len(req.Version) != 64 || len(req.Rewards) > 20 || req.Enabled && len(req.Rewards) == 0 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	for _, r := range req.Rewards {
		if r.ResourceID == "" || len(r.ResourceID) > 64 || strings.TrimSpace(r.ResourceID) != r.ResourceID || r.Amount < 1 || r.Amount > 2147483647 {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
	}
	out, err := gameService.AdminSaveLuxurySignRules(c.Request.Context(), adminPrincipal(c), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
