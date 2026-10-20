package handler

import (
	"encoding/base64"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/routes"
	"hyjs-server/wrapper"
)

func init() {
	routes.RegisterGetRouter("/voice/config", wrapper.Wrapper(P10VoiceConfigHandler))
	routes.RegisterPostRouter("/voice/upload", wrapper.Wrapper(P10VoiceUploadHandler))
	routes.RegisterPostRouter("/voice/download", wrapper.Wrapper(P10VoiceDownloadHandler))
	routes.RegisterPostRouter("/voice/delete", wrapper.Wrapper(P10VoiceDeleteHandler))
}

// P10VoiceConfigHandler 使用每次请求的当前原会话操作实际本地音频。
// @Summary 本地音频Config
// @Description 默认关闭；当前会话及频道权限每次验证，原外部GCloud认证未恢复。
// @Tags LocalVoice
// @Accept json
// @Produce json
// @Param X-Hyjs-Player-ID header int true "当前角色"
// @Param X-Hyjs-Session-Key header string true "当前原会话Base64"
// @Param chatType query int true "0=总配置，1=世界，2=公会，3=私聊，5=野外，6=公会战"
// @Success 200 {object} model.P10VoiceConfigResponse
// @Router /voice/config [get]
func P10VoiceConfigHandler(c *gin.Context) error {
	if gameService == nil || !gameService.P10VoiceReady {
		return model.NewErrSystemError()
	}
	var req model.P10VoiceConfigRequest
	if err := c.ShouldBindHeader(&req); err != nil {
		return model.NewErrParamsError()
	}
	session, err := base64.StdEncoding.DecodeString(req.SessionKey)
	if err != nil || len(session) != 16 || base64.StdEncoding.EncodeToString(session) != req.SessionKey {
		return model.NewErrParamsError()
	}
	kind, err := strconv.ParseInt(c.Query("chatType"), 10, 32)
	if err != nil || (kind != 0 && kind != 1 && kind != 2 && kind != 3 && kind != 5 && kind != 6) {
		return model.NewErrParamsError()
	}
	result, err := gameService.P10VoiceConfig(c.Request.Context(), req.PlayerID, session, int32(kind))
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}

// P10VoiceUploadHandler 使用每次请求的当前原会话操作实际本地音频。
// @Summary 本地音频Upload
// @Description 默认关闭；当前会话及频道权限每次验证，原外部GCloud认证未恢复。
// @Tags LocalVoice
// @Accept audio/wav
// @Produce json
// @Param X-Hyjs-Player-ID header int true "当前角色"
// @Param X-Hyjs-Session-Key header string true "当前原会话Base64"
// @Param X-Hyjs-Chat-Type header int true "1/2/3/5/6频道"
// @Param X-Hyjs-Request-ID header string true "ASCII上传请求键"
// @Param X-Hyjs-Voice-Scope header string true "录音开始的实际范围"
// @Param body body string true "真实PCM16单声道16k WAV"
// @Success 200 {object} model.P10VoiceUploadResponse
// @Router /voice/upload [post]
func P10VoiceUploadHandler(c *gin.Context) error {
	if gameService == nil || !gameService.P10VoiceReady {
		return model.NewErrSystemError()
	}
	var req model.P10VoiceUploadRequest
	if err := c.ShouldBindHeader(&req); err != nil {
		return model.NewErrParamsError()
	}
	session, err := base64.StdEncoding.DecodeString(req.SessionKey)
	if err != nil || len(session) != 16 || base64.StdEncoding.EncodeToString(session) != req.SessionKey {
		return model.NewErrParamsError()
	}
	if req.ChatType != 1 && req.ChatType != 2 && req.ChatType != 3 && req.ChatType != 5 && req.ChatType != 6 {
		return model.NewErrParamsError()
	}
	if strings.Trim(req.RequestID, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_") != "" || strings.TrimSpace(req.VoiceScope) != req.VoiceScope || !utf8.ValidString(req.VoiceScope) || strings.ContainsRune(req.VoiceScope, '\x00') || c.ContentType() != "audio/wav" {
		return model.NewErrParamsError()
	}
	// 对真正读取的请求体设硬上限，拒绝伪造Content-Length与分块超大音频。
	raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, constUtil.P10VoiceMaxBytes))
	if err != nil || len(raw) < 44 {
		return model.NewErrParamsError()
	}
	result, err := gameService.UploadP10Voice(c.Request.Context(), req.PlayerID, session, req.ChatType, req.RequestID, req.VoiceScope, raw)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}

// P10VoiceDownloadHandler 使用每次请求的当前原会话操作实际本地音频。
// @Summary 本地音频Download
// @Description 默认关闭；当前会话及频道权限每次验证，原外部GCloud认证未恢复。
// @Tags LocalVoice
// @Accept json
// @Produce audio/wav
// @Param X-Hyjs-Player-ID header int true "当前角色"
// @Param X-Hyjs-Session-Key header string true "当前原会话Base64"
// @Param X-Hyjs-File-Name header string true "实际opaque音频名称"
// @Success 200 {object} string
// @Router /voice/download [post]
func P10VoiceDownloadHandler(c *gin.Context) error {
	if gameService == nil || !gameService.P10VoiceReady {
		return model.NewErrSystemError()
	}
	var req model.P10VoiceDownloadRequest
	if err := c.ShouldBindHeader(&req); err != nil {
		return model.NewErrParamsError()
	}
	session, err := base64.StdEncoding.DecodeString(req.SessionKey)
	if err != nil || len(session) != 16 || base64.StdEncoding.EncodeToString(session) != req.SessionKey {
		return model.NewErrParamsError()
	}
	name := req.FileName
	if len(name) != len(constUtil.P10VoicePrefix)+32+len(".wav") || !strings.HasPrefix(name, constUtil.P10VoicePrefix) || !strings.HasSuffix(name, ".wav") || strings.Trim(name[len(constUtil.P10VoicePrefix):len(constUtil.P10VoicePrefix)+32], "0123456789abcdef") != "" {
		return model.NewErrParamsError()
	}
	raw, err := gameService.DownloadP10Voice(c.Request.Context(), req.PlayerID, session, name)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "audio/wav", raw)
	return nil
}

// P10VoiceDeleteHandler 使用每次请求的当前原会话操作实际本地音频。
// @Summary 本地音频Delete
// @Description 默认关闭；当前会话及频道权限每次验证，原外部GCloud认证未恢复。
// @Tags LocalVoice
// @Accept json
// @Produce json
// @Param X-Hyjs-Player-ID header int true "当前角色"
// @Param X-Hyjs-Session-Key header string true "当前原会话Base64"
// @Param X-Hyjs-File-Name header string true "实际opaque音频名称"
// @Success 200 {object} model.P10VoiceDeleteResponse
// @Router /voice/delete [post]
func P10VoiceDeleteHandler(c *gin.Context) error {
	if gameService == nil || !gameService.P10VoiceReady {
		return model.NewErrSystemError()
	}
	var req model.P10VoiceDeleteRequest
	if err := c.ShouldBindHeader(&req); err != nil {
		return model.NewErrParamsError()
	}
	session, err := base64.StdEncoding.DecodeString(req.SessionKey)
	if err != nil || len(session) != 16 || base64.StdEncoding.EncodeToString(session) != req.SessionKey {
		return model.NewErrParamsError()
	}
	name := req.FileName
	if len(name) != len(constUtil.P10VoicePrefix)+32+len(".wav") || !strings.HasPrefix(name, constUtil.P10VoicePrefix) || !strings.HasSuffix(name, ".wav") || strings.Trim(name[len(constUtil.P10VoicePrefix):len(constUtil.P10VoicePrefix)+32], "0123456789abcdef") != "" {
		return model.NewErrParamsError()
	}
	result, err := gameService.DeleteP10Voice(c.Request.Context(), req.PlayerID, session, name)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}
