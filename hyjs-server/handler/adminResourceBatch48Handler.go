package handler

import (
	"encoding/hex"
	"math"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"hyjs-server/constUtil"
	"hyjs-server/model"
)

func adminBatchIdentity(requestID, reason string) bool {
	_, err := uuid.Parse(requestID)
	return err == nil && len(requestID) == 36 && strings.TrimSpace(reason) != "" && len(reason) <= 1000
}

func adminBatchResourceValid(id string, amount int64) bool {
	return id != "" && len(id) <= 128 && !strings.ContainsAny(id, ",; \r\n\x00") && amount > 0 && amount <= math.MaxInt32
}

// AdminBatchGrantHandler freezes a complete resource batch as one reward attachment.
// @Summary 批量发放道具
// @Description 为一个角色发放最多20种道具；同一请求重试只生成一个领奖附件
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminBatchGrantRequest true "发放清单"
// @Success 200 {object} model.AdminGrantResponse
// @Router /admin/api/grants/batch [post]
func AdminBatchGrantHandler(c *gin.Context) {
	var req model.AdminBatchGrantRequest
	if c.ShouldBindJSON(&req) != nil || req.PlayerID < 1 || !adminBatchIdentity(req.RequestID, req.Reason) || len(req.Resources) < 1 || len(req.Resources) > constUtil.AdminResourceBatchMaximum {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	seen := map[string]bool{}
	for _, resource := range req.Resources {
		if !adminBatchResourceValid(resource.ResourceID, resource.Amount) || seen[resource.ResourceID] {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
		seen[resource.ResourceID] = true
	}
	out, err := gameService.AdminBatchGrant(c.Request.Context(), adminPrincipal(c), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminInventoryBatchHandler adds the entire list with version and retry protection.
// @Summary 批量添加背包资源
// @Description 同一角色的整批添加原子提交，保留现有背包保护及审计
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path integer true "玩家ID"
// @Param body body model.AdminInventoryBatchRequest true "添加清单"
// @Success 200 {object} model.AdminPlayerInventoryMutationResponse
// @Router /admin/api/game-players/{id}/inventory/batch [put]
func AdminInventoryBatchHandler(c *gin.Context) {
	id, ok := adminID(c)
	var req model.AdminInventoryBatchRequest
	if !ok || id < 1 || c.ShouldBindJSON(&req) != nil || !adminBatchIdentity(req.RequestID, req.Reason) || len(req.Version) != 64 || len(req.Resources) < 1 || len(req.Resources) > constUtil.AdminResourceBatchMaximum {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if _, err := hex.DecodeString(req.Version); err != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	seen := map[string]bool{}
	for _, resource := range req.Resources {
		instance := resource.Category == "warrior" || resource.Category == "equipment" || resource.Category == "accessory" || resource.Category == "pet"
		if !adminInventoryCategoryValid(resource.Category) || !adminBatchResourceValid(resource.ResourceID, resource.Amount) || seen[resource.ResourceID] || instance && resource.Amount > 100 {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
		seen[resource.ResourceID] = true
	}
	out, err := gameService.AdminInventoryBatch(c.Request.Context(), adminPrincipal(c), id, req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
