package handler

import (
	"encoding/hex"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/routes"
	"hyjs-server/service"
	"hyjs-server/wrapper"
	"net/http"
	"strings"
	"unicode/utf8"
)

func init() {
	routes.RegisterPostRouter("/game-feedback/list", wrapper.Wrapper(GameFeedbackListHandler))
	routes.RegisterPostRouter("/game-feedback/create", wrapper.Wrapper(GameFeedbackCreateHandler))
	routes.RegisterPostRouter("/game-feedback/detail", wrapper.Wrapper(GameFeedbackDetailHandler))
	routes.RegisterPostRouter("/game-feedback/reply", wrapper.Wrapper(GameFeedbackReplyHandler))
}

// RegisterAdminFeedbackRoutes binds the credential-protected support inbox.
func RegisterAdminFeedbackRoutes(a *gin.RouterGroup) {
	a.GET("/feedback", AdminFeedbackListHandler)
	a.GET("/feedback/:id", AdminFeedbackDetailHandler)
	a.POST("/feedback/:id/replies", AdminFeedbackReplyHandler)
	a.PUT("/feedback/:id/state", AdminFeedbackStateHandler)
}

// GameFeedbackListHandler handles authenticated player support.
// @Summary 玩家客服List
// @Description 复用原账号会话，反馈历史只对本人开放
// @Tags Feedback
// @Accept json
// @Produce json
// @Param body body model.GameFeedbackListRequest true "玩家会话及反馈参数"
// @Success 200 {object} model.FeedbackListResponse
// @Router /game-feedback/list [post]
func GameFeedbackListHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16384)
	var req model.GameFeedbackListRequest
	if c.ShouldBindJSON(&req) != nil || req.PlayerID < 1 || len(req.SessionHex) != 32 {
		return model.NewErrParamsError()
	}
	session, err := hex.DecodeString(req.SessionHex)
	if err != nil || len(session) != 16 {
		return model.NewErrParamsError()
	}
	owner, err := service.AuthorizeAccount(c.Request.Context(), req.PlayerID, session)
	if err != nil {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterDenied, "登录已失效，请重新登录")
	}
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 20
	}
	if req.Page < 1 || req.Page > 100000 || req.PageSize < 1 || req.PageSize > 100 {
		return model.NewErrParamsError()
	}
	out, err := service.GameFeedbackList(c.Request.Context(), owner.Id, req.Page, req.PageSize)
	if err != nil {
		return err
	}
	adminSuccess(c, out)
	return nil
}

// GameFeedbackCreateHandler handles authenticated player support.
// @Summary 玩家客服Create
// @Description 复用原账号会话，反馈历史只对本人开放
// @Tags Feedback
// @Accept json
// @Produce json
// @Param body body model.GameFeedbackCreateRequest true "玩家会话及反馈参数"
// @Success 200 {object} model.FeedbackDTO
// @Router /game-feedback/create [post]
func GameFeedbackCreateHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16384)
	var req model.GameFeedbackCreateRequest
	if c.ShouldBindJSON(&req) != nil || req.PlayerID < 1 || len(req.SessionHex) != 32 {
		return model.NewErrParamsError()
	}
	session, err := hex.DecodeString(req.SessionHex)
	if err != nil || len(session) != 16 {
		return model.NewErrParamsError()
	}
	owner, err := service.AuthorizeAccount(c.Request.Context(), req.PlayerID, session)
	if err != nil {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterDenied, "登录已失效，请重新登录")
	}
	req.Content = strings.TrimSpace(req.Content)
	if _, err = uuid.Parse(req.RequestID); err != nil || len(req.RequestID) != 36 || !utf8.ValidString(req.Content) || utf8.RuneCountInString(req.Content) < 1 || utf8.RuneCountInString(req.Content) > 2000 {
		return model.NewErrParamsError()
	}
	req.Subject = strings.TrimSpace(req.Subject)
	if !utf8.ValidString(req.Subject) || utf8.RuneCountInString(req.Subject) < 1 || utf8.RuneCountInString(req.Subject) > 80 {
		return model.NewErrParamsError()
	}
	switch req.Category {
	case "bug", "suggestion", "account", "other":
	default:
		return model.NewErrParamsError()
	}
	out, err := service.GameFeedbackCreate(c.Request.Context(), owner.Id, model.GameFeedbackRequest{RequestID: req.RequestID, Category: req.Category, Subject: req.Subject, Content: req.Content})
	if err != nil {
		return err
	}
	adminSuccess(c, out)
	return nil
}

// GameFeedbackDetailHandler handles authenticated player support.
// @Summary 玩家客服Detail
// @Description 复用原账号会话，反馈历史只对本人开放
// @Tags Feedback
// @Accept json
// @Produce json
// @Param body body model.GameFeedbackDetailRequest true "玩家会话及反馈参数"
// @Success 200 {object} model.FeedbackDTO
// @Router /game-feedback/detail [post]
func GameFeedbackDetailHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16384)
	var req model.GameFeedbackDetailRequest
	if c.ShouldBindJSON(&req) != nil || req.PlayerID < 1 || len(req.SessionHex) != 32 {
		return model.NewErrParamsError()
	}
	session, err := hex.DecodeString(req.SessionHex)
	if err != nil || len(session) != 16 {
		return model.NewErrParamsError()
	}
	owner, err := service.AuthorizeAccount(c.Request.Context(), req.PlayerID, session)
	if err != nil {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterDenied, "登录已失效，请重新登录")
	}
	if _, err = uuid.Parse(req.ID); err != nil {
		return model.NewErrParamsError()
	}
	out, err := service.GameFeedbackDetail(c.Request.Context(), owner.Id, req.ID)
	if err != nil {
		return err
	}
	adminSuccess(c, out)
	return nil
}

// GameFeedbackReplyHandler handles authenticated player support.
// @Summary 玩家客服Reply
// @Description 复用原账号会话，反馈历史只对本人开放
// @Tags Feedback
// @Accept json
// @Produce json
// @Param body body model.GameFeedbackReplyRequest true "玩家会话及反馈参数"
// @Success 200 {object} model.FeedbackDTO
// @Router /game-feedback/reply [post]
func GameFeedbackReplyHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16384)
	var req model.GameFeedbackReplyRequest
	if c.ShouldBindJSON(&req) != nil || req.PlayerID < 1 || len(req.SessionHex) != 32 {
		return model.NewErrParamsError()
	}
	session, err := hex.DecodeString(req.SessionHex)
	if err != nil || len(session) != 16 {
		return model.NewErrParamsError()
	}
	owner, err := service.AuthorizeAccount(c.Request.Context(), req.PlayerID, session)
	if err != nil {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterDenied, "登录已失效，请重新登录")
	}
	req.Content = strings.TrimSpace(req.Content)
	if _, err = uuid.Parse(req.RequestID); err != nil || len(req.RequestID) != 36 || !utf8.ValidString(req.Content) || utf8.RuneCountInString(req.Content) < 1 || utf8.RuneCountInString(req.Content) > 2000 {
		return model.NewErrParamsError()
	}
	if _, err = uuid.Parse(req.ID); err != nil {
		return model.NewErrParamsError()
	}
	out, err := service.GameFeedbackReply(c.Request.Context(), owner.Id, model.GameFeedbackRequest{ID: req.ID, RequestID: req.RequestID, Content: req.Content})
	if err != nil {
		return err
	}
	adminSuccess(c, out)
	return nil
}

// AdminFeedbackListHandler 客服工单列表.
// @Summary 客服工单列表
// @Description 保留消息历史；写入要求运营权限和CSRF验证
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} model.FeedbackListResponse
// @Router /admin/api/feedback [get]
func AdminFeedbackListHandler(c *gin.Context) {
	page, size, ok := adminPage(c)
	state := c.Query("state")
	q := strings.TrimSpace(c.Query("q"))
	if !ok || len(q) > 128 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	switch state {
	case "", "open", "processing", "replied", "closed":
	default:
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := service.AdminFeedbackList(c.Request.Context(), adminPrincipal(c), state, q, int(page), int(size))
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminFeedbackDetailHandler 客服工单详情.
// @Summary 客服工单详情
// @Description 保留消息历史；写入要求运营权限和CSRF验证
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} model.FeedbackDTO
// @Router /admin/api/feedback/{id} [get]
func AdminFeedbackDetailHandler(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := service.AdminFeedbackDetail(c.Request.Context(), adminPrincipal(c), id)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminFeedbackReplyHandler 客服回复.
// @Summary 客服回复
// @Description 保留消息历史；写入要求运营权限和CSRF验证
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} model.FeedbackDTO
// @Router /admin/api/feedback/{id}/replies [post]
func AdminFeedbackReplyHandler(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	var req model.AdminFeedbackReplyRequest
	if c.ShouldBindJSON(&req) != nil || len(req.Version) != 64 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	req.Content = strings.TrimSpace(req.Content)
	if _, err := uuid.Parse(req.RequestID); err != nil || len(req.RequestID) != 36 || !utf8.ValidString(req.Content) || utf8.RuneCountInString(req.Content) < 1 || utf8.RuneCountInString(req.Content) > 2000 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := service.AdminFeedbackReply(c.Request.Context(), adminPrincipal(c), id, req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}

// AdminFeedbackStateHandler 客服处理状态.
// @Summary 客服处理状态
// @Description 保留消息历史；写入要求运营权限和CSRF验证
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} model.FeedbackDTO
// @Router /admin/api/feedback/{id}/state [put]
func AdminFeedbackStateHandler(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	var req model.AdminFeedbackStateRequest
	if c.ShouldBindJSON(&req) != nil || len(req.Version) != 64 {
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	switch req.State {
	case "open", "processing", "replied", "closed":
	default:
		adminError(c, constUtil.ErrAdminInput)
		return
	}
	out, err := service.AdminFeedbackState(c.Request.Context(), adminPrincipal(c), id, req)
	if err != nil {
		adminError(c, err)
		return
	}
	adminSuccess(c, out)
}
