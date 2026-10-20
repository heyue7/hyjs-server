package handler

import (
	"math"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
)

// RegisterAdminGuildRewardRoutes attaches independent reward publication endpoints.
func RegisterAdminGuildRewardRoutes(a *gin.RouterGroup) {
	a.GET("/guild-rewards", AdminGuildRewardsHandler)
	a.PUT("/guild-rewards/:kind/:id", AdminGuildRewardSaveHandler)
	a.POST("/guild-rewards/:kind", AdminGuildRewardCreateHandler)
}

// AdminGuildRewardsHandler returns current reward publications.
// @Summary 公会奖励设置
// @Description 查看战后、周和月奖励及可编辑状态
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} model.AdminGuildRewardsResponse
// @Router /admin/api/guild-rewards [get]
func AdminGuildRewardsHandler(c *gin.Context) {
	out, err := gameService.AdminGuildRewards(c.Request.Context(), adminPrincipal(c))
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

func adminGuildRewardRequestValid(r model.AdminGuildRewardSaveRequest, kind, id string, create bool) bool {
	if r.Kind != kind || strings.TrimSpace(r.Reason) == "" || len(r.Reason) > 500 || r.Version == "" || strings.TrimSpace(r.Version) != r.Version || len(r.Version) > 64 || strings.TrimSpace(r.Source) == "" || len(r.Source) > 2048 || create && r.RowVersion != "" || !create && len(r.RowVersion) != 64 {
		return false
	}
	resources := [][]model.AdminResourceAmount{r.ParticipationRewards, r.VictoryRewards, r.TransportMVPRewards, r.TaskRewards}
	for _, rank := range r.Ranks {
		resources = append(resources, rank.Rewards)
	}
	for _, list := range resources {
		if len(list) > 100 {
			return false
		}
		seen := map[string]bool{}
		for _, reward := range list {
			if reward.ResourceID == "" || len(reward.ResourceID) > 128 || strings.ContainsAny(reward.ResourceID, ",; \r\n") || reward.Amount < 1 || reward.Amount > math.MaxInt32 || seen[reward.ResourceID] {
				return false
			}
			seen[reward.ResourceID] = true
		}
	}
	switch kind {
	case "settlement":
		if create || !strings.Contains("ABCD", r.Group) || len(r.Group) != 1 || r.Group != id || r.WeekStart != "" || r.Month != "" || len(r.Ranks) > 0 || r.MemberTitle || r.ChairmanTitle || len(r.PersonalMissionsJSON) > 4096 || r.FactoryMissionPrestige < 0 || r.WinPrestige < 0 || r.LosePrestige < 0 {
			return false
		}
		return !r.Enabled || len(r.ParticipationRewards) > 0 && len(r.VictoryRewards) > 0
	case "weekly", "monthly":
		if len(r.ParticipationRewards)+len(r.VictoryRewards)+len(r.TransportMVPRewards)+len(r.TaskRewards) > 0 || r.PersonalMissionsJSON != "" || r.FactoryMissionPrestige != 0 || r.WinPrestige != 0 || r.LosePrestige != 0 {
			return false
		}
		maxRank := int32(1000)
		if kind == "weekly" {
			at, err := time.Parse("2006-01-02", r.WeekStart)
			if err != nil || at.Weekday() != time.Monday || len(r.Group) != 1 || !strings.Contains("ABCD", r.Group) || r.Month != "" || r.MemberTitle || r.ChairmanTitle || !create && id != r.WeekStart {
				return false
			}
			maxRank = 20
		} else {
			at, err := time.Parse("2006-01", r.Month)
			if err != nil || at.Format("2006-01") != r.Month || r.Group != "" || r.WeekStart != "" || !create && id != r.Month {
				return false
			}
		}
		if len(r.Ranks) > int(maxRank) || kind == "weekly" && r.Enabled && len(r.Ranks) == 0 {
			return false
		}
		seen := map[int32]bool{}
		for _, rank := range r.Ranks {
			if rank.Rank < 1 || rank.Rank > maxRank || seen[rank.Rank] || len(rank.Rewards) == 0 {
				return false
			}
			seen[rank.Rank] = true
		}
		return true
	}
	return false
}

// AdminGuildRewardSaveHandler updates one reward publication.
// @Summary 保存公会奖励
// @Description 逐项保存并核对读取版本，已开始的周月奖励只读
// @Tags Admin
// @Accept json
// @Produce json
// @Param kind path string true "settlement、weekly、monthly"
// @Param id path string true "分组、周一日期或月份"
// @Param body body model.AdminGuildRewardSaveRequest true "奖励配置及修改原因"
// @Success 200 {object} model.AdminGuildRewardRow
// @Router /admin/api/guild-rewards/{kind}/{id} [put]
func AdminGuildRewardSaveHandler(c *gin.Context) { adminSaveGuildReward(c, false) }

// AdminGuildRewardCreateHandler creates a future weekly or monthly publication.
// @Summary 新建未来公会奖励
// @Description 新建未来周或月奖配置，启用由运营明确选择
// @Tags Admin
// @Accept json
// @Produce json
// @Param kind path string true "weekly、monthly"
// @Param body body model.AdminGuildRewardSaveRequest true "奖励配置及修改原因"
// @Success 200 {object} model.AdminGuildRewardRow
// @Router /admin/api/guild-rewards/{kind} [post]
func AdminGuildRewardCreateHandler(c *gin.Context) { adminSaveGuildReward(c, true) }

func adminSaveGuildReward(c *gin.Context, create bool) {
	var req model.AdminGuildRewardSaveRequest
	if c.ShouldBindJSON(&req) != nil || !adminGuildRewardRequestValid(req, c.Param("kind"), c.Param("id"), create) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminSaveGuildReward(c.Request.Context(), adminPrincipal(c), req, create)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
