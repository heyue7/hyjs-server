package service_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/handler"
	"hyjs-server/model"
	"hyjs-server/service"
)

func TestClientUpdateSemanticHTTP(t *testing.T) {
	ctx := context.Background()
	if err := db.InitGame(ctx, filepath.Join(t.TempDir(), "semantic-http.sqlite")); err != nil {
		t.Fatal(err)
	}
	d := db.GetIns()
	sqlDB, err := d.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close(); handler.ConfigureGame(nil) })
	if err = d.AutoMigrate(&db.AdminUser{}, &db.AdminAudit{}); err != nil {
		t.Fatal(err)
	}
	game := &service.GameService{}
	if err = game.InitializeClientUpdates(ctx); err != nil {
		t.Fatal(err)
	}
	handler.ConfigureGame(game)
	user := db.AdminUser{Username: "semantic-http-admin", Role: constUtil.AdminRoleAdmin, Enabled: true}
	if err = d.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	actor := &model.AdminPrincipal{UserID: user.ID, Username: user.Username, Role: user.Role}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/client-update/status", handler.ClientUpdateStatusHandler)
	router.POST("/client-update/ack", handler.ClientUpdateAckHandler)
	admin := router.Group("/admin")
	admin.Use(func(c *gin.Context) { c.Set(constUtil.AdminPrincipalContextKey, actor); c.Next() })
	handler.RegisterAdminClientUpdateRoutes(admin)
	call := func(path string, value any) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		router.ServeHTTP(out, request)
		return out
	}
	draft := model.AdminClientUpdateDraftRequest{Version: "0", VersionPolicy: true, VersionName: "01.1.1", Mode: "notice", Title: "更新", Content: "更新说明", Reason: "test"}
	if out := call("/admin/client-updates/draft", draft); out.Code != http.StatusBadRequest {
		t.Fatalf("invalid draft: %d %s", out.Code, out.Body)
	}
	draft.VersionName = "1.1.1"
	out := call("/admin/client-updates/draft", draft)
	if out.Code != http.StatusOK {
		t.Fatalf("semantic draft: %d %s", out.Code, out.Body)
	}
	var draftReply struct {
		Data model.AdminClientUpdatesResponse `json:"data"`
	}
	if err = json.Unmarshal(out.Body.Bytes(), &draftReply); err != nil || len(draftReply.Data.Items) != 1 || !draftReply.Data.Items[0].VersionPolicy {
		t.Fatalf("draft response: %+v %v", draftReply, err)
	}
	action := model.AdminClientUpdateActionRequest{ID: draftReply.Data.Items[0].ID, Version: draftReply.Data.Version, Action: "publish", RequestID: "semantic-http-publish", Reason: "test"}
	out = call("/admin/client-updates/action", action)
	if out.Code != http.StatusOK {
		t.Fatalf("publish: %d %s", out.Code, out.Body)
	}
	var actionReply struct {
		Data model.AdminClientUpdatesResponse `json:"data"`
	}
	if err = json.Unmarshal(out.Body.Bytes(), &actionReply); err != nil || !actionReply.Data.Policy.VersionPolicy {
		t.Fatalf("publish response: %+v %v", actionReply, err)
	}
	startup := "1234567890abcdef1234567890abcdef"
	status := model.ClientUpdateStatusRequest{BuildCode: 2026110101, StartupID: startup}
	status.ClientVersion = "1.01.0"
	if out = call("/client-update/status", status); out.Code != http.StatusBadRequest {
		t.Fatalf("invalid status version: %d %s", out.Code, out.Body)
	}
	status.ClientVersion = ""
	out = call("/client-update/status", status)
	if out.Code != http.StatusOK {
		t.Fatalf("old status: %d %s", out.Code, out.Body)
	}
	var statusReply struct {
		Data model.ClientUpdateStatusResponse `json:"data"`
	}
	if err = json.Unmarshal(out.Body.Bytes(), &statusReply); err != nil || !statusReply.Data.VersionPolicy || statusReply.Data.VersionName != "1.1.1" || !statusReply.Data.NoticeEnabled || statusReply.Data.LatestBuild <= 2100000000 {
		t.Fatalf("old status response: %+v %v", statusReply, err)
	}
	profile := &db.Profile{Uid: "semantic-http-player", Session: []byte("1234567890123456")}
	if err = d.Create(profile).Error; err != nil {
		t.Fatal(err)
	}
	if err = game.BindClientLogin(ctx, profile, fmt.Sprintf("hyjs/%d/%s/1.1.1", int64(2026110102), startup)); err != nil {
		t.Fatal(err)
	}
	status.BuildCode, status.PlayerID, status.SessionHex = 2026110102, profile.Id, hex.EncodeToString(profile.Session)
	status.ClientVersion = "1.1.2"
	if out = call("/client-update/status", status); out.Code != http.StatusConflict {
		t.Fatalf("forged version status: %d %s", out.Code, out.Body)
	}
	ack := model.ClientUpdateAckRequest{BuildCode: status.BuildCode, ClientVersion: "1.01.1", StartupID: startup, BatchID: actionReply.Data.Policy.NoticeBatch, Kind: "shown"}
	if out = call("/client-update/ack", ack); out.Code != http.StatusBadRequest {
		t.Fatalf("invalid ack version: %d %s", out.Code, out.Body)
	}
	ack.ClientVersion, ack.PlayerID, ack.SessionHex = "1.1.2", profile.Id, status.SessionHex
	if out = call("/client-update/ack", ack); out.Code != http.StatusConflict {
		t.Fatalf("forged version ack: %d %s", out.Code, out.Body)
	}
	ack.ClientVersion = "1.1.1"
	if out = call("/client-update/ack", ack); out.Code != http.StatusOK {
		t.Fatalf("valid version ack: %d %s", out.Code, out.Body)
	}
	draft.Version = actionReply.Data.Version
	draft.VersionName, draft.Mode = "1.1.2", "maintenance"
	draft.MaintenanceStartAt = time.Now().UnixMilli() + 100000
	draft.MaintenanceEndAt = draft.MaintenanceStartAt + 100000
	out = call("/admin/client-updates/draft", draft)
	if out.Code != http.StatusOK {
		t.Fatalf("maintenance draft: %d %s", out.Code, out.Body)
	}
	if err = json.Unmarshal(out.Body.Bytes(), &draftReply); err != nil || len(draftReply.Data.Items) == 0 {
		t.Fatalf("maintenance draft response: %+v %v", draftReply, err)
	}
	action = model.AdminClientUpdateActionRequest{ID: draftReply.Data.Items[0].ID, Version: draftReply.Data.Version, Action: "publish", RequestID: "semantic-http-maintenance-publish", Reason: "test"}
	out = call("/admin/client-updates/action", action)
	if out.Code != http.StatusOK {
		t.Fatalf("maintenance publish: %d %s", out.Code, out.Body)
	}
	if err = json.Unmarshal(out.Body.Bytes(), &actionReply); err != nil {
		t.Fatal(err)
	}
	action.Version, action.Action, action.RequestID = actionReply.Data.Version, "cancel_maintenance", "semantic-http-maintenance-cancel"
	out = call("/admin/client-updates/action", action)
	if out.Code != http.StatusOK {
		t.Fatalf("maintenance cancel: %d %s", out.Code, out.Body)
	}
	if err = json.Unmarshal(out.Body.Bytes(), &actionReply); err != nil || actionReply.Data.Policy.MaintenanceStartAt != 0 || actionReply.Data.Policy.MaintenanceEndAt != 0 || actionReply.Data.Policy.NoticeEnabled || actionReply.Data.Policy.MinLoginVersion != "" || len(actionReply.Data.Items) == 0 || actionReply.Data.Items[0].State != "cancelled" {
		t.Fatalf("cancel response: %+v %v", actionReply, err)
	}
}
