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
	"hyjs-server/model"
	"hyjs-server/service"
)

// TestAdminFieldHTTP validates actual auth/CSRF/routes and every original scheme
// reference without running six duplicate gameplay journeys.
func TestAdminFieldHTTP(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := db.InitGame(ctx, filepath.Join(dir, "field-admin.sqlite")); err != nil {
		t.Fatal(err)
	}
	pool, _ := db.GetIns().DB()
	t.Cleanup(func() { pool.Close() })
	boot := filepath.Join(dir, "bootstrap.json")
	if err := os.WriteFile(boot, []byte(`{"username":"fieldfixtureadmin","password":"isolated-field-password-12345"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := service.AdminInitialize(ctx, boot); err != nil {
		t.Fatal(err)
	}
	if err := service.InitializeAdminOperations(ctx); err != nil {
		t.Fatal(err)
	}
	prior := gameService
	t.Cleanup(func() { gameService = prior })
	gameService = &service.GameService{}
	for _, load := range []func(string) error{gameService.LoadTables, gameService.LoadP8Tables, gameService.LoadP12WorldBossTables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	if err := gameService.InitializeAdminFields(ctx); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterAdminRoutes(router, "")
	call := func(method, path, body, csrf string, cookies []*http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/admin/api"+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	if w := call("GET", "/fields/templates", "", "", nil); w.Code != 401 {
		t.Fatal("anonymous fields read", w.Code)
	}
	login := call("POST", "/auth/login", `{"username":"fieldfixtureadmin","password":"isolated-field-password-12345"}`, "", nil)
	if login.Code != 200 {
		t.Fatal("isolated login", login.Code)
	}
	var session struct {
		Data struct {
			CSRF string `json:"csrfToken"`
		}
	}
	json.Unmarshal(login.Body.Bytes(), &session)
	cookies := login.Result().Cookies()
	read := call("GET", "/fields/templates", "", "", cookies)
	if read.Code != 200 {
		t.Fatal("global scheme read", read.Code, read.Body.String())
	}
	var catalog struct{ Data model.AdminFieldListResponse }
	if err := json.Unmarshal(read.Body.Bytes(), &catalog); err != nil || len(catalog.Data.Items) != 6 {
		t.Fatal("scheme reference list", err)
	}
	for _, scheme := range catalog.Data.Items {
		req := model.AdminFieldSaveRequest{Name: scheme.Name + " HTTP保存", Configuration: scheme.Configuration}
		raw, _ := json.Marshal(req)
		if w := call("POST", "/fields/templates", string(raw), "", cookies); w.Code != 403 {
			t.Fatal("CSRF scheme save", w.Code)
		}
		if w := call("POST", "/fields/templates", string(raw), session.Data.CSRF, cookies); w.Code != 200 {
			t.Fatal("original reference rejected", scheme.Configuration.Scene, w.Code, w.Body.String())
		}
	}
	invalid := model.AdminFieldSaveRequest{Name: "非法点位", Configuration: catalog.Data.Items[0].Configuration}
	invalid.Configuration.MinePoints[0].ID = "invented-point"
	raw, _ := json.Marshal(invalid)
	if w := call("POST", "/fields/templates", string(raw), session.Data.CSRF, cookies); w.Code != 400 {
		t.Fatal("nonexistent client point accepted", w.Code)
	}
	if w := call("GET", "/fields/deployments", "", "", cookies); w.Code != 400 {
		t.Fatal("missing realm fell back to all realms", w.Code)
	}
}
