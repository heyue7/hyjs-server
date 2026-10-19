package handler

import (
	"encoding/hex"
	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"strings"
	"unicode/utf8"
)

// RegisterAdminCDKRoutes uses the existing session, realm and CSRF boundaries.
func RegisterAdminCDKRoutes(a *gin.RouterGroup) {
	a.GET("/cdk", AdminCDKListHandler)
	a.POST("/cdk", AdminCDKCreateHandler)
	a.PUT("/cdk/:id", AdminCDKUpdateHandler)
	a.GET("/cdk/:id/claims", AdminCDKClaimsHandler)
}

// AdminCDKListHandler lists policies without redeemable code values.
// @Summary CDK列表
// @Description 当前运行区服的CDK、次数和状态
// @Tags Admin
// @Accept json
// @Produce json
// @Param serverId query integer true "区服"
// @Param q query string false "名称或尾号"
// @Success 200 {object} model.AdminCDKListResponse
// @Router /admin/api/cdk [get]
func AdminCDKListHandler(c *gin.Context) {
	page, size, ok := adminPage(c)
	q := strings.TrimSpace(c.Query("q"))
	if !ok || len(q) > 128 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminCDKList(c.Request.Context(), adminPrincipal(c), q, page, size)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminCDKCreateHandler generates original gift codes atomically with operator-selected rewards.
// @Summary 生成CDK
// @Description 完整兑换码仅生成成功时返回，请及时保存
// @Tags Admin
// @Accept json
// @Produce json
// @Param body body model.AdminCDKCreateRequest true "奖励、次数及有效期"
// @Success 200 {object} model.AdminCDKCreateResponse
// @Router /admin/api/cdk [post]
func AdminCDKCreateHandler(c *gin.Context) {
	var req model.AdminCDKCreateRequest
	if c.ShouldBindJSON(&req) != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Reason = strings.TrimSpace(req.Reason)
	req.Code = strings.ToUpper(strings.TrimSpace(req.Code))
	req.Prefix = strings.ToUpper(strings.TrimSpace(req.Prefix))
	validASCII := func(v string) bool {
		for _, r := range v {
			if !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
				return false
			}
		}
		return true
	}
	if req.Name == "" || utf8.RuneCountInString(req.Name) > 80 || req.Reason == "" || len(req.Reason) > 1000 || req.Count < 1 || req.Count > 500 || req.TotalLimit < 1 || req.TotalLimit > 1000000 || len(req.Prefix) > 12 || !validASCII(req.Prefix) || req.Code != "" && (req.Count != 1 || len(req.Code) < 4 || len(req.Code) > 128 || !validASCII(req.Code)) || !req.Unlimited && (req.StartAt <= 0 || req.EndAt <= req.StartAt) || len(req.Rewards) < 1 || len(req.Rewards) > 100 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	for _, r := range req.Rewards {
		if r.ResourceID == "" || len(r.ResourceID) > 128 || r.Amount < 1 || r.Amount > 2147483647 {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
	}
	if req.Unlimited {
		req.StartAt = 0
		req.EndAt = 0
	}
	out, err := gameService.AdminCreateCDK(c.Request.Context(), adminPrincipal(c), req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminCDKUpdateHandler changes eligibility without rewriting already used rewards.
// @Summary 编辑CDK
// @Description 校验版本和已兑换次数，原领取记录保留
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path string true "CDK哈希"
// @Param body body model.AdminCDKUpdateRequest true "目标配置"
// @Success 200 {object} model.AdminCDKDTO
// @Router /admin/api/cdk/{id} [put]
func AdminCDKUpdateHandler(c *gin.Context) {
	hash := c.Param("id")
	decoded, e := hex.DecodeString(hash)
	var req model.AdminCDKUpdateRequest
	if e != nil || len(decoded) != 32 || len(hash) != 64 || hash != strings.ToLower(hash) || c.ShouldBindJSON(&req) != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Name == "" || utf8.RuneCountInString(req.Name) > 80 || req.Reason == "" || len(req.Reason) > 1000 || len(req.Version) != 64 || req.TotalLimit < 1 || req.TotalLimit > 1000000 || !req.Unlimited && (req.StartAt <= 0 || req.EndAt <= req.StartAt) || len(req.Rewards) < 1 || len(req.Rewards) > 100 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	for _, r := range req.Rewards {
		if r.ResourceID == "" || len(r.ResourceID) > 128 || r.Amount < 1 || r.Amount > 2147483647 {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
	}
	if req.Unlimited {
		req.StartAt = 0
		req.EndAt = 0
	}
	out, err := gameService.AdminUpdateCDK(c.Request.Context(), adminPrincipal(c), hash, req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminCDKClaimsHandler reads committed native redemptions.
// @Summary CDK兑换记录
// @Description 当前区服的实际角色、名称及领取时间
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path string true "CDK哈希"
// @Success 200 {object} model.AdminCDKClaimsResponse
// @Router /admin/api/cdk/{id}/claims [get]
func AdminCDKClaimsHandler(c *gin.Context) {
	page, size, ok := adminPage(c)
	hash := c.Param("id")
	decoded, e := hex.DecodeString(hash)
	if !ok || e != nil || len(decoded) != 32 || len(hash) != 64 || hash != strings.ToLower(hash) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminCDKClaims(c.Request.Context(), adminPrincipal(c), hash, page, size)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
