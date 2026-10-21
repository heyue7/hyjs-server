package handler

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
)

// RegisterAdminActivityScheduleRoutes attaches global definitions and realm calendars to the protected admin group.
func RegisterAdminActivityScheduleRoutes(a *gin.RouterGroup) {
	a.GET("/activity-cost-resources", AdminActivityCostResourcesHandler)
	a.GET("/activity-templates", AdminActivityDefinitionsHandler)
	a.POST("/activity-templates", AdminActivityDefinitionCreateHandler)
	a.PUT("/activity-templates/:id", AdminActivityDefinitionSaveHandler)
	a.GET("/activity-schedules", AdminActivityScheduleHandler)
	a.POST("/activity-schedules/preview", AdminActivitySchedulePreviewHandler)
	a.PUT("/activity-schedules", AdminActivityScheduleSaveHandler)
	a.GET("/activity-patterns", AdminActivityPatternsHandler)
	a.POST("/activity-patterns", AdminActivityPatternCreateHandler)
	a.PUT("/activity-patterns/:id", AdminActivityPatternSaveHandler)
	a.DELETE("/activity-patterns/:id", AdminActivityPatternDeleteHandler)
}

// AdminActivityDefinitionsHandler reads global reusable typed forms.
// @Summary 全局活动设置
// @Description 仅返回全局模板，修改不会变更已发布排期和历史领取
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {array} model.AdminActivityDefinitionDTO
// @Router /admin/api/activity-templates [get]
func AdminActivityDefinitionsHandler(c *gin.Context) {
	out, err := gameService.AdminActivityDefinitions(c.Request.Context(), adminPrincipal(c))
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, gin.H{"items": out})
}

// AdminActivityDefinitionCreateHandler creates an unscheduled global template.
// @Summary 新建全局活动模板
// @Description 使用原模式资源和条件的业务表单，日期由区服排期指定
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminActivityDefinitionSaveRequest true "全局活动设置"
// @Success 200 {object} model.AdminActivityDefinitionDTO
// @Router /admin/api/activity-templates [post]
func AdminActivityDefinitionCreateHandler(c *gin.Context) { adminActivityDefinitionWrite(c, false) }

// AdminActivityDefinitionSaveHandler edits future scheduling input only.
// @Summary 修改全局活动模板
// @Description 使用版本保护，所有已发布活动继续使用自己的规则快照
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path string true "模板ID"
// @Param body body model.AdminActivityDefinitionSaveRequest true "全局活动设置和版本"
// @Success 200 {object} model.AdminActivityDefinitionDTO
// @Router /admin/api/activity-templates/{id} [put]
func AdminActivityDefinitionSaveHandler(c *gin.Context) { adminActivityDefinitionWrite(c, true) }

func adminActivityDefinitionWrite(c *gin.Context, update bool) {
	var req model.AdminActivityDefinitionSaveRequest
	if c.ShouldBindJSON(&req) != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Form.Alias = strings.TrimSpace(req.Form.Alias)
	form := req.Form
	if (form.RepeatPolicy != "" && form.RepeatPolicy != "once" && form.RepeatPolicy != "repeat") || req.Name == "" || len([]rune(req.Name)) > 80 || req.DefaultDays < 1 || req.DefaultDays > 366 || req.ClaimDays < 0 || req.ClaimDays > 90 || len([]rune(form.Alias)) > 80 || len(form.Mode) > 64 || len(form.TemplateID) > 96 || len(form.Source) > 1000 || len(form.Products) > 100 || len(form.Milestones) > 31 || (len(form.Prizes) > 1000 || form.Mode != "king-recruit" && len(form.Prizes) > 9) || form.Mode == "opening-14" && form.TemplateID != "opening-14-original" || update && (len(req.Version) != 64 || len(c.Param("id")) > 64) || !update && req.Version != "" {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	id := ""
	if update {
		id = c.Param("id")
	}
	if !adminRecruitSettingsValid(req.RecruitSettings) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminSaveActivityDefinition(c.Request.Context(), adminPrincipal(c), id, req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminActivityScheduleHandler returns the selected realm's complete calendar.
// @Summary 区服活动排期
// @Description 返回全日期排期，当前日历视图的隐藏日期不会被覆盖丢弃
// @Tags Admin
// @Accept json
// @Produce json
// @Param serverId query int true "已登记区服ID"
// @Success 200 {object} model.AdminActivityScheduleResponse
// @Router /admin/api/activity-schedules [get]
func AdminActivityScheduleHandler(c *gin.Context) {
	serverID, err := strconv.ParseInt(c.Query("serverId"), 10, 64)
	if err != nil || serverID < 1 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminActivitySchedule(c.Request.Context(), adminPrincipal(c), serverID)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// adminActivityCalendarInput centralizes the shared complete-calendar structural contract.
// Business conflicts and original consumer rules remain in the schedule service.
func adminActivityCalendarInput(version string, items []model.AdminActivityOccurrenceDTO) bool {
	if len(version) != 64 || len(items) > 1000 {
		return false
	}
	seen := map[string]bool{}
	for _, item := range items {
		if item.Timing != "" && item.Timing != "calendar" && item.Timing != "character-day" || item.StartDay < 0 || item.StartDay > 366 || item.DurationDays < 0 || item.DurationDays > 366 || item.ClaimDays < 0 || item.ClaimDays > 90 {
			return false
		}
		if len(item.ID) > 64 || len(item.TemplateID) < 1 || len(item.TemplateID) > 64 || item.StartAt < 946684800000 || item.EndAt <= item.StartAt || item.EndAt-item.StartAt > 366*86400000 || item.ClaimEndAt < item.EndAt || item.ClaimEndAt-item.EndAt > 90*86400000 || item.Lane < 0 || item.Lane > 31 || item.ID != "" && len(item.Version) != 64 || item.ID == "" && len(item.TemplateVersion) != 64 {
			return false
		}
		if item.ID != "" {
			if seen[item.ID] {
				return false
			}
			seen[item.ID] = true
		}
	}
	return true
}

// AdminActivitySchedulePreviewHandler checks dates, singleton overlap and frozen history without writing.
// @Summary 预览区服排期冲突
// @Description 相同原无实例模式不可重叠，不同活动可并行；已冻结时间受保护
// @Tags Admin
// @Accept json
// @Produce json
// @Param serverId query int true "已登记区服ID"
// @Param body body model.AdminActivitySchedulePreviewRequest true "整份排期预览"
// @Success 200 {object} model.AdminActivitySchedulePreviewResponse
// @Router /admin/api/activity-schedules/preview [post]
func AdminActivitySchedulePreviewHandler(c *gin.Context) {
	var req model.AdminActivitySchedulePreviewRequest
	serverID, err := strconv.ParseInt(c.Query("serverId"), 10, 64)
	if err != nil || serverID < 1 || c.ShouldBindJSON(&req) != nil || !adminActivityCalendarInput(req.Version, req.Items) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminPreviewActivitySchedule(c.Request.Context(), adminPrincipal(c), serverID, req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminActivityScheduleSaveHandler publishes all proposed changes in one transaction.
// @Summary 原子保存区服活动排期
// @Description 版本冲突返回409；成功后同事务写原玩法消费者及审计
// @Tags Admin
// @Accept json
// @Produce json
// @Param serverId query int true "已登记区服ID"
// @Param body body model.AdminActivityScheduleSaveRequest true "整份排期和版本"
// @Success 200 {object} model.AdminActivityScheduleResponse
// @Router /admin/api/activity-schedules [put]
func AdminActivityScheduleSaveHandler(c *gin.Context) {
	var req model.AdminActivityScheduleSaveRequest
	serverID, err := strconv.ParseInt(c.Query("serverId"), 10, 64)
	if err != nil || serverID < 1 || c.ShouldBindJSON(&req) != nil || !adminActivityCalendarInput(req.Version, req.Items) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminSaveActivitySchedule(c.Request.Context(), adminPrincipal(c), serverID, req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminActivityPatternsHandler lists reusable global relative-day schemes.
// @Summary 活动排期方案库
// @Description 方案通过日期偏移复用，可拖入任一区服的排期草稿
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {array} model.AdminActivityPatternDTO
// @Router /admin/api/activity-patterns [get]
func AdminActivityPatternsHandler(c *gin.Context) {
	out, err := gameService.AdminActivityPatterns(c.Request.Context(), adminPrincipal(c))
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, gin.H{"items": out})
}

// AdminActivityPatternCreateHandler creates a reusable rotation.
// @Summary 保存新排期方案
// @Description 仅保存相对日序和模板引用，不直接发布活动
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminActivityPatternSaveRequest true "相对日序方案"
// @Success 200 {object} model.AdminActivityPatternDTO
// @Router /admin/api/activity-patterns [post]
func AdminActivityPatternCreateHandler(c *gin.Context) { adminActivityPatternWrite(c, false) }

// AdminActivityPatternSaveHandler edits a reusable scheme.
// @Summary 修改排期方案
// @Description 已拖入区服的活动独立保存，修改方案不追改已发布内容
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path string true "方案ID"
// @Param body body model.AdminActivityPatternSaveRequest true "方案与版本"
// @Success 200 {object} model.AdminActivityPatternDTO
// @Router /admin/api/activity-patterns/{id} [put]
func AdminActivityPatternSaveHandler(c *gin.Context) { adminActivityPatternWrite(c, true) }
func adminActivityPatternWrite(c *gin.Context, update bool) {
	var req model.AdminActivityPatternSaveRequest
	if c.ShouldBindJSON(&req) != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len([]rune(req.Name)) > 80 || req.CycleDays < 1 || req.CycleDays > 366 || len(req.Entries) < 1 || len(req.Entries) > 100 || update && (len(c.Param("id")) > 64 || len(req.Version) != 64) || !update && req.Version != "" {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	for _, entry := range req.Entries {
		if entry.TemplateID == "" || len(entry.TemplateID) > 64 || entry.StartDay < 0 || entry.StartDay > 365 || entry.DurationDays < 1 || entry.DurationDays > 366 || entry.ClaimDays < 0 || entry.ClaimDays > 90 || entry.Lane < 0 || entry.Lane > 31 {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
	}
	id := ""
	if update {
		id = c.Param("id")
	}
	out, err := gameService.AdminSaveActivityPattern(c.Request.Context(), adminPrincipal(c), id, req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminActivityPatternDeleteHandler archives a scheme without touching copied realm dates.
// @Summary 归档排期方案
// @Description 保留已经发布的区服活动与全部历史
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path string true "方案ID"
// @Param body body model.AdminActivityDeleteRequest true "方案版本"
// @Success 200 {object} map[string]bool
// @Router /admin/api/activity-patterns/{id} [delete]
func AdminActivityPatternDeleteHandler(c *gin.Context) {
	var req model.AdminActivityDeleteRequest
	if len(c.Param("id")) > 64 || c.ShouldBindJSON(&req) != nil || len(req.Version) != 64 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if err := gameService.AdminDeleteActivityPattern(c.Request.Context(), adminPrincipal(c), c.Param("id"), req.Version); err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, gin.H{"ok": true})
}

// AdminActivityCostResourcesHandler returns a searchable catalogue accepted by original activity purchases.
// @Summary 活动可用消耗资源
// @Description 只返回原交易实际支持的货币与道具，包含可读名称和缩略图
// @Tags Admin
// @Accept json
// @Produce json
// @Param q query string false "资源名称或ID"
// @Success 200 {object} model.AdminActivityCostResourcesResponse
// @Router /admin/api/activity-cost-resources [get]
func AdminActivityCostResourcesHandler(c *gin.Context) {
	if _, err := url.ParseQuery(c.Request.URL.RawQuery); err != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	page, size, ok := adminPage(c)
	req := model.AdminItemsRequest{Q: strings.TrimSpace(c.Query("q")), Category: strings.TrimSpace(c.Query("category")), ApplicableType: strings.TrimSpace(c.Query("applicableType")), Page: page, PageSize: size}
	if !ok || len(req.Q) > 128 || len(req.Category) > 128 || len(req.ApplicableType) > 128 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if raw, present := c.GetQuery("quality"); present {
		quality, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil || quality < 0 {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
		req.Quality = &quality
	}
	if raw := c.Query("ids"); raw != "" {
		parts := strings.Split(raw, ",")
		if len(parts) > 100 {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
		for _, part := range parts {
			id := strings.TrimSpace(part)
			if id == "" || len(id) > 128 || strings.ContainsAny(id, " \t\r\n/\\?#&;\x00") {
				adminError(c, constUtil.ErrAdminInput)
				return
			}
			req.IDs = append(req.IDs, id)
		}
	}
	if raw := c.Query("preferredIds"); raw != "" {
		parts := strings.Split(raw, ",")
		if len(parts) > 20 {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
		for _, part := range parts {
			id := strings.TrimSpace(part)
			if id == "" || len(id) > 128 || strings.ContainsAny(id, " \t\r\n/\\?#&;\x00") {
				adminError(c, constUtil.ErrAdminInput)
				return
			}
			req.PreferredIDs = append(req.PreferredIDs, id)
		}
	}
	facets := gameService.AdminCatalogueFiltered(model.AdminItemsRequest{Page: 1, PageSize: 1}).Facets
	categoryOK, typeOK, qualityOK := req.Category == "", req.ApplicableType == "", req.Quality == nil
	for _, option := range facets.Categories {
		if option.Value == req.Category {
			categoryOK = true
		}
	}
	for _, option := range facets.ApplicableTypes {
		if option.Value == req.ApplicableType {
			typeOK = true
		}
	}
	for _, option := range facets.Qualities {
		if req.Quality != nil && option.Value == *req.Quality {
			qualityOK = true
		}
	}
	if !categoryOK || !typeOK || !qualityOK {
		adminError(c, constUtil.ErrAdminInput)
		return
	}

	out, err := gameService.AdminActivityCostResources(c.Request.Context(), adminPrincipal(c), req.Q, req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
