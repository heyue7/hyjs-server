package handler

import (
	"math"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
)

// RegisterAdminNativeActivityRoutes 在已认证并具CSRF保护的后台Group注册精彩活动配置。
func RegisterAdminNativeActivityRoutes(a *gin.RouterGroup) {
	a.GET("/activity-native-settings", AdminNativeActivitiesHandler)
	a.PUT("/activity-native-settings/:id", AdminSaveNativeActivityHandler)
	a.DELETE("/activity-native-settings/:id", AdminRemoveNativeActivityHandler)
}

// AdminNativeActivitiesHandler 返回真实活动来源及当前生效规则。
// @Summary 精彩活动配置列表
// @Description 列出开服基金、支付、卡类、会员、副本福利、招募、首充双倍、庆典及节日/新版活动的真实配置
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} map[string][]model.AdminNativeActivityDTO
// @Router /admin/api/activity-native-settings [get]
func AdminNativeActivitiesHandler(c *gin.Context) {
	items, err := gameService.AdminNativeActivities(c.Request.Context(), adminPrincipal(c))
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, gin.H{"items": items})
}

// AdminSaveNativeActivityHandler 只修改已有真实配置行，冻结规则仅允许启停和别名。
// @Summary 保存精彩活动配置
// @Description 按版本保存经济字段；ID与类型不可编辑，冻结行变化返回409
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path string true "URL编码后的配置ID"
// @Param body body model.AdminNativeActivitySaveRequest true "版本、开关、别名与业务表单"
// @Success 200 {object} model.AdminNativeActivityDTO
// @Router /admin/api/activity-native-settings/{id} [put]
func AdminSaveNativeActivityHandler(c *gin.Context) {
	id := c.Param("id")
	var req model.AdminNativeActivitySaveRequest
	if id == "" || len(id) > 160 || c.ShouldBindJSON(&req) != nil || len(req.Version) != 64 || utf8.RuneCountInString(req.Alias) > 80 || !adminNativeFormValid(req.Form, true) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	// 原首充资格只有一笔订单，原连充页面固定三档；后台不开放无法被原消费者执行的目标。
	if strings.HasPrefix(id, "payment:firstRecharge_") && req.Form.Threshold != 1 || strings.HasPrefix(id, "payment:continuedRecharge_") && req.Form.Threshold != 3 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if !adminRecruitSettingsValid(req.RecruitSettings) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	req.Alias = strings.TrimSpace(req.Alias)
	item, err := gameService.AdminSaveNativeActivity(c.Request.Context(), adminPrincipal(c), id, req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, item)
}

// adminNativeFormValid 校验请求结构与数值范围；资源合法性和冻结版本仍由service按真实消费者判断。
func adminNativeFormValid(form model.AdminNativeActivityForm, allowChildren bool) bool {
	if form.StartAt < 0 || form.EndAt < 0 || form.ClaimEndAt < 0 {
		return false
	}
	if (form.StartAt == 0) != (form.EndAt == 0) || form.StartAt > 0 && form.EndAt <= form.StartAt {
		return false
	}
	if form.ClaimEndAt > 0 && form.EndAt > 0 && form.ClaimEndAt < form.EndAt {
		return false
	}
	if form.MinVip < 0 || form.MinVip > 12 || form.DiamondCost < 0 || form.DiamondCost > math.MaxInt32 || form.Threshold < 0 || form.Threshold > math.MaxInt32 || form.Cents < 0 || form.Cents > math.MaxInt32 || form.Charge < 0 || form.Charge > math.MaxInt32 || form.DailyDiamond < 0 || form.DailyDiamond > math.MaxInt32 {
		return false
	}
	if form.WeekdayMask < 0 || form.WeekdayMask > 127 || form.StartMinute < 0 || form.StartMinute > 1439 || form.EndMinute < 0 || form.EndMinute > 1440 {
		return false
	}
	if (form.StartMinute > 0 || form.EndMinute > 0) && form.StartMinute >= form.EndMinute {
		return false
	}
	if form.CurrencyFactor < 0 || form.CurrencyFactor > constUtil.P4CopyBenefitsMaximumFactor || form.DropFactor < 0 || form.DropFactor > constUtil.P4CopyBenefitsMaximumFactor {
		return false
	}
	if len(form.Rewards) > 100 || !adminNativeResourcesValid(form.Rewards) {
		return false
	}
	if len(form.Milestones) > 31 {
		return false
	}
	for _, milestone := range form.Milestones {
		if len(milestone.Key) > 64 || milestone.Goal < 0 || milestone.Goal > math.MaxInt32 || len(milestone.Rewards) > 100 || !adminNativeResourcesValid(milestone.Rewards) {
			return false
		}
	}
	if len(form.Prizes) > 1000 {
		return false
	}
	for _, prize := range form.Prizes {
		if prize.ID < 0 || prize.ID > math.MaxInt32 || len(prize.ResourceID) > 128 || prize.Amount < 0 || prize.Amount > math.MaxInt32 || prize.Weight < 0 || prize.Weight > 1000000000000 || prize.Stock < 0 || prize.Stock > math.MaxInt32 {
			return false
		}
	}
	if len(form.VipTiers) > 13 {
		return false
	}
	for _, tier := range form.VipTiers {
		if tier.Vip < 0 || tier.Vip > 12 || len(tier.DailyRewards) > 100 || len(tier.WeekRewards) > 100 || !adminNativeResourcesValid(tier.DailyRewards) || !adminNativeResourcesValid(tier.WeekRewards) || tier.WeekCost.Amount < 0 || tier.WeekCost.Amount > math.MaxInt32 || len(tier.WeekCost.ResourceID) > 128 {
			return false
		}
	}
	if len(form.Children) > 16 {
		return false
	}
	for _, child := range form.Children {
		if !allowChildren || child.ID == "" || len(child.ID) > 64 || (child.Domain != "period" && child.Domain != "commerce") || !adminNativeActivityFormValid(child.Form) {
			return false
		}
	}
	return true
}

// adminNativeActivityFormValid 校验节日子活动表单的数组上限和数值范围，业务合法性仍由原消费者校验。
func adminNativeActivityFormValid(form model.AdminActivityForm) bool {
	if utf8.RuneCountInString(form.Name) > 80 || len(form.Mode) > 64 || len(form.TemplateID) > 96 || len(form.Source) > 1000 || utf8.RuneCountInString(form.Alias) > 80 {
		return false
	}
	if form.StartAt < 0 || form.EndAt < 0 || form.ClaimEndAt < 0 || (form.StartAt == 0) != (form.EndAt == 0) || form.StartAt > 0 && form.EndAt <= form.StartAt || form.ClaimEndAt > 0 && form.EndAt > 0 && form.ClaimEndAt < form.EndAt {
		return false
	}
	if len(form.Products) > 100 || len(form.Milestones) > 31 || len(form.Prizes) > 1000 {
		return false
	}
	for _, product := range form.Products {
		if len(product.ID) > 64 || utf8.RuneCountInString(product.Name) > 128 || product.Index < 0 || len(product.Rewards) > 100 || !adminNativeResourcesValid(product.Rewards) || product.Cost.Amount < 0 || product.Cost.Amount > math.MaxInt32 || product.DailyLimit < 0 || product.DailyLimit > 10000 || product.TotalLimit < 0 || product.TotalLimit > math.MaxInt32 || product.Stock < -1 || product.Stock > math.MaxInt32 || product.MinLevel < 0 || product.MinLevel > 1000 || product.MinVip < 0 || product.MinVip > 12 || product.DiscountBps < 0 || product.DiscountBps > 10000 || product.Weight < 0 || product.Weight > 1000000000 {
			return false
		}
	}
	for _, milestone := range form.Milestones {
		if milestone.Goal < 0 || milestone.Goal > math.MaxInt32 || len(milestone.Rewards) > 100 || !adminNativeResourcesValid(milestone.Rewards) || len(milestone.Options) > 3 || !adminNativeResourcesValid(milestone.Options) {
			return false
		}
	}
	for _, prize := range form.Prizes {
		if prize.ID < 0 || prize.ID > math.MaxInt32 || len(prize.ResourceID) > 128 || prize.Amount < 0 || prize.Amount > math.MaxInt32 || prize.Weight < 0 || prize.Weight > 1000000000000 || prize.Stock < 0 || prize.Stock > math.MaxInt32 {
			return false
		}
	}
	return true
}

func adminNativeResourcesValid(values []model.AdminResourceAmount) bool {
	for _, value := range values {
		if value.ResourceID == "" || len(value.ResourceID) > 128 || value.Amount < 0 || value.Amount > math.MaxInt32 {
			return false
		}
	}
	return true
}

// AdminRemoveNativeActivityHandler 移除已停用配置并保留历史。
// @Summary 移除停用的精彩活动配置
// @Description 仅当前版本的停用配置可移除，源配置和奖励历史保留
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path string true "配置ID"
// @Param body body model.AdminNativeActivityRemoveRequest true "当前版本"
// @Success 200 {object} map[string]bool
// @Router /admin/api/activity-native-settings/{id} [delete]
func AdminRemoveNativeActivityHandler(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	var req model.AdminNativeActivityRemoveRequest
	if id == "" || len(id) > 160 || c.ShouldBindJSON(&req) != nil || len(req.Version) != 64 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if err := gameService.AdminRemoveNativeActivity(c.Request.Context(), adminPrincipal(c), id, req.Version); err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, gin.H{"removed": true})
}
