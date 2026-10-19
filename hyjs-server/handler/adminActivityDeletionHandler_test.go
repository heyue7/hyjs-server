package handler

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"hyjs-server/db"
	"hyjs-server/service"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdminActivityDefinitionDeletionHTTP(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if e := db.InitGame(ctx, filepath.Join(dir, "delete.sqlite")); e != nil {
		t.Fatal(e)
	}
	conn, _ := db.GetIns().DB()
	t.Cleanup(func() { conn.Close() })
	boot := filepath.Join(dir, "bootstrap.json")
	os.WriteFile(boot, []byte(`{"username":"deleteadmin","password":"delete-isolated-password-12345"}`), 0600)
	if e := service.AdminInitialize(ctx, boot); e != nil {
		t.Fatal(e)
	}
	if e := db.GetIns().AutoMigrate(&db.AdminActivityDefinition{}); e != nil {
		t.Fatal(e)
	}
	prior := gameService
	gameService = &service.GameService{}
	t.Cleanup(func() { gameService = prior })
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterAdminRoutes(router, "")
	call := func(method, path, body, csrf string, cookies []*http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		for _, c := range cookies {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	url := "/admin/api/activity-templates/template-test"
	if w := call("DELETE", url, `{"version":"`+strings.Repeat("a", 64)+`"}`, "", nil); w.Code != 401 {
		t.Fatal("anonymous delete", w.Code)
	}
	login := call("POST", "/admin/api/auth/login", `{"username":"deleteadmin","password":"delete-isolated-password-12345"}`, "", nil)
	var auth struct {
		Data struct {
			CSRF string `json:"csrfToken"`
		}
	}
	if json.Unmarshal(login.Body.Bytes(), &auth) != nil || auth.Data.CSRF == "" {
		t.Fatal("login", login.Code)
	}
	cookies := login.Result().Cookies()
	if w := call("DELETE", url, `{"version":"`+strings.Repeat("a", 64)+`"}`, "", cookies); w.Code != 403 {
		t.Fatal("csrf bypass", w.Code)
	}
	if w := call("DELETE", url, `{"version":"bad"}`, auth.Data.CSRF, cookies); w.Code != 400 {
		t.Fatal("invalid version", w.Code)
	}
	row := db.AdminActivityDefinition{ID: "template-test", Name: "删除测试", Mode: "opening-14", FormJSON: []byte(`{"mode":"opening-14"}`), Revision: 1}
	if e := db.GetIns().Create(&row).Error; e != nil {
		t.Fatal(e)
	}
	if w := call("DELETE", url, `{"version":"`+strings.Repeat("a", 64)+`"}`, auth.Data.CSRF, cookies); w.Code != 409 {
		t.Fatal("stale delete", w.Code)
	}
	// Obtain the production template version through the same list endpoint.
	listing := call("GET", "/admin/api/activity-templates", "", "", cookies)
	var data struct {
		Data struct {
			Items []struct {
				ID      string `json:"id"`
				Version string `json:"version"`
			} `json:"items"`
		}
	}
	if json.Unmarshal(listing.Body.Bytes(), &data) != nil || len(data.Data.Items) != 1 {
		t.Fatal("list", listing.Code, listing.Body.String())
	}
	version := data.Data.Items[0].Version
	if w := call("DELETE", url, `{"version":"`+version+`"}`, auth.Data.CSRF, cookies); w.Code != 200 {
		t.Fatal("delete", w.Code, w.Body.String())
	}
	var after db.AdminActivityDefinition
	db.GetIns().First(&after, "id=?", row.ID)
	if !after.Archived || after.Enabled {
		t.Fatal("not archived")
	}
}
