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
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"hyjs-server/service"
)

// TestAdminHTTPActualAuthenticationCSRFPublicationAndStatic exercises the real router,
// cookies, CSRF, viewer permissions, pure-text publication and SPA/API isolation.
func TestAdminHTTPActualAuthenticationCSRFPublicationAndStatic(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if e := db.InitGame(ctx, filepath.Join(dir, "admin.sqlite")); e != nil {
		t.Fatal(e)
	}
	sqlDB, _ := db.GetIns().DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	boot := filepath.Join(dir, "bootstrap.json")
	if e := os.WriteFile(boot, []byte(`{"username":"fixtureadmin","password":"fixture-password-12345"}`), 0600); e != nil {
		t.Fatal(e)
	}
	if e := service.AdminInitialize(ctx, boot); e != nil {
		t.Fatal(e)
	}
	if e := service.InitializeAdminOperations(ctx); e != nil {
		t.Fatal(e)
	}
	prior := gameService
	t.Cleanup(func() { gameService = prior })
	gameService = &service.GameService{}
	if e := gameService.LoadTables("../../analysis/tables"); e != nil {
		t.Fatal(e)
	}
	assets := filepath.Join(dir, "dist")
	_ = os.MkdirAll(filepath.Join(assets, "assets"), 0700)
	_ = os.WriteFile(filepath.Join(assets, "index.html"), []byte("<html>admin-static-fixture</html>"), 0600)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterAdminRoutes(router, assets)
	call := func(method, path, body, csrf, origin string, cookies []*http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	if w := call("GET", "/admin/api/overview", "", "", "", nil); w.Code != 401 {
		t.Fatal("unauthenticated admin endpoint", w.Code)
	}
	loginBody := `{"username":"fixtureadmin","password":"fixture-password-12345"}`
	if w := call("POST", "/admin/api/auth/login", loginBody, "", "https://untrusted.invalid", nil); w.Code != 403 {
		t.Fatal("foreign origin login accepted", w.Code)
	}
	w := call("POST", "/admin/api/auth/login", loginBody, "", "", nil)
	if w.Code != 200 {
		t.Fatal("real cookie login", w.Code, w.Body.String())
	}
	var result struct {
		Code int
		Data struct {
			User model.AdminUserDTO
			CSRF string `json:"csrfToken"`
		}
	}
	if json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Code != 0 || len(result.Data.CSRF) != 64 {
		t.Fatal("login DTO/CSRF missing")
	}
	cookies := w.Result().Cookies()
	csrf := result.Data.CSRF
	for _, cookie := range cookies {
		if cookie.Name == constUtil.AdminSessionCookieName && (!cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode) {
			t.Fatal("session cookie not protected")
		}
	}
	if w = call("GET", "/admin/api/auth/me", "", "", "", cookies); w.Code != 200 {
		t.Fatal("cookie reauthentication", w.Code)
	}
	if w = call("GET", "/admin/api/items?q=item_020&page=1&pageSize=20", "", "", "", cookies); w.Code != 200 || !strings.Contains(w.Body.String(), "王者之魂") {
		t.Fatal("actual original catalogue", w.Code, w.Body.String())
	}
	if w = call("GET", "/admin/api/items?page=0", "", "", "", cookies); w.Code != 400 {
		t.Fatal("invalid pagination accepted")
	}
	notice := `{"title":"测试公告","content":"<script>window.bad=1</script>","enabled":true,"startAt":0,"endAt":0}`
	if w = call("POST", "/admin/api/announcements", notice, "", "", cookies); w.Code != 403 {
		t.Fatal("missing CSRF published notice")
	}
	if w = call("POST", "/admin/api/announcements", notice, csrf, "", cookies); w.Code != 200 {
		t.Fatal("authorized publication", w.Code, w.Body.String())
	}
	if w = call("GET", "/notices", "", "", "", nil); w.Code != 200 || strings.Contains(w.Body.String(), "<script>") || !strings.Contains(w.Body.String(), "&lt;script&gt;") {
		t.Fatal("pure text public notice failed", w.Body.String())
	}
	principal, e := service.AdminAuthenticate(ctx, func() string {
		for _, c := range cookies {
			if c.Name == constUtil.AdminSessionCookieName {
				return c.Value
			}
		}
		return ""
	}())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = service.AdminCreateUser(ctx, principal, "fixtureviewer", "viewer-password-12345", constUtil.AdminRoleViewer); e != nil {
		t.Fatal(e)
	}
	w = call("POST", "/admin/api/auth/login", `{"username":"fixtureviewer","password":"viewer-password-12345"}`, "", "", nil)
	var viewResult struct {
		Data struct {
			CSRF string `json:"csrfToken"`
		}
	}
	_ = json.Unmarshal(w.Body.Bytes(), &viewResult)
	if w = call("POST", "/admin/api/announcements", notice, viewResult.Data.CSRF, "", w.Result().Cookies()); w.Code != 403 {
		t.Fatal("viewer published notice", w.Code, w.Body.String())
	}
	if w = call("GET", "/admin/", "", "", "", nil); w.Code != 200 || !strings.Contains(w.Body.String(), "admin-static-fixture") {
		t.Fatal("admin index loops through trailing slash redirect", w.Code)
	}
	if w = call("GET", "/admin/activities", "", "", "", nil); w.Code != 200 || !strings.Contains(w.Body.String(), "admin-static-fixture") {
		t.Fatal("SPA route unavailable")
	}
	if w = call("GET", "/admin/api/unknown", "", "", "", nil); w.Code != 404 || strings.Contains(w.Body.String(), "admin-static-fixture") {
		t.Fatal("unknown API returned SPA")
	}
	if w = call("POST", "/admin/api/auth/logout", "{}", csrf, "", cookies); w.Code != 200 {
		t.Fatal("logout failed")
	}
	if w = call("GET", "/admin/api/auth/me", "", "", "", cookies); w.Code != 401 {
		t.Fatal("revoked cookie survived logout")
	}
}
