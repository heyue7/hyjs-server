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
	"time"

	"github.com/gin-gonic/gin"
	"hyjs-server/db"
	"hyjs-server/model"
	"hyjs-server/service"
)

// TestAdminActivityScheduleHTTP verifies typed publication through the actual protected production router.
func TestAdminActivityScheduleHTTP(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := db.InitGame(ctx, filepath.Join(dir, "schedule.sqlite")); err != nil {
		t.Fatal(err)
	}
	connection, _ := db.GetIns().DB()
	t.Cleanup(func() { connection.Close() })
	boot := filepath.Join(dir, "bootstrap.json")
	if err := os.WriteFile(boot, []byte(`{"username":"scheduleadmin","password":"schedule-isolated-password-12345"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := service.AdminInitialize(ctx, boot); err != nil {
		t.Fatal(err)
	}
	if err := service.InitializeAdminOperations(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().AutoMigrate(&db.AdminActivityDefinition{}, &db.AdminActivityOccurrence{}, &db.AdminActivityPattern{}, &db.AdminActivityScheduleRevision{}); err != nil {
		t.Fatal(err)
	}
	prior := gameService
	t.Cleanup(func() { gameService = prior })
	gameService = &service.GameService{}
	if err := gameService.InitializeAdminGameServers(ctx); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterAdminRoutes(router, "")
	call := func(method, path string, body any, csrf string, cookies []*http.Cookie) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, strings.NewReader(string(raw)))
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
	if w := call("GET", "/admin/api/activity-schedules?serverId=1", nil, "", nil); w.Code != 401 {
		t.Fatal("anonymous", w.Code)
	}
	login := call("POST", "/admin/api/auth/login", map[string]string{"username": "scheduleadmin", "password": "schedule-isolated-password-12345"}, "", nil)
	var auth struct {
		Data struct {
			CSRF string `json:"csrfToken"`
		}
	}
	if login.Code != 200 || json.Unmarshal(login.Body.Bytes(), &auth) != nil || len(auth.Data.CSRF) != 64 {
		t.Fatal("login", login.Code)
	}
	cookies := login.Result().Cookies()
	// Search the same readable resource catalogue used by the activity cost selector.
	costResponse := call("GET", "/admin/api/activity-cost-resources?q=diamond", nil, "", cookies)
	var costs struct {
		Data model.AdminActivityCostResourcesResponse
	}
	if costResponse.Code != 200 || json.Unmarshal(costResponse.Body.Bytes(), &costs) != nil || len(costs.Data.Items) != 1 || costs.Data.Items[0].ID != "diamond" || costs.Data.Items[0].Name != "钻石" {
		t.Fatal("typed cost resource search", costResponse.Code, costResponse.Body.String())
	}
	pagedCosts := call("GET", "/admin/api/activity-cost-resources?category=currency&ids=coin,diamond&page=2&pageSize=1", nil, "", cookies)
	if pagedCosts.Code != 200 || json.Unmarshal(pagedCosts.Body.Bytes(), &costs) != nil || costs.Data.Total != 2 || len(costs.Data.Items) != 1 || costs.Data.Items[0].ID != "diamond" || len(costs.Data.Facets.Categories) == 0 {
		t.Fatal("cost selector filter/paging contract", pagedCosts.Code, pagedCosts.Body.String())
	}
	for _, query := range []string{"category=unknown-category", "quality=999", "applicableType=unknown-type", "ids=coin;drop", "page=0"} {
		if invalid := call("GET", "/admin/api/activity-cost-resources?"+query, nil, "", cookies); invalid.Code != 400 {
			t.Fatal("invalid cost filter accepted", query, invalid.Code)
		}
	}
	create := model.AdminActivityDefinitionSaveRequest{Name: "开服原任务", Form: model.AdminActivityForm{Mode: "opening-14", TemplateID: "opening-14-original", Name: "开服原任务"}, DefaultDays: 14, ClaimDays: 1, Enabled: true}
	if w := call("POST", "/admin/api/activity-templates", create, "", cookies); w.Code != 403 {
		t.Fatal("csrf", w.Code)
	}
	response := call("POST", "/admin/api/activity-templates", create, auth.Data.CSRF, cookies)
	var template struct {
		Data model.AdminActivityDefinitionDTO
	}
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &template) != nil || template.Data.ID == "" {
		t.Fatal("template", response.Code, response.Body.String())
	}
	response = call("GET", "/admin/api/activity-schedules?serverId=1", nil, "", cookies)
	var calendar struct {
		Data model.AdminActivityScheduleResponse
	}
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &calendar) != nil {
		t.Fatal("read", response.Code)
	}
	start := time.Now().Add(24 * time.Hour).UnixMilli()
	req := model.AdminActivityScheduleSaveRequest{Version: calendar.Data.Version, Items: []model.AdminActivityOccurrenceDTO{{TemplateID: template.Data.ID, TemplateVersion: template.Data.Version, StartAt: start, EndAt: start + 14*86400000, ClaimEndAt: start + 15*86400000, Enabled: true}}}
	response = call("POST", "/admin/api/activity-schedules/preview?serverId=1", req, auth.Data.CSRF, cookies)
	var preview struct {
		Data model.AdminActivitySchedulePreviewResponse
	}
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &preview) != nil || !preview.Data.Valid {
		t.Fatal("preview", response.Code, response.Body.String())
	}
	if w := call("PUT", "/admin/api/activity-schedules?serverId=1", req, auth.Data.CSRF, cookies); w.Code != 200 {
		t.Fatal("publish", w.Code, w.Body.String())
	}
	if w := call("PUT", "/admin/api/activity-schedules?serverId=1", req, auth.Data.CSRF, cookies); w.Code != 409 {
		t.Fatal("stale", w.Code)
	}
	if w := call("GET", "/admin/api/activity-schedules?serverId=99999", nil, "", cookies); w.Code != 400 {
		t.Fatal("realm", w.Code)
	}
	req.Items[0].EndAt = req.Items[0].StartAt
	if w := call("POST", "/admin/api/activity-schedules/preview?serverId=1", req, auth.Data.CSRF, cookies); w.Code != 400 {
		t.Fatal("dates", w.Code)
	}
	var audits int64
	db.GetIns().Model(&db.AdminAudit{}).Where("action=?", "activity.schedule.save").Count(&audits)
	if audits != 1 {
		t.Fatal("audit", audits)
	}
}
