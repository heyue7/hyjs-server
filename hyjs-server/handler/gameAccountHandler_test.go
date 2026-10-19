package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"hyjs-server/db"
	"hyjs-server/model"
	"hyjs-server/wrapper"
)

// TestGameAccountRegistrationHTTPValidationDuplicateAndThrottle exercises the actual guest registration handler.
func TestGameAccountRegistrationHTTPValidationDuplicateAndThrottle(t *testing.T) {
	if err := db.InitGame(context.Background(), filepath.Join(t.TempDir(), "register.sqlite")); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.GetIns().DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/game-account/register", wrapper.Wrapper(GameAccountRegisterHandler))
	call := func(body string, clients ...string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/game-account/register", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "198.51.100.77:31415"
		if len(clients) > 0 {
			req.RemoteAddr = clients[0]
		}
		out := httptest.NewRecorder()
		router.ServeHTTP(out, req)
		return out
	}
	if out := call(`{"username":"nickname-required","password":"original-password","confirmPassword":"original-password"}`, "198.51.100.78:31415"); out.Code != 400 {
		t.Fatal("registration accepted a missing nickname", out.Code)
	}
	if out := call(`{"username":"bad space","password":"original-password","confirmPassword":"original-password","nickname":"冒险者"}`); out.Code != 400 {
		t.Fatal("invalid login identity accepted", out.Code)
	}
	if out := call(`{"username":"valid-name","password":"中文","confirmPassword":"中文","nickname":"冒险者"}`); out.Code != 400 {
		t.Fatal("registered password rejected later by original six-character login", out.Code)
	}
	body := `{"username":"registered-http","password":"original-password","confirmPassword":"original-password","nickname":"冒险者"}`
	first := call(body)
	var decoded struct {
		Code int
		Data model.GameAccountRegisterResponse
	}
	if first.Code != 200 || json.Unmarshal(first.Body.Bytes(), &decoded) != nil || decoded.Code != 0 || !decoded.Data.Registered || decoded.Data.PlayerID < 1 || strings.Contains(first.Body.String(), "original-password") || strings.Contains(first.Body.String(), "session") {
		t.Fatal("registration success contract or credential disclosure", first.Code)
	}
	if out := call(body); out.Code != 409 {
		t.Fatal("duplicate HTTP registration silently logged in", out.Code)
	}
	if out := call(`{"username":"registered-second","password":"original-password","confirmPassword":"original-password","nickname":"冒险者"}`); out.Code != 200 {
		t.Fatal("registration budget rejected before configured limit", out.Code)
	}
	if out := call(`{"username":"registered-third","password":"original-password","confirmPassword":"original-password","nickname":"冒险者"}`); out.Code != 429 || out.Header().Get("Retry-After") == "" {
		t.Fatal("guest registration limit not enforced", out.Code)
	}
	var count int64
	if err := db.GetIns().Model(&db.Profile{}).Where("uid=?", "registered-third").Count(&count).Error; err != nil || count != 0 {
		t.Fatal("rate-limited request created an account", err)
	}
}
