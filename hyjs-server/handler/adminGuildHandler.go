package handler

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
)

// RegisterAdminGuildRoutes attaches global guild settings and realm guild management.
func RegisterAdminGuildRoutes(a *gin.RouterGroup) {
	a.GET("/guild-settings", AdminGuildSettingsHandler)
	a.PUT("/guild-settings", AdminGuildSettingsSaveHandler)
	a.GET("/guilds", AdminGuildsHandler)
	a.GET("/guilds/:id", AdminGuildHandler)
	a.PUT("/guilds/:id", AdminGuildUpdateHandler)
	a.PUT("/guilds/:id/members/:playerId", AdminGuildMemberUpdateHandler)
	a.POST("/guilds/:id/members", AdminGuildMemberAddHandler)
	a.DELETE("/guilds/:id/members/:playerId", AdminGuildMemberRemoveHandler)
	a.DELETE("/guilds/:id", AdminGuildDissolveHandler)
}

// AdminGuildSettingsHandler reads effective global guild overrides.
// @Summary 全局公会设置
// @Description 返回实际消费参数、原分级表与独立门禁；不写原静态表
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} model.AdminGuildSettingDTO
// @Router /admin/api/guild-settings [get]
func AdminGuildSettingsHandler(c *gin.Context) {
	out, err := gameService.AdminGuildSettings(c.Request.Context(), adminPrincipal(c))
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminGuildSettingsSaveHandler validates whitelist fields and optimistic version.
// @Summary 保存全局公会设置
// @Description 只更新后台 singleton 覆盖，旧祈祷/科技/战争快照不回写
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminGuildSettingSaveRequest true "全局设置"
// @Success 200 {object} model.AdminGuildSettingDTO
// @Router /admin/api/guild-settings [put]
func AdminGuildSettingsSaveHandler(c *gin.Context) {
	var req model.AdminGuildSettingSaveRequest
	if c.ShouldBindJSON(&req) != nil || len(req.Version) != 64 || strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 500 || len(req.Fields) > 100 || len(req.UnionLevels) > 20 || len(req.UnionTech) > 256 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	seen := map[string]bool{}
	for _, field := range req.Fields {
		if field.Key == "" || len(field.Key) > 64 || len(field.Value) > 2048 || seen[field.Key] {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
		seen[field.Key] = true
	}
	if req.Policies != nil {
		if p := req.Policies.Guild; p != nil && (p.Source != "" && len(p.Source) > 1024) {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
		if p := req.Policies.DonateBox; p != nil && len(p.Tiers) != 0 {
			if len(p.Tiers) != 4 {
				adminError(c, constUtil.ErrAdminInput)
				return
			}
			var previous int32
			for _, tier := range p.Tiers {
				if tier.Threshold <= previous || len(tier.Reward) == 0 || len(tier.Reward) > 2048 {
					adminError(c, constUtil.ErrAdminInput)
					return
				}
				previous = tier.Threshold
			}
		}
		if p := req.Policies.DonationBadge; p != nil && (p.CoinCost < 0 || p.BadgeAmount < 0 || p.DailyLimit < 0 || p.DailyLimit > 1000) {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
		if p := req.Policies.Prayer; p != nil && (len(p.Version) > 128 || len(p.PoolJSON) > 65536) {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
		if p := req.Policies.Tech; p != nil && (len(p.Version) > 128 || len(p.DirectoryJSON) > 65536) {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
		if p := req.Policies.UnionFight; p != nil {
			if f := p.Field; f != nil && (f.PreparationMillis < 0 || f.PreparationMillis > 86400000 || len(f.Source) > 1024 || f.GuardHPMode != "" && f.GuardHPMode != "uniform" && f.GuardHPMode != "template") {
				adminError(c, constUtil.ErrAdminInput)
				return
			}
			if ch := p.Chat; ch != nil && (ch.TalkCooldownMillis < 0 || ch.TalkCooldownMillis > 3600000) {
				adminError(c, constUtil.ErrAdminInput)
				return
			}
			if len(p.PlayerBattle) > 3 {
				adminError(c, constUtil.ErrAdminInput)
				return
			}
			seenPolicies := map[int64]bool{}
			for _, policy := range p.PlayerBattle {
				if seenPolicies[policy.ID] || policy.ID != 1 && policy.ID != constUtil.P13UFPlayerPartialPolicyID && policy.ID != constUtil.P13UFPlayerPartyPolicyID {
					adminError(c, constUtil.ErrAdminInput)
					return
				}
				seenPolicies[policy.ID] = true
			}
		}
	}
	out, err := gameService.AdminSaveGuildSettings(c.Request.Context(), adminPrincipal(c), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminGuildsHandler lists current-realm guilds with validated ownership.
// @Summary 区服公会列表
// @Description 仅返回成员角色全部归属当前选中区服的公会
// @Tags Admin
// @Accept json
// @Produce json
// @Param q query string false "公会名或编号"
// @Param page query int false "页码"
// @Param pageSize query int false "每页数量"
// @Success 200 {object} model.AdminGuildListResponse
// @Router /admin/api/guilds [get]
func AdminGuildsHandler(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if len(q) > 64 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	page, size, ok := adminPage(c)
	if !ok {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminGuilds(c.Request.Context(), q, page, size)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminGuildHandler reads one current-realm guild.
// @Summary 公会详情
// @Description 返回共享数值、建筑、文本、旗帜和成员投影
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path string true "公会编号"
// @Success 200 {object} model.AdminGuildDetail
// @Router /admin/api/guilds/{id} [get]
func AdminGuildHandler(c *gin.Context) {
	id := c.Param("id")
	if id == "" || len(id) > 64 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminGuildDetail(c.Request.Context(), id)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminGuildUpdateHandler edits shared guild values under optimistic version.
// @Summary 保存公会数值
// @Description 编辑等级、资源、积分、建筑开关、名称/文本和旗帜
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path string true "公会编号"
// @Param body body model.AdminGuildUpdateRequest true "公会数值"
// @Success 200 {object} model.AdminGuildDetail
// @Router /admin/api/guilds/{id} [put]
func AdminGuildUpdateHandler(c *gin.Context) {
	var req model.AdminGuildUpdateRequest
	id := c.Param("id")
	if c.ShouldBindJSON(&req) != nil || id == "" || len(id) > 64 || len(req.Version) != 64 || strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 500 || req.Name != strings.TrimSpace(req.Name) || req.Name == "" || len(req.Name) > 64 || len(req.Notice) > 4096 || len(req.Declaration) > 4096 || req.Level < 1 || req.Flag < 0 || req.Coin < 0 || req.Badge < 0 || req.Exp < 0 || req.DonateScore < 0 || req.ActiveScore < 0 || req.HeroStatueOpen < 0 || req.HeroStatueOpen > 1 || req.GuardTreeOpen < 0 || req.GuardTreeOpen > 1 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminUpdateGuild(c.Request.Context(), adminPrincipal(c), id, req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminGuildMemberUpdateHandler edits contribution, daily donation and role.
// @Summary 保存公会成员
// @Description 会长转让原子保持唯一会长，不能把现任会长直接降为成员
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path string true "公会编号"
// @Param playerId path int true "角色编号"
// @Param body body model.AdminGuildMemberUpdateRequest true "成员数值"
// @Success 200 {object} model.AdminGuildDetail
// @Router /admin/api/guilds/{id}/members/{playerId} [put]
func AdminGuildMemberUpdateHandler(c *gin.Context) {
	var req model.AdminGuildMemberUpdateRequest
	id := c.Param("id")
	playerID, err := strconv.ParseInt(c.Param("playerId"), 10, 64)
	if c.ShouldBindJSON(&req) != nil || id == "" || len(id) > 64 || err != nil || playerID < 1 || len(req.Version) != 64 || strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 500 || req.Contribution < 0 || req.TodayCoin < 0 || req.TodayBadge < 0 || req.Role < constUtil.P13GuildMember || req.Role > constUtil.P13GuildChairman {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if req.SpendableContribution != nil && (req.ExpectedSpendableContribution == nil || *req.SpendableContribution < 0 || *req.ExpectedSpendableContribution < 0) || req.SpendableContribution == nil && req.ExpectedSpendableContribution != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminUpdateGuildMember(c.Request.Context(), adminPrincipal(c), id, playerID, req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminGuildMemberAddHandler adds an existing same-realm player.
// @Summary 添加公会成员
// @Description 复用原开放等级、退会冷却、人数和已有公会资格
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path string true "公会编号"
// @Param body body model.AdminGuildMemberAddRequest true "成员"
// @Success 200 {object} model.AdminGuildDetail
// @Router /admin/api/guilds/{id}/members [post]
func AdminGuildMemberAddHandler(c *gin.Context) {
	var req model.AdminGuildMemberAddRequest
	id := c.Param("id")
	if c.ShouldBindJSON(&req) != nil || id == "" || len(id) > 64 || req.PlayerID < 1 || len(req.Version) != 64 || strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 500 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminAddGuildMember(c.Request.Context(), adminPrincipal(c), id, req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminGuildMemberRemoveHandler removes a member through the original leave flow.
// @Summary 移除公会成员
// @Description 保留离会冷却、历史回执、祈祷和战场结算，提交后撤销房间/战场资格
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path string true "公会编号"
// @Param playerId path int true "角色编号"
// @Param body body model.AdminGuildMemberRemoveRequest true "版本与原因"
// @Success 200 {object} model.AdminGuildDetail
// @Router /admin/api/guilds/{id}/members/{playerId} [delete]
func AdminGuildMemberRemoveHandler(c *gin.Context) {
	var req model.AdminGuildMemberRemoveRequest
	id := c.Param("id")
	playerID, err := strconv.ParseInt(c.Param("playerId"), 10, 64)
	if c.ShouldBindJSON(&req) != nil || id == "" || len(id) > 64 || err != nil || playerID < 1 || len(req.Version) != 64 || strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 500 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminRemoveGuildMember(c.Request.Context(), adminPrincipal(c), id, playerID, req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminGuildDissolveHandler validates exact name and version before dissolution.
// @Summary 解散公会
// @Description 按原离会流程移除成员并撤销房间/战场资格，保留历史记录与奖励
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path string true "公会编号"
// @Param body body model.AdminGuildDissolveRequest true "确认信息"
// @Success 200 {object} model.AdminGuildDissolveResponse
// @Router /admin/api/guilds/{id} [delete]
func AdminGuildDissolveHandler(c *gin.Context) {
	var req model.AdminGuildDissolveRequest
	id := c.Param("id")
	if c.ShouldBindJSON(&req) != nil || id == "" || len(id) > 64 || len(req.Version) != 64 || strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 500 || req.ConfirmName == "" || len(req.ConfirmName) > 64 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminDissolveGuild(c.Request.Context(), adminPrincipal(c), id, req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
