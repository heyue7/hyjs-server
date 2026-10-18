package handler

import (
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"hyjs-server/service"
)

// TestP12WorldBossFinishRejectsClientResults 先在边界拒绝客户端伤害/串场/伪造认证输入。
func TestP12WorldBossFinishRejectsClientResults(t *testing.T) {
	previous := gameService
	gameService = &service.GameService{}
	defer func() { gameService = previous }()
	gin.SetMode(gin.TestMode)
	for _, body := range []string{
		`{}`, `{"fightId":""}`, `{"fightId":"one","damage":999999}`, `{"fightId":"one","win":true}`,
		`{"fightId":"one"} {"fightId":"two"}`, `{"fightId":" one"}`, `{"fightId":"../one"}`,
		`{"fightId":"` + strings.Repeat("x", 65) + `"}`,
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/world-boss/finish", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Request.Header.Set("X-Hyjs-Player-ID", "4")
		c.Request.Header.Set("X-Hyjs-Session-Key", base64.StdEncoding.EncodeToString(make([]byte, 16)))
		if err := P12WorldBossFinishHandler(c); err == nil {
			t.Fatal("untrusted result or invalid fight ID reached settlement", body)
		}
	}
	for _, session := range []string{"", "not-base64", base64.StdEncoding.EncodeToString(make([]byte, 15))} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/world-boss/finish", strings.NewReader(`{"fightId":"one"}`))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Request.Header.Set("X-Hyjs-Player-ID", "4")
		c.Request.Header.Set("X-Hyjs-Session-Key", session)
		if err := P12WorldBossFinishHandler(c); err == nil {
			t.Fatal("invalid session reached settlement")
		}
	}
}
