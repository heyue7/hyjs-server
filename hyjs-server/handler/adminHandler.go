package handler

import (
	"errors"
	"html/template"
	"hyjs-server/middleware"
	"hyjs-server/utils"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/service"
)

// RegisterAdminRoutes 在既有Go端口接入独立后台认证及原运营服务。
func RegisterAdminRoutes(r *gin.Engine, assets string) {
	a := r.Group("/admin/api")
	a.Use(adminSecurityObservation)
	a.Use(func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1048576)
		if c.Request.Method != "GET" && !middleware.ValidAdminOrigin(c.Request, middleware.SecureAdminCookies(c)) {
			adminError(c, constUtil.ErrAdminPermission)
			c.Abort()
			return
		}
		c.Next()
	})
	a.POST("/auth/login", AdminLoginHandler)
	a.Use(adminAuthentication)
	a.Use(adminServerScope)
	RegisterAdminSecurityEventRoutes(a)
	RegisterAdminGameServerRoutes(a)
	RegisterAdminActivityScheduleRoutes(a)
	RegisterAdminFieldRoutes(a)
	RegisterAdminGuildRoutes(a)
	RegisterAdminGuildRewardRoutes(a)
	RegisterAdminActivityModeRoutes(a)
	RegisterAdminKingRecruitRoutes(a)
	RegisterAdminRecruitRoutes(a)
	RegisterAdminGamePlayerRoutes(a)
	RegisterAdminAccountCreationRoutes(a)
	RegisterAdminWalletRoutes(a)
	RegisterAdminServerGameClockRoutes(a)
	RegisterAdminShopRoutes(a)
	RegisterAdminArenaPrizeRoutes(a)
	RegisterAdminGameContentRoutes(a)
	RegisterAdminFeedbackRoutes(a)
	RegisterAdminCDKRoutes(a)
	RegisterAdminItemDetailRoutes(a)
	RegisterAdminBroadcastRoutes(a)
	RegisterAdminMailRoutes(a)
	RegisterAdminMonthSignRoutes(a)
	RegisterAdminActivityDeletionRoutes(a)
	a.GET("/auth/me", AdminMeHandler)
	a.POST("/auth/logout", AdminLogoutHandler)
	a.GET("/overview", AdminOverviewHandler)
	a.GET("/items", AdminItemsHandler)
	a.GET("/players", AdminPlayersHandler)
	a.GET("/activities", AdminActivitiesHandler)
	a.PUT("/activities/:id", AdminActivityUpdateHandler)
	a.POST("/activities", AdminActivityCreateHandler)
	a.GET("/announcements", AdminAnnouncementsHandler)
	a.POST("/announcements", AdminAnnouncementSaveHandler)
	a.PUT("/announcements/:id", AdminAnnouncementSaveHandler)
	a.GET("/audit", AdminAuditHandler)
	a.POST("/grants", AdminGrantHandler)
	a.GET("/users", AdminUsersHandler)
	a.POST("/users", AdminUserCreateHandler)
	a.PUT("/users/:id/role", AdminUserRoleHandler)
	a.PUT("/users/:id/password", AdminUserPasswordHandler)
	r.GET("/notices", PublicAnnouncementsHandler)
	if assets != "" {
		root, e := filepath.Abs(assets)
		if e == nil {
			if st, err := os.Stat(filepath.Join(root, "index.html")); err == nil && !st.IsDir() {
				r.GET("/admin", func(c *gin.Context) { c.Redirect(http.StatusTemporaryRedirect, "/admin/") })
				r.GET("/admin/", func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.File(filepath.Join(root, "index.html")) })
				r.Static("/admin/assets", filepath.Join(root, "assets"))
				r.Static("/admin/media", filepath.Join(root, "media"))
				if _, err := os.Stat(filepath.Join(root, "favicon.svg")); err == nil {
					r.StaticFile("/admin/favicon.svg", filepath.Join(root, "favicon.svg"))
				}
				r.NoRoute(func(c *gin.Context) {
					if strings.HasPrefix(c.Request.URL.Path, "/admin/") && !strings.HasPrefix(c.Request.URL.Path, "/admin/api/") {
						c.Header("Cache-Control", "no-store")
						c.File(filepath.Join(root, "index.html"))
						return
					}
					c.Status(http.StatusNotFound)
				})
			}
		}
	}
}
func adminError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	message := "操作失败，请查看服务日志"
	switch {
	case errors.Is(err, constUtil.ErrAdminSession), errors.Is(err, constUtil.ErrAdminCredentials):
		status = http.StatusUnauthorized
		message = err.Error()
	case errors.Is(err, constUtil.ErrAdminPermission):
		status = http.StatusForbidden
		message = err.Error()
	case errors.Is(err, utils.ErrPasswordWorkBusy):
		status = http.StatusServiceUnavailable
		message = "登录服务繁忙，请稍后重试"
		c.Header("Retry-After", "2")
	case errors.Is(err, constUtil.ErrAdminLoginCooldown):
		status = http.StatusTooManyRequests
		message = err.Error()
	case errors.Is(err, constUtil.ErrAdminConfigConflict):
		status = http.StatusConflict
		message = err.Error()
	case errors.Is(err, constUtil.ErrAdminInput), errors.Is(err, constUtil.ErrAdminConfiguration):
		status = http.StatusBadRequest
		message = err.Error()
	}
	c.JSON(status, gin.H{"code": status, "msg": message, "data": nil})
}
func adminSuccess(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "", "data": data})
}
func adminPrincipal(c *gin.Context) *model.AdminPrincipal {
	p, _ := c.Get(constUtil.AdminPrincipalContextKey)
	v, _ := p.(*model.AdminPrincipal)
	return v
}
func adminAuthentication(c *gin.Context) {
	token, _ := c.Cookie(constUtil.AdminSessionCookieName)
	p, err := service.AdminAuthenticate(c.Request.Context(), token)
	if err != nil {
		adminError(c, err)
		c.Abort()
		return
	}
	if c.Request.Method != "GET" && !service.AdminCheckCSRF(p, c.GetHeader("X-CSRF-Token")) {
		adminError(c, constUtil.ErrAdminPermission)
		c.Abort()
		return
	}
	c.Set(constUtil.AdminPrincipalContextKey, p)
	c.Next()
}
func adminPage(c *gin.Context) (int64, int64, bool) {
	page, size := int64(1), int64(20)
	var err error
	if raw := c.Query("page"); raw != "" {
		page, err = strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return 0, 0, false
		}
	}
	if raw := c.Query("pageSize"); raw != "" {
		size, err = strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return 0, 0, false
		}
	}
	return page, size, page > 0 && page <= 100000 && size > 0 && size <= 100
}
func adminID(c *gin.Context) (int64, bool) {
	if c.Param("id") == "" {
		return 0, true
	}
	id, e := strconv.ParseInt(c.Param("id"), 10, 64)
	return id, e == nil && id > 0
}

// AdminLoginHandler 验证独立运营账号并设置绑定CSRF的会话Cookie。
// @Summary 后台登录
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminLoginRequest true "登录"
// @Success 200 {object} model.AdminUserDTO
// @Router /admin/api/auth/login [post]
func AdminLoginHandler(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var req model.AdminLoginRequest
	if c.ShouldBindJSON(&req) != nil || len(req.Username) > 64 || len(req.Password) > 72 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	c.Set("hyjs-login-actor", strings.ToLower(strings.TrimSpace(req.Username)))
	if !service.LoginIngressAllowed(c.ClientIP(), time.Now()) {
		c.Header("Retry-After", "2")
		adminError(c, constUtil.ErrAdminLoginCooldown)
		return
	}
	result, err := service.AdminLogin(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		adminError(c, err)
		return
	}
	c.SetSameSite(http.SameSiteStrictMode)
	secure := middleware.SecureAdminCookies(c)
	c.SetCookie(constUtil.AdminSessionCookieName, result.SessionToken, int(constUtil.AdminSessionLifetimeSeconds), "/admin", "", secure, true)
	c.SetCookie(constUtil.AdminCSRFCookieName, result.CsrfToken, int(constUtil.AdminSessionLifetimeSeconds), "/admin", "", secure, false)
	adminSuccess(c, gin.H{"user": result.User, "csrfToken": result.CsrfToken})
}

// AdminMeHandler 返回当前已认证身份与本会话CSRF。
// @Summary 后台当前身份
// @Tags Admin
// @Produce json
// @Success 200 {object} model.AdminUserDTO
// @Router /admin/api/auth/me [get]
func AdminMeHandler(c *gin.Context) {
	p := adminPrincipal(c)
	csrf, _ := c.Cookie(constUtil.AdminCSRFCookieName)
	if !service.AdminCheckCSRF(p, csrf) {
		adminError(c, constUtil.ErrAdminSession)
		return
	}
	adminSuccess(c, gin.H{"user": model.AdminUserDTO{ID: p.UserID, Username: p.Username, Role: p.Role}, "csrfToken": csrf})
}

// AdminLogoutHandler 撤销当前会话。
// @Summary 后台退出
// @Tags Admin
// @Produce json
// @Success 200 {object} map[string]bool
// @Router /admin/api/auth/logout [post]
func AdminLogoutHandler(c *gin.Context) {
	token, _ := c.Cookie(constUtil.AdminSessionCookieName)
	if err := service.AdminLogout(c.Request.Context(), token); err != nil {
		adminError(c, err)
		return
	}
	c.SetSameSite(http.SameSiteStrictMode)
	for _, n := range []string{constUtil.AdminSessionCookieName, constUtil.AdminCSRFCookieName} {
		c.SetCookie(n, "", -1, "/admin", "", middleware.SecureAdminCookies(c), n == constUtil.AdminSessionCookieName)
	}
	adminSuccess(c, gin.H{"ok": true})
}

// AdminOverviewHandler 查询实际运营计数。
// @Summary 运营概览
// @Tags Admin
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /admin/api/overview [get]
func AdminOverviewHandler(c *gin.Context) {
	count := gameService.AdminCatalogue("", "", 1, 1).Total
	out, e := service.AdminOverview(c.Request.Context(), count)
	if e != nil {
		adminError(c, e)
		return
	}
	adminSuccess(c, out)
}

// AdminItemsHandler 搜索独立加载的原物品目录并校验所有筛选枚举。
// @Summary 道具目录
// @Description 按中文名称、原ID、原品质及有来源分组的类型筛选；矿点、钓点和怪物不列为可发放物品。
// @Tags Admin
// @Accept json
// @Produce json
// @Param q query string false "中文名称、原ID或说明，最多128字节"
// @Param category query string false "facets.categories中的分类值"
// @Param quality query integer false "颜色品质：0未标注1白2绿3蓝4紫5橙6红"
// @Param applicableType query string false "facets.applicableTypes中的原字段类型键"
// @Param ids query string false "最多100个逗号分隔的精确资源ID"
// @Param page query integer false "页码，从1开始"
// @Param pageSize query integer false "每页条目数，1至100"
// @Success 200 {object} model.AdminItemsResponse
// @Router /admin/api/items [get]
func AdminItemsHandler(c *gin.Context) {
	page, size, ok := adminPage(c)
	req := model.AdminItemsRequest{
		Q: strings.TrimSpace(c.Query("q")), Category: strings.TrimSpace(c.Query("category")),
		ApplicableType: strings.TrimSpace(c.Query("applicableType")), Page: page, PageSize: size,
	}
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
	// 筛选项沿实际启动目录校验，避免旧固定分类遗漏生活材料和精灵装备。
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
	adminSuccess(c, gameService.AdminCatalogueFiltered(req))
}

// AdminPlayersHandler 查询有限角色索引。
// @Summary 查询角色
// @Tags Admin
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /admin/api/players [get]
func AdminPlayersHandler(c *gin.Context) {
	if len(c.Query("q")) > 128 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, e := service.AdminPlayers(c.Request.Context(), c.Query("q"))
	if e != nil {
		adminError(c, e)
		return
	}
	adminSuccess(c, gin.H{"items": out})
}

// AdminActivitiesHandler 返回实际配置与原活动登记。
// @Summary 活动配置目录
// @Tags Admin
// @Produce json
// @Success 200 {array} model.AdminActivityDTO
// @Router /admin/api/activities [get]
func AdminActivitiesHandler(c *gin.Context) {
	out, e := service.AdminActivities(c.Request.Context())
	if e != nil {
		adminError(c, e)
		return
	}
	adminSuccess(c, gin.H{"items": out})
}

// AdminActivityUpdateHandler 按版本更新运营配置及启停。
// @Summary 保存活动配置
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminActivityUpdateRequest true "配置"
// @Success 200 {object} model.AdminActivityDTO
// @Router /admin/api/activities/{id} [put]
func AdminActivityUpdateHandler(c *gin.Context) {
	var req model.AdminActivityUpdateRequest
	if len(c.Param("id")) > 200 || c.ShouldBindJSON(&req) != nil || len(req.Version) != 64 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, e := gameService.AdminUpdateActivity(c.Request.Context(), adminPrincipal(c), c.Param("id"), req)
	if e != nil {
		adminError(c, e)
		return
	}
	adminSuccess(c, out)
}

// AdminActivityCreateHandler 复制新运营期，保留原快照。
// @Summary 创建新活动期
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminActivityCreateRequest true "新一期"
// @Success 200 {object} model.AdminActivityDTO
// @Router /admin/api/activities [post]
func AdminActivityCreateHandler(c *gin.Context) {
	var req model.AdminActivityCreateRequest
	if c.ShouldBindJSON(&req) != nil || len(req.TemplateID) > 200 || len(req.NewID) > 64 || strings.TrimSpace(req.NewID) != req.NewID || strings.ContainsAny(req.NewID, "/\\\r\n\x00") {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, e := gameService.AdminCreateActivity(c.Request.Context(), adminPrincipal(c), req)
	if e != nil {
		adminError(c, e)
		return
	}
	adminSuccess(c, out)
}

// AdminAnnouncementsHandler 查询保存的公告。
// @Summary 公告列表
// @Tags Admin
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /admin/api/announcements [get]
func AdminAnnouncementsHandler(c *gin.Context) {
	out, e := service.AdminAnnouncements(c.Request.Context(), false)
	if e != nil {
		adminError(c, e)
		return
	}
	adminSuccess(c, gin.H{"items": out})
}

// AdminAnnouncementSaveHandler 保存纯文本公告和发布窗口。
// @Summary 保存公告
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminAnnouncementRequest true "公告"
// @Success 200 {object} map[string]interface{}
// @Router /admin/api/announcements [post]
func AdminAnnouncementSaveHandler(c *gin.Context) {
	id, ok := adminID(c)
	var req model.AdminAnnouncementRequest
	if !ok || c.ShouldBindJSON(&req) != nil || len([]rune(req.Title)) > 80 || len(req.Content) > 32000 || strings.TrimSpace(req.Title) == "" || req.StartAt < 0 || req.EndAt < 0 || (req.EndAt > 0 && req.EndAt <= req.StartAt) || (id > 0 && len(req.Version) != 64) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, e := service.AdminSaveAnnouncement(c.Request.Context(), adminPrincipal(c), id, req)
	if e != nil {
		adminError(c, e)
		return
	}
	adminSuccess(c, out)
}

// AdminAuditHandler 查询已提交运营操作。
// @Summary 操作审计
// @Tags Admin
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /admin/api/audit [get]
func AdminAuditHandler(c *gin.Context) {
	page, size, ok := adminPage(c)
	if !ok {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, e := service.AdminAuditPage(c.Request.Context(), page, size)
	if e != nil {
		adminError(c, e)
		return
	}
	adminSuccess(c, out)
}

// AdminGrantHandler 经原邮件投递有原因且幂等的附件。
// @Summary 发放道具附件
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminGrantRequest true "投递"
// @Success 200 {object} model.AdminGrantResponse
// @Router /admin/api/grants [post]
func AdminGrantHandler(c *gin.Context) {
	var req model.AdminGrantRequest
	if c.ShouldBindJSON(&req) != nil || req.PlayerID < 1 || req.Amount < 1 || req.Amount > 1000000 || len(req.ResourceID) > 64 || strings.ContainsAny(req.ResourceID, ",; \r\n\x00") || len(req.Reason) > 1000 || strings.TrimSpace(req.Reason) == "" || len(req.RequestID) != 36 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, e := gameService.AdminGrant(c.Request.Context(), adminPrincipal(c), req)
	if e != nil {
		adminError(c, e)
		return
	}
	adminSuccess(c, out)
}

// AdminUsersHandler 返回管理员可见的后台账号目录。
// @Summary 后台账号列表
// @Tags Admin
// @Produce json
// @Success 200 {array} model.AdminUserDTO
// @Router /admin/api/users [get]
func AdminUsersHandler(c *gin.Context) {
	out, e := service.AdminListUsers(c.Request.Context(), adminPrincipal(c))
	if e != nil {
		adminError(c, e)
		return
	}
	adminSuccess(c, gin.H{"items": out})
}

// AdminUserCreateHandler 创建具有固定角色的独立后台账号。
// @Summary 创建后台账号
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} model.AdminUserDTO
// @Router /admin/api/users [post]
func AdminUserCreateHandler(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if c.ShouldBindJSON(&req) != nil || !service.AdminRoleValid(req.Role) || len(req.Username) > 64 || len(req.Password) < 12 || len(req.Password) > 72 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, e := service.AdminCreateUser(c.Request.Context(), adminPrincipal(c), req.Username, req.Password, req.Role)
	if e != nil {
		adminError(c, e)
		return
	}
	adminSuccess(c, out)
}

// AdminUserRoleHandler 更新角色并撤销旧会话。
// @Summary 修改后台角色
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} map[string]bool
// @Router /admin/api/users/{id}/role [put]
func AdminUserRoleHandler(c *gin.Context) {
	id, ok := adminID(c)
	var req struct {
		Role string `json:"role"`
	}
	if !ok || c.ShouldBindJSON(&req) != nil || !service.AdminRoleValid(req.Role) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if e := service.AdminUpdateRole(c.Request.Context(), adminPrincipal(c), id, req.Role); e != nil {
		adminError(c, e)
		return
	}
	adminSuccess(c, gin.H{"ok": true})
}

// AdminUserPasswordHandler 修改口令并撤销该账号旧会话。
// @Summary 修改后台口令
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} map[string]bool
// @Router /admin/api/users/{id}/password [put]
func AdminUserPasswordHandler(c *gin.Context) {
	id, ok := adminID(c)
	var req struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if !ok || c.ShouldBindJSON(&req) != nil || len(req.NewPassword) < 12 || len(req.NewPassword) > 72 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if e := service.AdminChangePassword(c.Request.Context(), adminPrincipal(c), id, req.CurrentPassword, req.NewPassword); e != nil {
		adminError(c, e)
		return
	}
	adminSuccess(c, gin.H{"ok": true})
}

// PublicAnnouncementsHandler 向原游戏公告页提供转义后的纯文本内容。
// @Summary 游戏公告页
// @Tags Game
// @Produce html
// @Success 200 {string} string
// @Router /notices [get]
func PublicAnnouncementsHandler(c *gin.Context) {
	rows, e := service.AdminAnnouncements(c.Request.Context(), true)
	if e != nil {
		c.Status(500)
		return
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'")
	// 内容直接铺在客户端原公告框体内，保留原背景、标题栏和关闭按钮。
	t := template.Must(template.New("notices").Parse(`<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>游戏公告</title><style>html,body{margin:0;background:transparent!important;color:#343b40}body{font:16px/1.75 "Microsoft YaHei",sans-serif;padding:12px 18px}article{margin:0;padding:8px 0 16px}article+article{border-top:1px solid rgba(103,111,113,.3);padding-top:16px}h2{font-size:19px;font-weight:600;color:#246c92;margin:0 0 10px;padding:0 0 7px;border-bottom:2px solid #d5bd64}p{margin:0;white-space:pre-wrap;overflow-wrap:anywhere}.empty{text-align:center;padding:36px 0;color:#667176}</style>{{range .}}<article><h2>{{.Title}}</h2><p>{{.Content}}</p></article>{{else}}<p class="empty">暂无公告</p>{{end}}</html>`))
	_ = t.Execute(c.Writer, rows)
}
