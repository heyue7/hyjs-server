package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"hyjs-server/db"
	"hyjs-server/model"
	"hyjs-server/service"
	"hyjs-server/utils"
	"hyjs-server/wrapper"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestGameAccountCredentials(t *testing.T) {
	token := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	for _, v := range []struct {
		token   string
		id      int64
		session string
		valid   bool
	}{
		{token, 0, "", true}, {"", 1, "0123456789abcdef0123456789abcdef", true}, {token, 1, "0123456789abcdef0123456789abcdef", true},
		{"", 0, "", false}, {token, 1, "", false}, {token, 0, "0123456789abcdef0123456789abcdef", false}, {"bad", 0, "", false},
	} {
		if got := gameAccountCredentialsValid(v.token, v.id, v.session); got != v.valid {
			t.Fatal(v, got)
		}
	}
}

// Historical credentials pass through the actual HTTP handler and password verifier.
func TestGameAccountLoginHistoricalLongPassword(t *testing.T) {
	ctx := context.Background()
	if e := db.InitGame(ctx, filepath.Join(t.TempDir(), "long-login.sqlite")); e != nil {
		t.Fatal(e)
	}
	connection, e := db.GetIns().DB()
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { connection.Close() })
	if e = db.InitializeGameAccountsSchema(ctx); e != nil {
		t.Fatal(e)
	}
	if e = service.InitializeLoginSecurity(ctx); e != nil {
		t.Fatal(e)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/game-account/login", wrapper.Wrapper(GameAccountLoginHandler))
	call := func(username, password string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"username": username, "password": password})
		request := httptest.NewRequest("POST", "/game-account/login", strings.NewReader(string(body)))
		request.Header.Set("Content-Type", "application/json")
		request.RemoteAddr = "198.51.100.191:31415"
		out := httptest.NewRecorder()
		router.ServeHTTP(out, request)
		return out
	}
	for i, password := range []string{strings.Repeat("a", 80), strings.Repeat("密", 26), strings.Repeat("界", 128)} {
		username := fmt.Sprintf("historical-long-%d", i)
		salt, hash, e := utils.NewPasswordCredential(ctx, password)
		if e != nil {
			t.Fatal(e)
		}
		account := db.GameAccount{Username: username, Nickname: "历史账号", Enabled: true, PasswordSalt: salt, PasswordHash: hash, Revision: 1}
		if e = db.AccountsTable(db.GetIns(), "game_accounts").Create(&account).Error; e != nil {
			t.Fatal(e)
		}
		response := call(username, password)
		var decoded struct {
			Code int
			Data model.GameAccountLoginResponse
		}
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &decoded) != nil || decoded.Code != 0 || decoded.Data.Account.ID != account.ID || len(decoded.Data.AccountSession) != 64 {
			t.Fatal("historical long credential rejected", i, response.Code)
		}
		var count int64
		db.GetIns().Model(&db.Profile{}).Where("account_id=?", account.ID).Count(&count)
		if count != 0 {
			t.Fatal("global authentication created a realm profile")
		}
	}
	for _, password := range []string{"short", strings.Repeat("a", 129), strings.Repeat("a", 1025)} {
		response := call("historical-long-0", password)
		var decoded model.BizCode
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &decoded) != nil || decoded.Code != model.NewErrParamsError().Code {
			t.Fatal("invalid original login length accepted", response.Code)
		}
	}
}
