package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"hyjs-server/service"
)

// TestAdminSecurityEventHTTPRoleFiltersAndPaging 验证独立路由的admin复核、固定枚举与现有分页边界。
func TestAdminSecurityEventHTTPRoleFiltersAndPaging(t *testing.T) {
	ctx := context.Background()
	if err := db.InitGame(ctx, filepath.Join(t.TempDir(), "security-handler.sqlite")); err != nil {
		t.Fatal(err)
	}
	connection, _ := db.GetIns().DB()
	t.Cleanup(func() { connection.Close() })
	if err := db.GetIns().WithContext(ctx).AutoMigrate(&db.AdminUser{}); err != nil {
		t.Fatal(err)
	}
	if err := service.InitializeSecurityEvents(ctx); err != nil {
		t.Fatal(err)
	}
	adminUser := db.AdminUser{Username: "handler-security-admin", PasswordHash: "unused-fixture", Role: constUtil.AdminRoleAdmin, Enabled: true}
	operatorUser := db.AdminUser{Username: "handler-security-operator", PasswordHash: "unused-fixture", Role: constUtil.AdminRoleOperator, Enabled: true}
	for _, user := range []*db.AdminUser{&adminUser, &operatorUser} {
		if err := db.GetIns().WithContext(ctx).Create(user).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := service.RecordSecurityEvent(ctx, model.SecurityEventInput{
		Actor: "alice", SourceIP: "203.0.113.7", Action: "auth.login", Target: "acct-1",
		Outcome: constUtil.AdminSecurityEventOutcomeSuccess, ReasonCode: "ok", RequestID: "req-1",
	}); err != nil {
		t.Fatal(err)
	}

	principals := map[string]*model.AdminPrincipal{
		"admin":    {UserID: adminUser.ID, Username: adminUser.Username, Role: adminUser.Role},
		"operator": {UserID: operatorUser.ID, Username: operatorUser.Username, Role: operatorUser.Role},
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	a := router.Group("/admin/api")
	// 本测试直接注入已认证身份，只验证新增路由的角色复核和参数边界。
	a.Use(func(c *gin.Context) {
		c.Set(constUtil.AdminPrincipalContextKey, principals[c.GetHeader("X-Test-Principal")])
		c.Next()
	})
	RegisterAdminSecurityEventRoutes(a)
	call := func(query, principal string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/admin/api/security-events"+query, nil)
		request.Header.Set("X-Test-Principal", principal)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder
	}

	recorder := call("?page=1&pageSize=25&action=auth.login&outcome=success&actor=alice", "admin")
	if recorder.Code != http.StatusOK {
		t.Fatalf("admin security events status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Code int                               `json:"code"`
		Data model.AdminSecurityEventsResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != 0 || response.Data.Total != 1 || len(response.Data.Items) != 1 || response.Data.Items[0].Actor != "alice" {
		t.Fatalf("admin security response = %+v", response)
	}

	if recorder = call("?page=1&pageSize=25", "operator"); recorder.Code != http.StatusForbidden {
		t.Fatalf("operator security events status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	for name, query := range map[string]string{
		"bad outcome":  "?page=1&pageSize=25&outcome=unknown",
		"long action":  "?page=1&pageSize=25&action=" + strings.Repeat("a", 65),
		"long actor":   "?page=1&pageSize=25&actor=" + strings.Repeat("a", 65),
		"zero page":    "?page=0&pageSize=25",
		"large page":   "?page=1&pageSize=101",
		"missing page": "?page=not-a-number",
	} {
		if recorder = call(query, "admin"); recorder.Code != http.StatusBadRequest {
			t.Fatalf("%s security events status = %d, body = %s", name, recorder.Code, recorder.Body.String())
		}
	}
}
