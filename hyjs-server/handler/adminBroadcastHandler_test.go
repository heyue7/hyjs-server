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
	"github.com/google/uuid"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"hyjs-server/service"
)

// TestAdminBroadcastHTTPRealmAuthCSRFPreviewAndReceipt exercises the actual registered HTTP boundary.
func TestAdminBroadcastHTTPRealmAuthCSRFPreviewAndReceipt(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := db.InitGame(ctx, filepath.Join(dir, "broadcast.sqlite")); err != nil {
		t.Fatal(err)
	}
	connection, _ := db.GetIns().DB()
	t.Cleanup(func() { connection.Close() })
	boot := filepath.Join(dir, "bootstrap.json")
	if err := os.WriteFile(boot, []byte(`{"username":"broadcastfixtureadmin","password":"isolated-broadcast-password-12345"}`), 0600); err != nil {
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
	if err := gameService.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	for _, initialize := range []func(context.Context) error{gameService.InitializeP1, gameService.InitializeAdminGameServers, gameService.InitializeAdminBroadcast, gameService.InitializeAdminMail} {
		if err := initialize(ctx); err != nil {
			t.Fatal(err)
		}
	}
	// Two normal protocol-created roles provide a protected legacy role and one ordinary recipient.
	for i := 0; i < 2; i++ {
		username := "broadcast_" + uuid.NewString()[:16]
		if _, err := service.RegisterGameAccount(ctx, username, "isolated-game-password"); err != nil {
			t.Fatal(err)
		}
		account, err := service.LoginAccount(ctx, username, "isolated-game-password")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = service.CreatePlayer(ctx, account.Id, "群发验收", "hero_043"); err != nil {
			t.Fatal(err)
		}
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterAdminRoutes(router, "")
	call := func(method, path string, body any, csrf string, cookies []*http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		raw, _ := json.Marshal(body)
		request := httptest.NewRequest(method, path, strings.NewReader(string(raw)))
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
	if got := call("GET", "/admin/api/broadcast?serverId=1", nil, "", nil); got.Code != 401 {
		t.Fatal("anonymous", got.Code)
	}
	login := call("POST", "/admin/api/auth/login", map[string]string{"username": "broadcastfixtureadmin", "password": "isolated-broadcast-password-12345"}, "", nil)
	if login.Code != 200 {
		t.Fatal("login", login.Code)
	}
	var authenticated struct {
		Data struct {
			CSRF string `json:"csrfToken"`
		}
	}
	if err := json.Unmarshal(login.Body.Bytes(), &authenticated); err != nil {
		t.Fatal(err)
	}
	cookies, csrf := login.Result().Cookies(), authenticated.Data.CSRF
	request := model.AdminBroadcastPreviewRequest{RequestID: uuid.NewString(), Audience: model.AdminAudience{Mode: constUtil.AdminAudienceModeAll}, Title: "停服补偿", Body: "隔离HTTP代表", Rewards: []model.AdminResourceAmount{{ResourceID: "coin", Amount: 7}}, Reason: "finite HTTP boundary verification"}
	if got := call("POST", "/admin/api/broadcast/preview?serverId=1", request, "", cookies); got.Code != 403 {
		t.Fatal("csrf", got.Code)
	}
	if got := call("GET", "/admin/api/broadcast?serverId=bad", nil, "", cookies); got.Code != 400 {
		t.Fatal("bad realm", got.Code)
	}
	if got := call("POST", "/admin/api/broadcast/preview?serverId=1", model.AdminBroadcastPreviewRequest{RequestID: "bad"}, csrf, cookies); got.Code != 400 {
		t.Fatal("invalid request", got.Code)
	}
	invalidAudience := request
	invalidAudience.RequestID = uuid.NewString()
	invalidAudience.Audience = model.AdminAudience{Mode: constUtil.AdminAudienceModeAll, PlayerIDs: []int64{1}}
	if got := call("POST", "/admin/api/broadcast/preview?serverId=1", invalidAudience, csrf, cookies); got.Code != 400 {
		t.Fatal("all audience with explicit ids", got.Code)
	}
	preview := call("POST", "/admin/api/broadcast/preview?serverId=1", request, csrf, cookies)
	if preview.Code != 200 {
		t.Fatal("preview", preview.Code, preview.Body.String())
	}
	var frozen struct {
		Data model.AdminBroadcastPreviewResponse
	}
	if err := json.Unmarshal(preview.Body.Bytes(), &frozen); err != nil || frozen.Data.Batch.RecipientCount != 1 {
		t.Fatal("preview shape", err, frozen)
	}
	var notices int64
	db.GetIns().Model(&db.RewardDelivery{}).Where("source_key=?", "admin-broadcast:"+frozen.Data.Batch.ID).Count(&notices)
	if notices != 0 {
		t.Fatal("preview delivered", notices)
	}
	confirm := model.AdminBroadcastSendRequest{RequestID: uuid.NewString(), Version: frozen.Data.Batch.Version}
	path := "/admin/api/broadcast/" + frozen.Data.Batch.ID + "/send?serverId=1"
	for i := 0; i < 2; i++ {
		sent := call("POST", path, confirm, csrf, cookies)
		if sent.Code != 200 {
			t.Fatal("send or retry", sent.Code, sent.Body.String())
		}
	}
	db.GetIns().Model(&db.RewardDelivery{}).Where("source_key=?", "admin-broadcast:"+frozen.Data.Batch.ID).Count(&notices)
	if notices != 1 {
		t.Fatal("HTTP retry duplicated notices", notices)
	}
	var mails int64
	db.GetIns().Model(&db.MailMessage{}).Where("source_key=?", "admin-broadcast:"+frozen.Data.Batch.ID).Count(&mails)
	if mails != 0 {
		t.Fatal("HTTP compensation created mail", mails)
	}
	var recipient db.AdminBroadcastRecipient
	if err := db.GetIns().Where("batch_id=?", frozen.Data.Batch.ID).First(&recipient).Error; err != nil {
		t.Fatal(err)
	}
	selected := request
	selected.RequestID = uuid.NewString()
	selected.Audience = model.AdminAudience{Mode: constUtil.AdminAudienceModePlayers, PlayerIDs: []int64{recipient.PlayerID}}
	selectedPreview := call("POST", "/admin/api/broadcast/preview?serverId=1", selected, csrf, cookies)
	if selectedPreview.Code != 200 {
		t.Fatal("players audience preview", selectedPreview.Code, selectedPreview.Body.String())
	}
	var selectedFrozen struct {
		Data model.AdminBroadcastPreviewResponse
	}
	if err := json.Unmarshal(selectedPreview.Body.Bytes(), &selectedFrozen); err != nil || selectedFrozen.Data.Batch.RecipientCount != 1 || selectedFrozen.Data.Batch.AudienceMode != constUtil.AdminAudienceModePlayers {
		t.Fatal("players audience shape", err, selectedFrozen)
	}
	confirm.RequestID = uuid.NewString()
	if got := call("POST", path, confirm, csrf, cookies); got.Code != 409 {
		t.Fatal("different confirmation", got.Code)
	}
	history := call("GET", "/admin/api/broadcast?serverId=1&page=1&pageSize=25", nil, "", cookies)
	var list struct {
		Data model.AdminBroadcastListResponse
	}
	if history.Code != 200 || json.Unmarshal(history.Body.Bytes(), &list) != nil || list.Data.Total != 1 || list.Data.Items[0].SuccessCount != 1 {
		t.Fatal("history", history.Code)
	}

	if got := call("GET", "/admin/api/mail?serverId=1", nil, "", nil); got.Code != 401 {
		t.Fatal("mail anonymous", got.Code)
	}
	mailRequest := model.AdminMailPreviewRequest{RequestID: uuid.NewString(), Audience: selected.Audience, Title: "独立邮件", Body: "邮件HTTP验收", Reason: "isolated HTTP fixture"}
	if got := call("POST", "/admin/api/mail/preview?serverId=1", mailRequest, "", cookies); got.Code != 403 {
		t.Fatal("mail csrf", got.Code)
	}
	badMail := mailRequest
	badMail.RequestID = "bad"
	if got := call("POST", "/admin/api/mail/preview?serverId=1", badMail, csrf, cookies); got.Code != 400 {
		t.Fatal("mail invalid", got.Code)
	}
	mailPreview := call("POST", "/admin/api/mail/preview?serverId=1", mailRequest, csrf, cookies)
	var mailFrozen struct {
		Data model.AdminMailPreviewResponse
	}
	if mailPreview.Code != 200 || json.Unmarshal(mailPreview.Body.Bytes(), &mailFrozen) != nil || mailFrozen.Data.Batch.RecipientCount != 1 {
		t.Fatal("mail preview", mailPreview.Code, mailPreview.Body.String())
	}
	mailSend := model.AdminMailSendRequest{RequestID: uuid.NewString(), Version: mailFrozen.Data.Batch.Version}
	mailPath := "/admin/api/mail/" + mailFrozen.Data.Batch.ID + "/send?serverId=1"
	for i := 0; i < 2; i++ {
		if got := call("POST", mailPath, mailSend, csrf, cookies); got.Code != 200 {
			t.Fatal("mail send/retry", got.Code, got.Body.String())
		}
	}
	mailSend.RequestID = uuid.NewString()
	if got := call("POST", mailPath, mailSend, csrf, cookies); got.Code != 409 {
		t.Fatal("mail confirmation conflict", got.Code)
	}
	db.GetIns().Model(&db.MailMessage{}).Where("source_key=?", "admin-mail:"+mailFrozen.Data.Batch.ID).Count(&mails)
	db.GetIns().Model(&db.RewardDelivery{}).Where("source_key=?", "admin-mail:"+mailFrozen.Data.Batch.ID).Count(&notices)
	if mails != 1 || notices != 0 {
		t.Fatal("mail protocol separation", mails, notices)
	}
	var token string
	for _, cookie := range cookies {
		if cookie.Name == constUtil.AdminSessionCookieName {
			token = cookie.Value
		}
	}
	actor, err := service.AdminAuthenticate(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.GetIns().Model(&db.AdminUser{}).Where("id=?", actor.UserID).Update("role", constUtil.AdminRoleViewer).Error; err != nil {
		t.Fatal(err)
	}
	request.RequestID = uuid.NewString()
	if got := call("POST", "/admin/api/broadcast/preview?serverId=1", request, csrf, cookies); got.Code != 403 {
		t.Fatal("viewer write", got.Code)
	}
	if got := call("POST", "/admin/api/mail/preview?serverId=1", mailRequest, csrf, cookies); got.Code != 403 {
		t.Fatal("viewer mail write", got.Code)
	}
	if got := call("GET", "/admin/api/mail?serverId=1", nil, "", cookies); got.Code != 200 {
		t.Fatal("viewer mail read", got.Code)
	}
	if got := call("GET", "/admin/api/broadcast?serverId=1", nil, "", cookies); got.Code != 200 {
		t.Fatal("viewer read", got.Code)
	}
}
