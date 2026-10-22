package handler

import (
	"errors"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/service"
)

// AdminWalletAdjustHandler 校验选区、版本与整数点数，提交原子钱包调整。
// @Summary 调整角色所属账号超频点数
// @Description 必须选择角色实际区服；按账号余额版本提交，同一UUID重试仅生效一次，保护账号禁止调整。
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path integer true "角色ID"
// @Param serverId query integer true "角色实际区服ID"
// @Param body body model.AdminWalletAdjustRequest true "操作、点数、原因与读取的余额版本"
// @Success 200 {object} model.AdminWalletAdjustResponse
// @Router /admin/api/wallets/{id}/adjust [post]
func AdminWalletAdjustHandler(c *gin.Context) {
	var req model.AdminWalletAdjustRequest
	id, ok := adminID(c)
	if !ok || id < 1 || service.AdminSelectedServerID(c.Request.Context()) < 1 || c.ShouldBindJSON(&req) != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	requestID, err := uuid.Parse(req.RequestID)
	if err != nil || len(req.RequestID) != 36 || (req.Action != constUtil.AdminWalletAdjustCredit && req.Action != constUtil.AdminWalletAdjustDebit) || req.Points < 1 || req.Points > constUtil.LocalWalletMaxCreditCents/100 || req.Revision == nil || *req.Revision < 0 || !utf8.ValidString(req.Reason) || utf8.RuneCountInString(req.Reason) < 1 || utf8.RuneCountInString(req.Reason) > 200 || strings.IndexFunc(req.Reason, unicode.IsControl) >= 0 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	req.RequestID = requestID.String()
	out, err := service.AdminWalletAdjust(c.Request.Context(), adminPrincipal(c), id, req)
	if errors.Is(err, service.ErrLocalWalletInsufficient) {
		c.JSON(http.StatusBadRequest, gin.H{"code": http.StatusBadRequest, "msg": constUtil.AdminWalletAdjustInsufficient, "data": nil})
		return
	}
	if errors.Is(err, constUtil.ErrAdminWalletAdjustRevision) {
		c.JSON(http.StatusConflict, gin.H{"code": http.StatusConflict, "msg": constUtil.AdminWalletAdjustConflictText, "data": nil})
		return
	}
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
