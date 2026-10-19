package handler

import (
	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
)

// RegisterAdminKingRecruitRoutes attaches read-only original king resources.
func RegisterAdminKingRecruitRoutes(a *gin.RouterGroup) {
	a.GET("/activity-king-resources", AdminKingRecruitResourcesHandler)
}

// AdminKingRecruitResourcesHandler lists original, recruitable king characters.
// @Summary 王者招募可用角色
// @Description 返回原SAO、ALO、GGO橙/红伙伴及名称、图片和世界标签；不返回主角或其他世界。
// @Tags AdminActivities
// @Accept json
// @Produce json
// @Success 200 {object} model.AdminKingRecruitResourcesResponse
// @Router /admin/api/activity-king-resources [get]
func AdminKingRecruitResourcesHandler(c *gin.Context) {
	if c.Request.URL.RawQuery != "" {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminKingRecruitResources(c.Request.Context(), adminPrincipal(c))
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
