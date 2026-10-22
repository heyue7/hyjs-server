package handler

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/service"
)

// RegisterAdminFieldRoutes uses the authenticated CSRF-protected operations group.
func RegisterAdminFieldRoutes(a *gin.RouterGroup) {
	a.GET("/fields/templates", AdminFieldListHandler)
	a.POST("/fields/templates", AdminFieldSaveHandler)
	a.DELETE("/fields/templates", AdminFieldDeleteHandler)
	a.GET("/fields/deployments", AdminFieldDeploymentsHandler)
	a.PUT("/fields/deployments", AdminFieldDeployHandler)
	a.GET("/fields/channels", AdminFieldChannelsHandler)
	a.PUT("/fields/channels", AdminFieldChannelSaveHandler)
}

// AdminFieldListHandler returns global reusable map-resource schemes.
// @Summary 野外全局方案
// @Description 返回包含矿物、钓鱼、小怪和首领奖励的完整地图资源组合
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} model.AdminFieldListResponse
// @Router /admin/api/fields/templates [get]
func AdminFieldListHandler(c *gin.Context) {
	v, err := gameService.AdminFields(c.Request.Context(), adminPrincipal(c))
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, v)
}

// AdminFieldSaveHandler validates original resource references before an audited save.
// @Summary 保存野外全局方案
// @Description 已部署区服保留原快照，修改后需重新部署
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminFieldSaveRequest true "完整方案"
// @Success 200 {object} model.AdminFieldTemplateDTO
// @Router /admin/api/fields/templates [post]
func AdminFieldSaveHandler(c *gin.Context) {
	var req model.AdminFieldSaveRequest
	if c.ShouldBindJSON(&req) != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len([]rune(req.Name)) > 80 || len(req.ID) > 64 || req.ID != "" && len(req.Version) != 64 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	cfg := &req.Configuration
	original, err := gameService.AdminFieldOriginalReference(cfg.Scene)
	if err != nil || cfg.MinLevel < 1 || cfg.MinLevel > 1000 || cfg.SceneResource != original.SceneResource || cfg.BattleResource != original.BattleResource || cfg.MineCount < 0 || cfg.MineCount > int64(len(cfg.MinePoints)) || cfg.FishCount < 0 || cfg.FishCount > int64(len(cfg.FishPoints)) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	// Candidate identities and coordinates are restricted to actual client resources.
	gatherIDs, monsterIDs := map[string]bool{}, map[string]bool{}
	for _, g := range original.GatherGroups {
		gatherIDs[g.ID] = true
	}
	for _, g := range original.MonsterGroups {
		monsterIDs[g.ID] = true
	}
	for _, spec := range []struct {
		points, source []model.AdminFieldPoint
		monster        bool
	}{{cfg.MinePoints, original.MinePoints, false}, {cfg.FishPoints, original.FishPoints, false}, {cfg.MonsterPoints, original.MonsterPoints, true}} {
		allowed := map[string]string{}
		for _, p := range spec.source {
			allowed[p.ID] = p.Point
		}
		seen := map[string]bool{}
		for _, p := range spec.points {
			if seen[p.ID] || allowed[p.ID] == "" || allowed[p.ID] != p.Point || len(p.Choices) < 1 || len(p.Choices) > 100 {
				adminError(c, constUtil.ErrAdminInput)
				return
			}
			seen[p.ID] = true
			weights := map[string]bool{}
			var total int64
			for _, w := range p.Choices {
				valid := gatherIDs[w.ID]
				if spec.monster {
					valid = monsterIDs[w.ID]
				} else if strings.HasPrefix(p.ID, "mine_") {
					valid = valid && strings.HasPrefix(w.ID, "group_mine_")
				} else {
					valid = valid && strings.HasPrefix(w.ID, "group_fish_")
				}
				if !valid || weights[w.ID] || w.Weight < 1 || w.Weight > 1000000 {
					adminError(c, constUtil.ErrAdminInput)
					return
				}
				weights[w.ID] = true
				total += w.Weight
			}
			if total > 2147483647 {
				adminError(c, constUtil.ErrAdminInput)
				return
			}
		}
	}
	seen := map[string]bool{}
	for _, g := range cfg.GatherGroups {
		if !gatherIDs[g.ID] || seen[g.ID] || g.Level < 1 || g.Level > 6 || g.GatherMS < 1 || g.GatherMS > constUtil.P8GatherMaxDurationMS || g.CooldownMS < 1 || g.CooldownMS > constUtil.P8GatherMaxDurationMS || g.Times < 1 || g.Times > 100 || g.Fatigue < 1 || g.Fatigue > 200 || !adminFieldValidDrops(g.Drops, "gather") {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
		seen[g.ID] = true
	}
	for id := range gatherIDs {
		if !seen[id] {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
	}
	seen = map[string]bool{}
	originalMonsters := map[string]model.AdminFieldMonsterGroup{}
	for _, g := range original.MonsterGroups {
		originalMonsters[g.ID] = g
	}
	for _, g := range cfg.MonsterGroups {
		source := originalMonsters[g.ID]
		if !monsterIDs[g.ID] || seen[g.ID] || g.HeroID != source.HeroID || g.Level != source.Level || g.CooldownMS < 1 || g.CooldownMS > 86400000 || !adminFieldValidDrops(g.Drops, "monster") || !adminFieldCombatMembersValid(g.Members, source.Members) {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
		seen[g.ID] = true
	}
	for id := range monsterIDs {
		if !seen[id] {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
	}
	// 普通首领沿用本地图原候选与组号；旧方案缺省该字段时继承原表。
	if cfg.OrdinaryBossGroups != nil {
		sourceGroups := map[string]model.AdminFieldBossGroup{}
		for _, g := range original.OrdinaryBossGroups {
			sourceGroups[fmt.Sprintf("%s:%d", g.ID, g.GroupIndex)] = g
		}
		if len(cfg.OrdinaryBossGroups) != len(sourceGroups) {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
		seen = map[string]bool{}
		for _, g := range cfg.OrdinaryBossGroups {
			key := fmt.Sprintf("%s:%d", g.ID, g.GroupIndex)
			source, ok := sourceGroups[key]
			if !ok || seen[key] || g.HeroID != source.HeroID || g.RebornHours < 1 || g.RebornHours > constUtil.AdminFieldBossMaxRebornHours || !adminFieldCombatMembersValid(g.Members, source.Members) || !adminFieldValidBossDrops(g.Drops) || !adminFieldValidBossDrops(g.KillDrops) {
				adminError(c, constUtil.ErrAdminInput)
				return
			}
			seen[key] = true
		}
	}
	if cfg.WorldBossLevel != nil && (*cfg.WorldBossLevel < 1 || *cfg.WorldBossLevel > constUtil.AdminFieldWorldBossMaxLevel) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if !adminFieldCombatMembersValid(cfg.WorldBossMembers, original.WorldBossMembers) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if len(cfg.BossRewards) != len(original.BossRewards) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	seen = map[string]bool{}
	for _, r := range cfg.BossRewards {
		key := strconv.FormatInt(r.Type, 10) + ":" + strconv.FormatInt(r.Level, 10)
		if seen[key] || r.Type < 0 || r.Type > 5 || r.Level < 1 || r.Level > 50 || len(r.Drops) < 1 || len(r.Drops) > 32 || r.Weighted && (r.Level < 30 || r.Type != 4 && r.Type != 5) {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
		seen[key] = true
		for _, d := range r.Drops {
			if !gameService.AdminFieldBossDropAllowed(d.ResourceID) || len(d.ResourceID) > 128 || len(d.Quantities) < 1 || len(d.Quantities) > 32 || !r.Weighted && len(d.Quantities) != 1 {
				adminError(c, constUtil.ErrAdminInput)
				return
			}
			for _, q := range d.Quantities {
				if q.Amount < 0 || q.Amount > 2147483647 || !gameService.AdminFieldBossAmountAllowed(d.ResourceID, q.Amount) || q.Weight < 1 || q.Weight > 1000000 || !r.Weighted && q.Amount < 1 {
					adminError(c, constUtil.ErrAdminInput)
					return
				}
			}
		}
	}
	v, err := gameService.AdminSaveField(c.Request.Context(), adminPrincipal(c), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, v)
}
func adminFieldValidDrops(drops []model.AdminFieldDrop, kind string) bool {
	if len(drops) < 1 || len(drops) > 32 {
		return false
	}
	seen := map[string]bool{}
	for _, d := range drops {
		if seen[d.ResourceID] || !gameService.AdminFieldDropAllowed(d.ResourceID, kind) || len(d.Quantities) < 1 || len(d.Quantities) > 32 {
			return false
		}
		seen[d.ResourceID] = true
		amounts := map[int64]bool{}
		var total int64
		for _, q := range d.Quantities {
			if q.Amount < 0 || q.Amount > 2147483647 || q.Weight < 1 || q.Weight > 1000000 || amounts[q.Amount] {
				return false
			}
			amounts[q.Amount] = true
			total += q.Weight
		}
		if total > 2147483647 {
			return false
		}
	}
	return true
}

func adminFieldCombatMembersValid(members, source []model.AdminFieldCombatMember) bool {
	if members == nil {
		return true
	}
	if len(members) != len(source) {
		return false
	}
	allowed := map[string]model.AdminFieldCombatMember{}
	for _, member := range source {
		allowed[member.ID] = member
	}
	seen := map[string]bool{}
	for _, member := range members {
		original, ok := allowed[member.ID]
		if !ok || seen[member.ID] || member.HeroID != original.HeroID || member.Location != original.Location || member.GroupIndex != original.GroupIndex || !service.AdminFieldCombatMemberAllowed(member) {
			return false
		}
		seen[member.ID] = true
	}
	return true
}

func adminFieldValidBossDrops(drops []model.AdminFieldDrop) bool {
	if len(drops) < 1 || len(drops) > 32 {
		return false
	}
	seen := map[string]bool{}
	for _, d := range drops {
		if seen[d.ResourceID] || len(d.ResourceID) > 128 || !gameService.AdminFieldBossDropAllowed(d.ResourceID) || len(d.Quantities) < 1 || len(d.Quantities) > 32 {
			return false
		}
		seen[d.ResourceID] = true
		amounts := map[int64]bool{}
		var total, positive int64
		for _, q := range d.Quantities {
			if q.Amount < 0 || q.Amount > 2147483647 || amounts[q.Amount] || !gameService.AdminFieldBossAmountAllowed(d.ResourceID, q.Amount) || q.Weight < 0 || q.Weight > 1000000 {
				return false
			}
			amounts[q.Amount] = true
			total += q.Weight
			if q.Weight > 0 {
				positive++
			}
		}
		if total < 1 || total > 2147483647 || positive == 0 {
			return false
		}
	}
	return true
}

// AdminFieldDeleteHandler deletes an unused scheme with a current version.
// @Summary 删除未部署野外方案
// @Description 有区服使用的方案保留
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminFieldDeleteRequest true "方案及版本"
// @Success 200 {object} map[string]bool
// @Router /admin/api/fields/templates [delete]
func AdminFieldDeleteHandler(c *gin.Context) {
	var req model.AdminFieldDeleteRequest
	if c.ShouldBindJSON(&req) != nil || len(req.ID) > 64 || len(req.Version) != 64 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if err := gameService.AdminDeleteField(c.Request.Context(), adminPrincipal(c), req); err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, map[string]bool{"deleted": true})
}

// AdminFieldDeploymentsHandler returns one selected realm's deployment state.
// @Summary 野外区服部署
// @Description 每个区服单独选择完整资源组合
// @Tags Admin
// @Accept json
// @Produce json
// @Param serverId query int true "区服编号"
// @Success 200 {object} model.AdminFieldDeploymentsResponse
// @Router /admin/api/fields/deployments [get]
func AdminFieldDeploymentsHandler(c *gin.Context) {
	id, err := strconv.ParseInt(c.Query("serverId"), 10, 64)
	if err != nil || id < 1 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if err = service.AdminRequireGameServer(c.Request.Context(), id); err != nil {
		adminError(c, err)
		return
	}
	v, err := gameService.AdminFieldDeployments(c.Request.Context(), adminPrincipal(c), id)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, v)
}

// AdminFieldDeployHandler publishes one immutable scheme into one realm.
// @Summary 发布野外资源组合
// @Description 不会开放缺失客户端场景的地图，后继刷新使用新资源组合
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminFieldDeployRequest true "区服、原地图及全局方案"
// @Success 200 {object} model.AdminFieldDeploymentsResponse
// @Router /admin/api/fields/deployments [put]
func AdminFieldDeployHandler(c *gin.Context) {
	var req model.AdminFieldDeployRequest
	if c.ShouldBindJSON(&req) != nil || req.ServerID < 1 || len(req.TemplateID) > 64 || len(req.TemplateVersion) != 64 || req.Version != "" && len(req.Version) != 64 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if err := service.AdminRequireGameServer(c.Request.Context(), req.ServerID); err != nil {
		adminError(c, err)
		return
	}
	v, err := gameService.AdminDeployField(c.Request.Context(), adminPrincipal(c), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, v)
}

// AdminFieldChannelsHandler returns original line capacities and authorized online counts.
// @Summary 野外分线
// @Description 返回所选区服各地图的线路和在线人数
// @Tags Admin
// @Accept json
// @Produce json
// @Param serverId query int true "区服编号"
// @Success 200 {object} model.AdminFieldChannelsResponse
// @Router /admin/api/fields/channels [get]
func AdminFieldChannelsHandler(c *gin.Context) {
	id, err := strconv.ParseInt(c.Query("serverId"), 10, 64)
	if err != nil || id < 1 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if err = service.AdminRequireGameServer(c.Request.Context(), id); err != nil {
		adminError(c, err)
		return
	}
	result, err := gameService.AdminFieldChannels(c.Request.Context(), adminPrincipal(c), id)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, result)
}

// AdminFieldChannelSaveHandler validates and saves one map's original line limits.
// @Summary 保存野外分线
// @Description 不会关闭仍有在线角色的线路
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminFieldChannelSaveRequest true "分线设置"
// @Success 200 {object} model.AdminFieldChannelsResponse
// @Router /admin/api/fields/channels [put]
func AdminFieldChannelSaveHandler(c *gin.Context) {
	var req model.AdminFieldChannelSaveRequest
	if c.ShouldBindJSON(&req) != nil || req.ServerID < 1 || !constUtil.P8OriginalFieldScene(req.Scene) || req.LineCount < 1 || req.LineCount > 32 || req.MaxPlayers < 1 || req.MaxPlayers > 1000 || req.Version != "" && len(req.Version) != 64 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if !constUtil.P8PackagedFieldScene(req.Scene) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if err := service.AdminRequireGameServer(c.Request.Context(), req.ServerID); err != nil {
		adminError(c, err)
		return
	}
	result, err := gameService.AdminSaveFieldChannels(c.Request.Context(), adminPrincipal(c), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, result)
}
