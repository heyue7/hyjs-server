package handler

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"io"
	"math"
	"strconv"
	"strings"
)

// RegisterAdminPlayerInventoryRoutes 注册独立背包管理入口。
func RegisterAdminPlayerInventoryRoutes(a *gin.RouterGroup) {
	a.POST("/grants/batch", AdminBatchGrantHandler)
	a.PUT("/game-players/:id/inventory/batch", AdminInventoryBatchHandler)
	a.GET("/game-players/:id/inventory", AdminPlayerInventoryHandler)
	a.GET("/game-players/:id/inventory/catalogue", AdminPlayerInventoryCatalogueHandler)
	a.PUT("/game-players/:id/inventory", AdminPlayerInventoryMutationHandler)
}
func adminInventoryCategoryValid(category string) bool {
	switch category {
	case "warrior", "equipment", "accessory", "item", "gem", "cooking", "pet":
		return true
	}
	return false
}
func adminInventoryQuery(c *gin.Context) (int64, string, string, *int64, int64, int64, bool) {
	id, ok := adminID(c)
	page, size, valid := adminPage(c)
	category := c.Query("category")
	q := strings.TrimSpace(c.Query("q"))
	var quality *int64
	if raw := c.Query("quality"); raw != "" {
		v, e := strconv.ParseInt(raw, 10, 64)
		if e != nil || v < 0 || v > 6 {
			return 0, "", "", nil, 0, 0, false
		}
		quality = &v
	}
	return id, category, q, quality, page, size, ok && id > 0 && valid && size <= 100 && len(q) <= 200 && (category == "" || adminInventoryCategoryValid(category))
}

// AdminPlayerInventoryHandler 返回角色真实背包与全量分类统计。
// @Summary 玩家背包
// @Description 按名称、资源ID、GUID及统一品质筛选全部背包后分页
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path integer true "玩家ID"
// @Success 200 {object} model.AdminPlayerInventoryResponse
// @Router /admin/api/game-players/{id}/inventory [get]
func AdminPlayerInventoryHandler(c *gin.Context) {
	id, category, q, quality, page, size, ok := adminInventoryQuery(c)
	if !ok {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminPlayerInventory(c.Request.Context(), id, category, q, quality, page, size)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminPlayerInventoryCatalogueHandler 返回当前可实际添加的原生背包资源。
// @Summary 可添加背包目录
// @Description 目录分类匹配真实存储并排除货币、职业模板和精灵装备
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path integer true "玩家ID"
// @Success 200 {object} model.AdminPlayerInventoryCatalogueResponse
// @Router /admin/api/game-players/{id}/inventory/catalogue [get]
func AdminPlayerInventoryCatalogueHandler(c *gin.Context) {
	id, category, q, quality, page, size, ok := adminInventoryQuery(c)
	if !ok {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if _, err := gameService.AdminPlayerInventory(c.Request.Context(), id, "", "", nil, 1, 1); err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, gameService.AdminPlayerInventoryCatalogue(category, q, quality, page, size))
}

// AdminPlayerInventoryMutationHandler 校验有限的数值操作并原子记录审计。
// @Summary 修改玩家背包
// @Description 通过UUID重试和版本保护添加、更新或整行移除，移除无卖出返还
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path integer true "玩家ID"
// @Param body body model.AdminPlayerInventoryRequest true "背包操作"
// @Success 200 {object} model.AdminPlayerInventoryMutationResponse
// @Router /admin/api/game-players/{id}/inventory [put]
func AdminPlayerInventoryMutationHandler(c *gin.Context) {
	id, ok := adminID(c)
	var req model.AdminPlayerInventoryRequest
	rawBody, readErr := io.ReadAll(io.LimitReader(c.Request.Body, 16385))
	if readErr != nil || len(rawBody) > 16384 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	var rawRequest map[string]json.RawMessage
	if json.Unmarshal(rawBody, &rawRequest) != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if raw, exists := rawRequest["attributes"]; exists {
		var attrs map[string]json.RawMessage
		if json.Unmarshal(raw, &attrs) != nil {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
		for _, value := range attrs {
			if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				adminError(c, constUtil.ErrAdminInput)
				return
			}
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(rawBody))
	decoder.DisallowUnknownFields()
	if !ok || id < 1 || decoder.Decode(&req) != nil || len(req.Version) != 64 || !adminInventoryCategoryValid(req.Category) || strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 1000 || len(req.Key) > 128 || len(req.ResourceID) > 128 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	if parsed, err := uuid.Parse(req.RequestID); err != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	} else {
		req.RequestID = parsed.String()
	}
	if req.Amount != nil && (*req.Amount < 1 || *req.Amount > math.MaxInt32) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	switch req.Operation {
	case "add":
		if req.Key != "" || req.ResourceID == "" || req.Amount == nil || len(req.Attributes) != 0 || (req.Category == "warrior" || req.Category == "equipment" || req.Category == "accessory" || req.Category == "pet") && *req.Amount > 100 {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
	case "update":
		if req.Key == "" || req.ResourceID != "" || (req.Amount == nil) == (len(req.Attributes) == 0) || len(req.Attributes) > 32 {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
		for key, value := range req.Attributes {
			allowed := false
			switch req.Category {
			case "warrior":
				switch key {
				case "level", "exp", "talent", "training.hp", "training.att", "training.pdef", "training.mdef", "breached.level", "breached.exp", "awake.stage", "awake.fuseLevel", "awake.fuseExp":
					allowed = true
				}
			case "equipment":
				switch key {
				case "level", "refineLv", "refineExp", "star", "starExp", "forgeLevel":
					allowed = true
				}
			case "accessory":
				switch key {
				case "strengthLevel", "refineLevel", "currentExp":
					allowed = true
				}
			case "pet":
				switch key {
				case "level", "exp", "breached.level", "eat.level", "eat.exp":
					allowed = true
				}
				if strings.HasPrefix(key, "equip.") {
					parts := strings.Split(key, ".")
					allowed = len(parts) == 3 && parts[1] != "" && (parts[2] == "strengthLevel" || parts[2] == "advanceLevel" || parts[2] == "awakeLevel")
				}
			}
			if !allowed || len(key) > 160 || value < 0 || value > math.MaxInt32 {
				adminError(c, constUtil.ErrAdminInput)
				return
			}
		}
	case "remove":
		if req.Key == "" || req.ResourceID != "" || req.Amount != nil || len(req.Attributes) != 0 {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
	default:
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminUpdatePlayerInventory(c.Request.Context(), adminPrincipal(c), id, req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
