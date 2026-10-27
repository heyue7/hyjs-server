package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"hyjs-server/db"
	"hyjs-server/service"
)

func TestTrialShopRepriceHTTPRequiresAuthenticationCSRFAndVersion(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := db.InitGame(ctx, filepath.Join(dir, "trial-admin.db")); err != nil {
		t.Fatal(err)
	}
	connection, _ := db.GetIns().DB()
	t.Cleanup(func() { _ = connection.Close() })
	boot := filepath.Join(dir, "bootstrap.json")
	if err := os.WriteFile(boot, []byte(`{"username":"trialadmin","password":"trial-password-12345"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := service.AdminInitialize(ctx, boot); err != nil {
		t.Fatal(err)
	}
	prior := gameService
	gameService = &service.GameService{}
	t.Cleanup(func() { gameService = prior })
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterAdminRoutes(router, dir)
	call := func(path, body, csrf string, cookies []*http.Cookie) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		if csrf != "" {
			request.Header.Set("X-CSRF-Token", csrf)
		}
		for _, cookie := range cookies {
			request.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	path := "/admin/api/shops/point/trial-reprice"
	if response := call(path, `{}`, "", nil); response.Code != 401 {
		t.Fatal("missing authentication accepted", response.Code)
	}
	login := call("/admin/api/auth/login", `{"username":"trialadmin","password":"trial-password-12345"}`, "", nil)
	if login.Code != 200 {
		t.Fatal(login.Code, login.Body.String())
	}
	var result struct {
		Data struct {
			CSRF string `json:"csrfToken"`
		}
	}
	if err := json.Unmarshal(login.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	cookies := login.Result().Cookies()
	if response := call(path, `{}`, "", cookies); response.Code != 403 {
		t.Fatal("missing CSRF accepted", response.Code)
	}
	for _, body := range []string{`{}`, `{"version":"short"}`, `{"version":"` + strings.Repeat("z", 64) + `"}`} {
		if response := call(path, body, result.Data.CSRF, cookies); response.Code != 400 {
			t.Fatal("invalid version accepted", response.Code, response.Body.String())
		}
	}
	if response := call("/admin/api/shops/ordinary/trial-reprice", `{}`, result.Data.CSRF, cookies); response.Code != 404 {
		t.Fatal("another shop had reprice route", response.Code)
	}
}
