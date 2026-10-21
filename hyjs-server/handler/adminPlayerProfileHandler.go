package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
)

// RegisterAdminPlayerProfileRoutes 注册同一角色详情下的资料编辑与原表选项。
func RegisterAdminPlayerProfileRoutes(a *gin.RouterGroup) {
	a.GET("/game-players/:id/edit-options", AdminGamePlayerEditOptionsHandler)
	a.PUT("/game-players/:id/profile", AdminGamePlayerProfileHandler)
}

// AdminGamePlayerEditOptionsHandler 返回当前角色可用的原表选项。
// @Summary 玩家资料编辑选项
// @Description 返回等级、会员、主线、场景及资源边界
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path integer true "玩家ID"
// @Success 200 {object} model.AdminGamePlayerEditOptionsResponse
// @Router /admin/api/game-players/{id}/edit-options [get]
func AdminGamePlayerEditOptionsHandler(c *gin.Context) {
	id, ok := adminID(c)
	if !ok || id < 1 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := gameService.AdminGamePlayerEditOptions(c.Request.Context(), id)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminGamePlayerProfileHandler 校验完整业务表单，拒绝身份、统计和未知字段。
// @Summary 编辑玩家资料和资源
// @Description 版本保护下保存资料，主线调整不发放奖励，已领取历史保留
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path integer true "玩家ID"
// @Param body body model.AdminGamePlayerProfileRequest true "资料、版本及操作原因"
// @Success 200 {object} model.AdminGamePlayerDTO
// @Router /admin/api/game-players/{id}/profile [put]
func AdminGamePlayerProfileHandler(c *gin.Context) {
	id, ok := adminID(c)
	var req model.AdminGamePlayerProfileRequest
	raw, readErr := io.ReadAll(io.LimitReader(c.Request.Body, 16385))
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if !ok || id < 1 || readErr != nil || len(raw) > 16384 || decoder.Decode(&req) != nil || len(req.Version) != 64 || !utf8.ValidString(req.Name) || req.Name == "" || strings.TrimSpace(req.Name) != req.Name || strings.IndexFunc(req.Name, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) >= 0 || req.Level < 1 || req.Level > math.MaxInt32 || req.Exp < 0 || req.Exp > 9007199254740991 || req.VIP < 0 || req.VIP > math.MaxInt32 || req.CreatedAt < 1 || req.CreatedAt > 253402300799999 || strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 1000 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	var present map[string]json.RawMessage
	if json.Unmarshal(raw, &present) != nil || present["exp"] == nil || present["vip"] == nil || bytes.Equal(present["exp"], []byte("null")) || bytes.Equal(present["vip"], []byte("null")) {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	// 数字字段显式传 null 不等同于运营输入零，避免资源被无意清空。
	var balanceValues map[string]json.RawMessage
	if json.Unmarshal(present["balances"], &balanceValues) != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	for _, value := range balanceValues {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
	}
	width := 0
	for _, unit := range utf16.Encode([]rune(req.Name)) {
		if unit <= 256 {
			width++
		} else {
			width += 2
		}
	}
	if width > 12 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	levelFound, vipFound, missionFound := false, false, false
	for _, row := range gameService.Tables["UserExp"] {
		levelFound = levelFound || row["level"] == strconv.FormatInt(req.Level, 10)
	}
	for _, row := range gameService.Tables["VipPerks"] {
		vipFound = vipFound || row["vip"] == strconv.FormatInt(req.VIP, 10)
	}
	for _, row := range gameService.Tables["CopyMission"] {
		missionFound = missionFound || row["MissionId"] == req.Mission && row["ChapterId"] == req.Chapter
	}
	if !levelFound || !vipFound || !missionFound || len(req.Scene) > 96 || req.Scene == "" || len(req.Balances) != 10 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	for key, value := range req.Balances {
		limit := int64(math.MaxInt32)
		switch key {
		case "coin", "diamond", "coinpet":
			limit = 9007199254740991
		case "energy", "stamina", "light", "point", "popularity", "score", "contribution":
		default:
			adminError(c, constUtil.ErrAdminInput)
			return
		}
		if value < 0 || value > limit {
			adminError(c, constUtil.ErrAdminInput)
			return
		}
	}
	options, err := gameService.AdminGamePlayerEditOptions(c.Request.Context(), id)
	if err != nil {
		adminError(c, err)
		return
	}
	sceneFound := false
	for _, scene := range options.Scenes {
		sceneFound = sceneFound || scene.Value == req.Scene
	}
	if !sceneFound {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	out, err := gameService.AdminUpdateGamePlayerProfile(c.Request.Context(), adminPrincipal(c), id, req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
